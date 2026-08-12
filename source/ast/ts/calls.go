package ts

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/c360studio/semsource/source/ast"
)

// TS/JS call-graph extraction (code-call-graph, D3). A call resolves to the
// entity ID of its callee's DEFINITION so `code.relationship.calls` edges
// connect instead of dangling. Resolution FAILS INERT: an edge is emitted only
// for a same-file function/arrow-function declarator, an in-tree import binding
// (named, namespace-qualified, or default) confirmed by parsing the target
// module, or a `this.` call resolved on the current class's own method set.
// Everything else — property chains (`a.b.c()`), computed callees (`obj[k]()`),
// call chains, re-export barrels beyond one hop — emits nothing, never a
// guessed edge.
//
// Exported for the Svelte parser (ExtractCalls, PrepareCallResolution): a
// component's script block parses as its own tree-sitter tree, rooted exactly
// like a standalone .ts file's, so the identical pass resolves it (design D3) —
// the Svelte parser just supplies "svelte" as the local-entity domain instead of
// "typescript"/"javascript" so a script call's edge byte-matches the component's
// own entity-ID construction.

// moduleCallInfo is the module-level view of another file needed to confirm an
// import-bound call target. Two DIFFERENT confirmable sets, deliberately kept
// apart:
//
//   - funcs is EXPORT-AWARE (collectModuleExportedFuncs): keyed by the PUBLIC
//     exported name (what an importer writes), valued by the INTERNAL
//     definition name (what the entity's own ID was built from) — these
//     DIFFER for an aliased export-list entry (`export { helper as h }`: an
//     importer writes `h`, but the function's own entity ID still uses
//     "helper", its real source-level name). Used for named- and namespace-
//     import resolution, where the imported name must be something the
//     module actually, publicly exports, but the edge must point at the
//     definition's REAL id, not the alias it was imported under.
//   - allDefs is every top-level function/arrow definition regardless of
//     export status (collectModuleFuncNames). Used ONLY to confirm a default
//     export's identifier-reference form (`export default foo;`): naming foo
//     as the default IS the export act, so foo need not ALSO appear in a
//     separate named export — checking it against funcs (as an earlier
//     version of this code did) wrongly rejected the common `function
//     foo(){} ... export default foo;` pattern, since foo is rarely also
//     separately named-exported.
//
// Both are computed once per relPath and reused for every call site that
// references it — one parse serves named-import, namespace-import, and
// default-import resolution alike.
type moduleCallInfo struct {
	funcs       map[string]string // public exported name -> internal definition name
	allDefs     map[string]bool
	defaultName string
	hasDefault  bool
}

// moduleInfo returns another file's call-resolution view, parsing it once per
// ParseFile (memoized by relPath — mirrors python/calls.go's moduleFuncs). The
// memo map itself is reallocated every ParseFile (see ParseFile in parser.go),
// so an edited target module is always re-read on the next parse rather than
// served stale; it only dedupes repeated lookups of the same imported module
// within one file's call-resolution pass.
func (p *Parser) moduleInfo(relPath string) moduleCallInfo {
	if cached, ok := p.moduleInfoMemo[relPath]; ok {
		return cached
	}
	info := moduleCallInfo{funcs: map[string]string{}, allDefs: map[string]bool{}}
	if content, err := os.ReadFile(filepath.Join(p.repoRoot, relPath)); err == nil {
		mp := sitter.NewParser()
		mp.SetLanguage(p.getTreeSitterLanguage(relPath))
		if tree, terr := mp.ParseCtx(context.Background(), nil, content); terr == nil {
			info.allDefs = collectModuleFuncNames(tree.RootNode(), content)
			info.funcs = collectModuleExportedFuncs(tree.RootNode(), content, info.allDefs)
			info.defaultName, info.hasDefault = findDefaultExportFunc(tree.RootNode(), content)
			tree.Close()
		}
	}
	if p.moduleInfoMemo == nil {
		p.moduleInfoMemo = make(map[string]moduleCallInfo)
	}
	p.moduleInfoMemo[relPath] = info
	return info
}

