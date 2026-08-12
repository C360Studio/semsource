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
// import-bound call target: its top-level function names (function_declaration
// or arrow-function declarator) and, if present, the name its default export
// points at. Computed once per relPath and reused for every call site that
// references it — one parse serves named-import, namespace-import, and
// default-import resolution alike.
type moduleCallInfo struct {
	funcs       map[string]bool
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
	info := moduleCallInfo{funcs: map[string]bool{}}
	if content, err := os.ReadFile(filepath.Join(p.repoRoot, relPath)); err == nil {
		mp := sitter.NewParser()
		mp.SetLanguage(p.getTreeSitterLanguage(relPath))
		if tree, terr := mp.ParseCtx(context.Background(), nil, content); terr == nil {
			info.funcs = collectModuleFuncNames(tree.RootNode(), content)
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
// declarators, exported or not (an import can only bind an actually-exported
// name, so confirming presence — not export status — is the fail-inert check).
// Used both for the file being parsed (same-file bare-call resolution) and, via
// moduleInfo, to confirm an imported callee is a function in its own module.
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

// findDefaultExportFunc reports the name a module's default export resolves
// to, when that name can be determined without inference: `export default
// function foo(){}` names foo directly; `export default foo;` names the
// identifier it references (confirmed as an actual top-level function by the
// caller via moduleInfo.funcs, since the identifier could equally name a class
// or const). An anonymous default export (`export default function(){}`,
// `export default () => {}`, `export default {...}`) has no name to point an
// edge at and reports ok=false.
func findDefaultExportFunc(root *sitter.Node, source []byte) (name string, ok bool) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		node := root.NamedChild(i)
		if node.Type() != "export_statement" {
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

// extractCalls walks a function/method/arrow body for call_expression sites and
// returns the entity IDs of the callees it can confirm, deduped in encounter
// order. lang is the domain used only for edges pointing back into the file
// being resolved (a same-file function, a this.-method call); scope is the
// enclosing class chain and classMethods the current class's method set (both
// nil outside a class), used to resolve this.-calls.
func (p *Parser) extractCalls(body *sitter.Node, source []byte, filePath, lang string, scope []string, classMethods map[string]bool) []string {
	if body == nil {
		return nil
	}
	var calls []string
	seen := make(map[string]bool)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "call_expression" {
			if id := p.callTargetID(n.ChildByFieldName("function"), body, source, filePath, lang, scope, classMethods); id != "" && !seen[id] {
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
// edge the design forbids.
func (p *Parser) callTargetID(fn, bodyRoot *sitter.Node, source []byte, filePath, lang string, scope []string, classMethods map[string]bool) string {
	if fn == nil {
		return ""
	}
	switch fn.Type() {
	case "identifier":
		return p.callNameToEntityID(nodeText(fn, source), filePath, lang)
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
			return p.namespaceCalleeID(nodeText(obj, source), method, filePath)
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
		if p.moduleInfo(rel).funcs[origin] {
			return ast.NewCodeEntity(p.org, p.detectLanguage(rel), p.project, ast.TypeFunction, origin, rel).ID
		}
		return "" // resolved module, but origin isn't a top-level function → inert
	}
	if _, imported := p.importBindings[name]; imported {
		return "external:" + name // bare specifier or unresolved relative import
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
		return "external:" + ns + "." + method
	}
	if p.moduleInfo(rel).funcs[method] {
		return ast.NewCodeEntity(p.org, p.detectLanguage(rel), p.project, ast.TypeFunction, method, rel).ID
	}
	return "" // resolved module, but method isn't a top-level function → inert
}

// defaultCalleeID resolves a default-import call (`import Def from '...'; Def()`)
// against the target module's OWN declared default export, never against a
// same-named local declaration: the local binding name (`Def`) is caller-chosen
// and carries no information about what the module actually exports.
func (p *Parser) defaultCalleeID(spec, filePath string) string {
	rel, ok := p.resolveTSModulePath(spec, filePath)
	if !ok {
		return "external:" + spec
	}
	info := p.moduleInfo(rel)
	if !info.hasDefault || !info.funcs[info.defaultName] {
		return "" // no default export, or it isn't a named in-tree function → inert
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
// extractCalls. lang is the domain used only for edges that point back into the
// file being resolved — a plain .ts/.js caller passes its own detected
// language, while the Svelte parser passes "svelte" so a script-block call's
// edge byte-matches the component's own entity-ID domain. Cross-module targets
// are unaffected by lang: they always resolve against the TARGET file's own
// extension (never a .svelte file — components are not import()-able as named
// function modules), so cross-module edges are correct regardless of caller.
func (p *Parser) ExtractCalls(body *sitter.Node, content []byte, filePath, lang string, scope []string, classMethods map[string]bool) []string {
	return p.extractCalls(body, content, filePath, lang, scope, classMethods)
}