// collectModuleFuncNames collects a module's TOP-LEVEL function names into a
// name set: function_declaration and arrow-function-valued const/let
// declarators, exported or not. Used for the file being parsed (same-file
// bare-call resolution, via p.localFuncs) — export status is irrelevant there,
// since a private function is freely callable from within its own file — and
// as the local-definition base collectModuleExportedFuncs layers export
// confirmation on top of for CROSS-file resolution.
//
// Deliberately NOT recursive beyond one `export_statement` unwrap: a name
// declared inside a nested function/block is not module-level and must not be
// treated as one (mirrors python/calls.go's extractLocalFunctions, which scans
// only root.NamedChild(i)).
func collectModuleFuncNames(root *sitter.Node, source []byte) map[string]bool {
	funcs := make(map[string]bool)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		collectTopLevelFuncNode(root.NamedChild(i), source, funcs)
	}
	return funcs
}

func collectTopLevelFuncNode(node *sitter.Node, source []byte, funcs map[string]bool) {
	switch node.Type() {
	case "function_declaration":
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			funcs[nodeText(nameNode, source)] = true
		}
	case "lexical_declaration", "variable_declaration":
		collectTopLevelArrowNames(node, source, funcs)
	case "export_statement":
		// `export function f(){}` / `export const f = () => {}` wrap their
		// declaration one level deep; `export default ...` and `export {a, b}`
		// re-export lists carry no "declaration" field and are deliberately not
		// unwrapped here (default handled separately by findDefaultExportFunc;
		// re-export barrels beyond one hop stay inert per D3).
		if decl := node.ChildByFieldName("declaration"); decl != nil {
			collectTopLevelFuncNode(decl, source, funcs)
		}
	}
}

// collectTopLevelArrowNames records the arrow-function-valued declarators of a
// top-level const/let/var statement. `const f = function(){}` (a plain function
// expression, not an arrow) is deliberately excluded: parser.go's own
// simpleDeclaratorEntity only special-cases arrow_function values into a
// TypeFunction entity, so a function-expression const has no such entity to
// point a call edge at.
func collectTopLevelArrowNames(node *sitter.Node, source []byte, funcs map[string]bool) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		decl := node.NamedChild(i)
		if decl.Type() != "variable_declarator" {
			continue
		}
		valueNode := decl.ChildByFieldName("value")
		if valueNode == nil || valueNode.Type() != "arrow_function" {
			continue
		}
		if nameNode := decl.ChildByFieldName("name"); nameNode != nil {
			funcs[nodeText(nameNode, source)] = true
		}
	}
}

// collectModuleExportedFuncs computes the EXPORT-AWARE confirmable function
// map for a module: PUBLIC exported name -> INTERNAL definition name. A
// public name confirms only when THIS FILE both defines it as a
// function/arrow-function AND exports it under that public name — either an
// export-wrapped in-place declaration (`export function f(){}` / `export const
// f = () => {}`, definition and export are the same statement, so public and
// internal names are identical) or a LOCAL export-list entry (`export { f }` /
// `export { f as g }`, with no `from` clause) whose internal name resolves to
// a local definition. The two names DIFFER for an aliased export-list entry:
// an importer of `g` must still land on `f`'s entity ID, since that is the
// definition's real, source-level name.
//
// A RE-EXPORT (`export { f } from './impl'`) does NOT confirm anything here,
// even when a same-named PRIVATE local definition happens to exist in this
// file: the review finding this closes — `export { f } from './impl'` beside a
// private `function f(){}` was resolving imports of `f` against the wrong,
// private, in-file definition instead of staying inert. impl's `f` is not
// this file's `f`, and barrel-chasing past one hop is out of scope (D3); a
// private (never-exported) definition confirms nothing on its own either — an
// import naming it would not even compile against the real module. localDefs
// is the raw definition set (collectModuleFuncNames), passed in rather than
// recomputed so moduleInfo's single walk serves both this and the default-
// export check. Used for BOTH named and namespace-qualified resolution
// (moduleInfo.funcs is shared).
func collectModuleExportedFuncs(root *sitter.Node, source []byte, localDefs map[string]bool) map[string]string {
	exported := make(map[string]string)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		node := root.NamedChild(i)
		if node.Type() != "export_statement" {
			continue
		}
		if decl := node.ChildByFieldName("declaration"); decl != nil {
			// export function f(){} / export const f = () => {} — the export
			// and the definition are the SAME statement, so whatever name(s)
			// this contributes are, by construction, already real definitions,
			// and there is no alias form here: public == internal.
			names := make(map[string]bool)
			collectTopLevelFuncNode(decl, source, names)
			for name := range names {
				exported[name] = name
			}
			continue
		}
		if node.ChildByFieldName("source") != nil {
			continue // re-export — never confirms a local definition (one-hop rule, D3)
		}
		clause := findChild(node, "export_clause")
		if clause == nil {
			continue
		}
		for j := 0; j < int(clause.NamedChildCount()); j++ {
			spec := clause.NamedChild(j)
			if spec.Type() != "export_specifier" {
				continue
			}
			nameNode := spec.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			localName := nodeText(nameNode, source)
			if !localDefs[localName] {
				continue // names something that isn't a local function definition — not our concern here
			}
			publicName := localName
			if alias := spec.ChildByFieldName("alias"); alias != nil {
				publicName = nodeText(alias, source)
			}
			exported[publicName] = localName
		}
	}
	return exported
}

// isDefaultExport reports whether an export_statement is `export default ...`
// rather than a plain `export ...`. The "default" keyword is an ANONYMOUS
// token in this grammar — invisible to ChildByFieldName and to a NamedChild-
// only walk — so `export function f(){}` and `export default function f(){}`
// produce an IDENTICAL `declaration: (function_declaration ...)` shape under
// NamedChild traversal (confirmed by dumping both and diffing the printed
// s-expressions: they were byte-identical). Only a full Child scan, which also
// visits unnamed tokens, can tell them apart — this was the review finding: a
// plain named export was being fabricated into a default-import's target.
func isDefaultExport(node *sitter.Node) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		if node.Child(i).Type() == "default" {
			return true
		}
	}
	return false
}

// findDefaultExportFunc reports the name a module's default export resolves
// to, when that name can be determined without inference: `export default
// function foo(){}` names foo directly; `export default foo;` names the
// identifier it references (confirmed as an actual top-level function by the
// caller via moduleInfo.funcs, since the identifier could equally name a class
// or const). An anonymous default export (`export default function(){}`,
// `export default () => {}`, `export default {...}`) has no name to point an
// edge at and reports ok=false. A plain (non-default) `export function f(){}`
// is EXCLUDED by isDefaultExport — it exports f under its own name, not as the
// module's default, so it must never confirm a default-import target.
func findDefaultExportFunc(root *sitter.Node, source []byte) (name string, ok bool) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		node := root.NamedChild(i)
		if node.Type() != "export_statement" || !isDefaultExport(node) {
			continue
		}
		if decl := node.ChildByFieldName("declaration"); decl != nil {
			if decl.Type() == "function_declaration" {
				if nameNode := decl.ChildByFieldName("name"); nameNode != nil {
					return nodeText(nameNode, source), true
				}
			}
			continue // export default class / other named declaration — not a function
		}
		if value := node.ChildByFieldName("value"); value != nil && value.Type() == "identifier" {
			return nodeText(value, source), true
		}
	}
	return "", false
}

// extractNamespaceImports walks a file's import statements into a
// localNamespaceName -> module-specifier map. Complements extractImportBindings
// (imports.go), which deliberately skips namespace imports for TYPE resolution
// (a `NS.Type` reference is already handled as external there) — call
// resolution instead confirms `ns.f()` against the namespace's target module.
func extractNamespaceImports(root *sitter.Node, source []byte) map[string]string {
	bindings := make(map[string]string)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "import_statement" {
			addNamespaceImport(n, source, bindings)
			return
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(root)
	return bindings
}

func addNamespaceImport(node *sitter.Node, source []byte, bindings map[string]string) {
	spec := ""
	if s := node.ChildByFieldName("source"); s != nil {
		spec = strings.Trim(nodeText(s, source), `'"`)
	}
	if spec == "" {
		return
	}
	clause := findChild(node, "import_clause")
	if clause == nil {
		return
	}
	for i := 0; i < int(clause.NamedChildCount()); i++ {
		child := clause.NamedChild(i)
		if child.Type() != "namespace_import" || child.NamedChildCount() == 0 {
			continue
		}
		bindings[nodeText(child.NamedChild(0), source)] = spec
	}
}

// extractDefaultImports walks a file's import statements into a
// localName -> module-specifier map, default-import bindings only (`import Def
// from './def'`). Kept separate from importBindings (imports.go): a default
// import's callee is confirmed against the TARGET module's declared default
// export (findDefaultExportFunc), never against a same-named top-level
// declaration the way a named import is — the two forms need different
// resolution, so they need different bindings.
func extractDefaultImports(root *sitter.Node, source []byte) map[string]string {
	bindings := make(map[string]string)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "import_statement" {
			addDefaultImport(n, source, bindings)
			return
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(root)
	return bindings
}

func addDefaultImport(node *sitter.Node, source []byte, bindings map[string]string) {
	spec := ""
	if s := node.ChildByFieldName("source"); s != nil {
		spec = strings.Trim(nodeText(s, source), `'"`)
	}
	if spec == "" {
		return
	}
	clause := findChild(node, "import_clause")
	if clause == nil {
		return
	}
	for i := 0; i < int(clause.NamedChildCount()); i++ {
		child := clause.NamedChild(i)
		if child.Type() == "identifier" {
			bindings[nodeText(child, source)] = spec
		}
	}
}

// classMethodNames collects the method names defined directly in a class body
// (constructor excluded, matching extractMethod's own skip in parser.go), so a
// `this.m()` call resolves only when m is actually a method of THIS class — an
// inherited/mixin method stays inert (mirrors python/calls.go's
// classMethodNames; D3 explicitly rules out a supers walk for TS).
func classMethodNames(classBody *sitter.Node, source []byte) map[string]bool {
	names := make(map[string]bool)
	for i := 0; i < int(classBody.ChildCount()); i++ {
		child := classBody.Child(i)
		if child.Type() != "method_definition" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nodeText(nameNode, source)
		if name == "constructor" {
			continue
		}
		names[name] = true
	}
	return names
}

// thisIsRebound reports whether a `this` reference at node is shadowed by an
// intervening ordinary function between it and bodyRoot: a plain
// function_declaration/function_expression rebinds `this` (a callback passed to
// e.g. addEventListener does not receive the enclosing method's `this`), and so
// — reachable only through a class expression — does a nested method_definition.
// An arrow function does not rebind `this` and is deliberately excluded, so
// `this.m()` inside a same-method arrow callback still resolves. Treating a
// rebound `this` as the enclosing class's would be a fabricated edge, not a
// missing one.
func thisIsRebound(node, bodyRoot *sitter.Node) bool {
	for n := node.Parent(); n != nil && n != bodyRoot; n = n.Parent() {
		switch n.Type() {
		case "function_declaration", "function_expression", "generator_function", "generator_function_declaration", "method_definition":
			return true
		}
	}
	return false
}

// ArrowParamsNode returns an arrow function's parameter node, accounting for
// the unparenthesized single-identifier shorthand (`x => ...`), which tree-
// sitter exposes under a DIFFERENT, singular field ("parameter") than the
// normal parenthesized form ("parameters", wrapping a formal_parameters node).
// Missing this shape would silently under-suppress the exact class of bug this
// file guards against — a shorthand arrow's own parameter is a local value.
// Exported so the Svelte parser's own arrow-function wiring (which drives
// ExtractCalls the same way parser.go does) uses the identical fallback.
func ArrowParamsNode(valueNode *sitter.Node) *sitter.Node {
	if params := valueNode.ChildByFieldName("parameters"); params != nil {
		return params
	}
	return valueNode.ChildByFieldName("parameter")
}

// localValueNames collects every name a function-like node's parameters and
// body bind LOCALLY: parameters (including destructured/rest patterns and the
// unparenthesized arrow shorthand, via arrowParamsNode/CollectPatternBindings),
// every let/const/var declarator (including destructuring — a classic
// `for (let i = ...)` initializer is covered for free, since tree-sitter nests
// it as an ordinary lexical_declaration child), for-in/for-of loop variables,
// catch-clause parameters, and nested function/class declarations.
//
// A bare or namespace-qualified call through any of these names is a call
// through a LOCAL VALUE — a parameter, a reassigned local, a loop variable —
// never a reference to the module-level or imported definition of the same
// name. The spec's "function-typed parameter" rule generalizes to every local
// binding, not just parameters: `function run(items, transform) {
// items.map(i => transform(i)) }` must not resolve `transform` against an
// unrelated module-level or imported function of the same name (mirrors
// c/calls.go's localValueNames).
//
// The set is FLAT across block depth: a name declared in one if/for block
// shadows the WHOLE enclosing function, not just its own block. This can
// over-suppress a real, resolvable edge when an unrelated sibling block
// happens to reuse the name, but it never under-suppresses a genuine shadow —
// inert is the safe direction the design doctrine already prefers over a
// wrong edge, so a full scope stack is not required here.
func localValueNames(params, body *sitter.Node, source []byte) map[string]bool {
	names := make(map[string]bool)
	bindPattern := func(pattern *sitter.Node) {
		if pattern == nil {
			return
		}
		var bindings []*sitter.Node
		ast.CollectPatternBindings(pattern, &bindings)
		for _, b := range bindings {
			names[nodeText(b, source)] = true
		}
	}
	switch {
	case params == nil:
	case params.Type() == "formal_parameters":
		for i := 0; i < int(params.NamedChildCount()); i++ {
			bindPattern(params.NamedChild(i).ChildByFieldName("pattern"))
		}
	default:
		bindPattern(params) // unparenthesized arrow shorthand: the field IS the pattern
	}
	if body == nil {
		return names
	}
	// bindNestedParams binds a nested function-like node's OWN parameters.
	// The call walk deliberately crosses nested-function boundaries, so a
	// callback's parameter (`items.map((transform) => transform(1))`) shadows
	// exactly like a local — leaving it unbound was the one remaining
	// wrong-edge path (re-review NIT 1; measured 0 of 3,593 ui/ edges, closed
	// so the spec's "in any language" holds as written, not in-practice).
	bindNestedParams := func(n *sitter.Node) {
		switch n.Type() {
		case "arrow_function":
			nested := ArrowParamsNode(n)
			if nested == nil {
				return
			}
			if nested.Type() == "formal_parameters" {
				for i := 0; i < int(nested.NamedChildCount()); i++ {
					bindPattern(nested.NamedChild(i).ChildByFieldName("pattern"))
				}
				return
			}
			bindPattern(nested)
		case "function_expression", "function_declaration", "method_definition",
			"generator_function", "generator_function_declaration":
			if nested := n.ChildByFieldName("parameters"); nested != nil {
				for i := 0; i < int(nested.NamedChildCount()); i++ {
					bindPattern(nested.NamedChild(i).ChildByFieldName("pattern"))
				}
			}
		}
	}
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "lexical_declaration", "variable_declaration":
			for i := 0; i < int(n.NamedChildCount()); i++ {
				decl := n.NamedChild(i)
				if decl.Type() == "variable_declarator" {
					bindPattern(decl.ChildByFieldName("name"))
				}
			}
		case "function_declaration", "class_declaration":
			if nameNode := n.ChildByFieldName("name"); nameNode != nil {
				names[nodeText(nameNode, source)] = true
			}
		case "for_in_statement":
			bindPattern(n.ChildByFieldName("left"))
		case "catch_clause":
			bindPattern(n.ChildByFieldName("parameter"))
		}
		bindNestedParams(n)
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(body)
	return names
}

// extractCalls walks a function/method/arrow body for call_expression sites and
// returns the entity IDs of the callees it can confirm, deduped in encounter
// order. lang is the domain used only for edges pointing back into the file
// being resolved (a same-file function, a this.-method call); scope is the
// enclosing class chain and classMethods the current class's method set (both
// nil outside a class), used to resolve this.-calls. params is the enclosing
// function-like node's own parameter node (nil when it has none), used with
// body to build the local-shadow guard (localValueNames) before any call site
// is resolved.
func (p *Parser) extractCalls(params, body *sitter.Node, source []byte, filePath, lang string, scope []string, classMethods map[string]bool) []string {
	if body == nil {
		return nil
	}
	locals := localValueNames(params, body, source)
	var calls []string
	seen := make(map[string]bool)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "call_expression" {
			if id := p.callTargetID(n.ChildByFieldName("function"), body, source, filePath, lang, scope, classMethods, locals); id != "" && !seen[id] {
				seen[id] = true
				calls = append(calls, id)
			}
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(body)
	return calls
}

// callTargetID resolves one call_expression's `function` node to a callee
// entity ID, or "" when the target cannot be confirmed (inert). Anything that
// is not a bare identifier or a one-level member-expression receiver
// (`this.m()` / `ns.f()`) stays inert: a property chain (`a.b.c()`), a computed
// callee (`obj[key]()`), and a call/paren-wrapped receiver would all need
// expression typing to resolve safely, and a guess there is exactly the wrong
// edge the design forbids. locals (localValueNames) gates BOTH the bare-
// identifier and the namespace-head branches: a name shadowed by a parameter
// or local declaration is a call through that VALUE, not a reference to any
// module-level/imported definition of the same name, so it must not resolve —
// not even to an "external:" marker.
func (p *Parser) callTargetID(fn, bodyRoot *sitter.Node, source []byte, filePath, lang string, scope []string, classMethods, locals map[string]bool) string {
	if fn == nil {
		return ""
	}
	switch fn.Type() {
	case "identifier":
		name := nodeText(fn, source)
		if locals[name] {
			return "" // shadowed by a parameter/local — a function-VALUE call, not a definition reference
		}
		return p.callNameToEntityID(name, filePath, lang)
	case "member_expression":
		obj := fn.ChildByFieldName("object")
		prop := fn.ChildByFieldName("property")
		if obj == nil || prop == nil {
			return ""
		}
		method := nodeText(prop, source)
		switch obj.Type() {
		case "this":
			if len(scope) == 0 || !classMethods[method] || thisIsRebound(fn, bodyRoot) {
				return ""
			}
			return ast.NewScopedCodeEntity(p.org, lang, p.project, ast.TypeMethod, scope, method, filePath).ID
		case "identifier":
			ns := nodeText(obj, source)
			if locals[ns] {
				return "" // ns is a local value, not a namespace-import binding
			}
			return p.namespaceCalleeID(ns, method, filePath)
		default:
			return "" // property chain (a.b.c(), this.x.y()) — inert
		}
	default:
		return "" // computed callee, call/paren-chain receiver, etc. — inert
	}
}

// callNameToEntityID resolves a bare `name()` call. A local top-level
// definition shadows an import of the same name (mirrors python/calls.go's
// callNameToEntityID's priority — kept for parity even though ordinary TS/JS
// syntax forbids the same-name collision this guards against; tree-sitter
// parses it without complaint regardless).
func (p *Parser) callNameToEntityID(name, filePath, lang string) string {
	if name == "" {
		return ""
	}
	if p.localFuncs[name] {
		return ast.NewCodeEntity(p.org, lang, p.project, ast.TypeFunction, name, filePath).ID
	}
	if spec, ok := p.defaultImports[name]; ok {
		return p.defaultCalleeID(spec, filePath)
	}
	if rel, origin, ok := p.resolveTSImport(name, filePath); ok {
		// internalName may differ from origin: an aliased export-list entry
		// (`export { helper as h }`) is imported by its public name ("h" here,
		// captured as origin), but the entity ID must use the definition's own
		// REAL name ("helper") — see moduleCallInfo's doc comment.
		if internalName, confirmed := p.moduleInfo(rel).funcs[origin]; confirmed {
			return ast.NewCodeEntity(p.org, p.detectLanguage(rel), p.project, ast.TypeFunction, internalName, rel).ID
		}
		return "" // resolved module, but origin isn't an EXPORTED top-level function → inert
	}
	if b, imported := p.importBindings[name]; imported {
		if strings.HasPrefix(b.spec, ".") {
			// A relative specifier that failed to resolve — outside repoRoot
			// (resolveTSModulePath's escape guard) or simply missing on disk —
			// names no real package, so it must stay fully inert: never an
			// "external:" marker, which would misreport a local (if broken or
			// out-of-root) reference as a third-party dependency.
			return ""
		}
		// Module-qualified by the binding's ORIGIN, not the local alias: for
		// `import { transform as t } from 'lodash'`, "t" names nothing real in
		// lodash — "transform" does. This was the review finding: the old
		// `"external:" + name` used the caller-chosen local alias verbatim.
		return "external:" + b.spec + "." + b.origin
	}
	return ""
}

// namespaceCalleeID resolves `ns.f()` through a namespace import binding
// (`import * as ns from '...'`). ns must actually be bound by a namespace
// import — a plain member access on some other identifier (`obj.method()`) is
// not a namespace import and stays inert here, falling through to nothing.
func (p *Parser) namespaceCalleeID(ns, method, filePath string) string {
	spec, ok := p.namespaceImports[ns]
	if !ok {
		return ""
	}
	rel, ok := p.resolveTSModulePath(spec, filePath)
	if !ok {
		if strings.HasPrefix(spec, ".") {
			// A relative specifier that failed to resolve — outside repoRoot or
			// simply missing — names no real package: inert, never "external:"
			// (mirrors callNameToEntityID's identical guard).
			return ""
		}
		// Module-qualified by the SPEC, not `ns` (the caller's own local alias
		// for the whole namespace) — consistent with callNameToEntityID's
		// origin-based marker: the marker should name the real package, not an
		// arbitrary local binding name.
		return "external:" + spec + "." + method
	}
	// internalName may differ from method for the same reason as
	// callNameToEntityID's named-import path: an aliased export-list entry
	// exports under a public name that isn't the definition's own.
	if internalName, confirmed := p.moduleInfo(rel).funcs[method]; confirmed {
		return ast.NewCodeEntity(p.org, p.detectLanguage(rel), p.project, ast.TypeFunction, internalName, rel).ID
	}
	return "" // resolved module, but method isn't an EXPORTED top-level function → inert
}

// defaultCalleeID resolves a default-import call (`import Def from '...'; Def()`)
// against the target module's OWN declared default export, never against a
// same-named local declaration: the local binding name (`Def`) is caller-chosen
// and carries no information about what the module actually exports.
func (p *Parser) defaultCalleeID(spec, filePath string) string {
	rel, ok := p.resolveTSModulePath(spec, filePath)
	if !ok {
		if strings.HasPrefix(spec, ".") {
			return "" // relative specifier that failed to resolve — inert, not external
		}
		return "external:" + spec
	}
	info := p.moduleInfo(rel)
	// allDefs, not funcs: naming a local identifier as the default export IS
	// the export act (`export default foo;`), so foo need not ALSO be a
	// separate named export to confirm — see moduleCallInfo's doc comment.
	if !info.hasDefault || !info.allDefs[info.defaultName] {
		return "" // no default export, or it isn't a real in-tree function → inert
	}
	return ast.NewCodeEntity(p.org, p.detectLanguage(rel), p.project, ast.TypeFunction, info.defaultName, rel).ID
}

// PrepareCallResolution refreshes the per-file import/function-binding state
// ExtractCalls resolves against, from an already-parsed root and its content.
// Exported so the Svelte parser — which parses its script block as its own
// tree-sitter tree, separately from any whole-file ParseFile call — can drive
// the identical resolution pass (design D3) instead of reimplementing it.
func (p *Parser) PrepareCallResolution(root *sitter.Node, content []byte) {
	p.importBindings = extractImportBindings(root, content)
	p.localFuncs = collectModuleFuncNames(root, content)
	p.namespaceImports = extractNamespaceImports(root, content)
	p.defaultImports = extractDefaultImports(root, content)
	p.moduleInfoMemo = make(map[string]moduleCallInfo)
}

// ExtractCalls resolves body's call_expression sites to callee entity IDs; see
// extractCalls. params is the enclosing function-like node's own parameter
// node (nil when it has none) — required so the local-shadow guard
// (localValueNames) sees the SAME parameters the caller built the entity's own
// signature from, not an empty set. lang is the domain used only for edges
// that point back into the file being resolved — a plain .ts/.js caller passes
// its own detected language, while the Svelte parser passes "svelte" so a
// script-block call's edge byte-matches the component's own entity-ID domain.
// Cross-module targets are unaffected by lang: they always resolve against the
// TARGET file's own extension (never a .svelte file — components are not
// import()-able as named function modules), so cross-module edges are correct
// regardless of caller.
func (p *Parser) ExtractCalls(params, body *sitter.Node, content []byte, filePath, lang string, scope []string, classMethods map[string]bool) []string {
	return p.extractCalls(params, body, content, filePath, lang, scope, classMethods)
}
