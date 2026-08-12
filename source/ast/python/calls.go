package python

import (
	"context"
	"os"
	"path/filepath"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/c360studio/semsource/source/ast"
)

// Call-graph extraction (task #45, code-call-graph). Reuses the task #44 import
// resolver (lookupBinding + moduleToRelPath) to point a call at the entity ID of
// its callee's DEFINITION, so `code.relationship.calls` edges connect instead of
// dangling. Resolution FAILS INERT: an edge is emitted only when the target is
// confirmed — a local module-level function, an imported top-level function (the
// defining module is parsed to confirm the origin is a function, not a class or
// object attribute), or a method that exists on the current class. Everything else
// (builtins, class instantiations, inherited/mixin methods, attribute calls on a
// local variable) emits nothing — never a wrong or phantom edge.
//
// A bare or `obj.method()` call whose head name is bound by the enclosing
// function's OWN parameters or by an assignment/def/for-target/with-as/walrus
// anywhere in its body (localValueNames) is a call through that LOCAL VALUE,
// never a reference to a module-level or imported definition of the same
// name, and is suppressed before either resolution path runs (spec: a
// function-typed parameter — generalized here to any local binding — SHALL
// NOT produce a call edge). The set is FLAT across block depth: Python has no
// block scope, so a name assigned in one branch is genuinely visible for the
// rest of the function — this is not an approximation the way it is for a
// block-scoped language.
//
// Known inert limitations (documented, never wrong — a missing edge, not a bad
// one): a call in a parameter default (`def f(x=g())`) is outside the body walk;
// and `from pkg import sub; sub.f()` resolves against pkg's package file (where f
// is absent → inert) rather than pkg/sub.py. These need submodule probing and are
// deferred.

// extractLocalFunctions collects a module's top-level function definitions into a
// name set. Used both for the file being parsed (to resolve bare local calls) and,
// via moduleFuncs, to confirm an imported callee is a function in its module.
func extractLocalFunctions(root *sitter.Node, content []byte) map[string]bool {
	funcs := make(map[string]bool)
	add := func(fn *sitter.Node) {
		if nameNode := fn.ChildByFieldName("name"); nameNode != nil {
			funcs[nodeText(nameNode, content)] = true
		}
	}
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		switch child.Type() {
		case "function_definition":
			add(child)
		case "decorated_definition":
			if def := findFunctionInDecorated(child); def != nil {
				add(def)
			}
		}
	}
	return funcs
}

// classMethodNames collects the method names defined directly in a class body, so
// a `self.m()` / `cls.m()` call resolves only when m is actually a method of this
// class (an inherited/mixin method or a typo stays inert).
func classMethodNames(classNode *sitter.Node, content []byte) map[string]bool {
	names := make(map[string]bool)
	body := classNode.ChildByFieldName("body")
	if body == nil {
		return names
	}
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		var def *sitter.Node
		switch child.Type() {
		case "function_definition":
			def = child
		case "decorated_definition":
			def = findFunctionInDecorated(child)
		}
		if def != nil {
			if nameNode := def.ChildByFieldName("name"); nameNode != nil {
				names[nodeText(nameNode, content)] = true
			}
		}
	}
	return names
}

// findFunctionInDecorated returns the function_definition wrapped by a
// decorated_definition, or nil.
func findFunctionInDecorated(node *sitter.Node) *sitter.Node {
	for j := 0; j < int(node.NamedChildCount()); j++ {
		if node.NamedChild(j).Type() == "function_definition" {
			return node.NamedChild(j)
		}
	}
	return nil
}

// moduleFuncs returns the top-level function names of an in-tree module file,
// parsing it once per ParseFile (memoized by relPath). Used to confirm an imported
// callee is a function before emitting a call edge.
func (p *Parser) moduleFuncs(relPath string) map[string]bool {
	if cached, ok := p.moduleFuncsMemo[relPath]; ok {
		return cached
	}
	funcs := make(map[string]bool)
	if content, err := os.ReadFile(filepath.Join(p.repoRoot, relPath)); err == nil {
		mp := sitter.NewParser()
		mp.SetLanguage(python.GetLanguage())
		if tree, terr := mp.ParseCtx(context.Background(), nil, content); terr == nil {
			funcs = extractLocalFunctions(tree.RootNode(), content)
			tree.Close()
		}
	}
	if p.moduleFuncsMemo == nil {
		p.moduleFuncsMemo = make(map[string]map[string]bool)
	}
	p.moduleFuncsMemo[relPath] = funcs
	return funcs
}

// localValueNames collects every name a function's OWN parameters and body
// bind locally: parameters (plain, defaulted, typed, *args, **kwargs — via
// collectPyParamNames), every assignment target (including tuple/list
// unpacking), for-loop targets, with-statement `as` targets, walrus (`:=`)
// targets, and nested def/class names. A bare or `obj.method()` call whose
// head name is in this set is a call through that LOCAL VALUE, not a
// reference to the module-level/imported definition of the same name (mirrors
// ts/calls.go's localValueNames and c/calls.go's localValueNames — the same
// rule applied to Python's declaration surface).
//
// Deliberately NOT scoped to Python's real block structure: Python has no
// block scope (an if/for/with body's assignments are visible for the rest of
// the enclosing function), so a flat set is not an approximation here the way
// it is for TS/JS — it is the correct scope model.
func localValueNames(fnNode *sitter.Node, content []byte) map[string]bool {
	names := make(map[string]bool)
	if params := fnNode.ChildByFieldName("parameters"); params != nil {
		for i := 0; i < int(params.NamedChildCount()); i++ {
			collectPyParamNames(params.NamedChild(i), content, names)
		}
	}
	body := fnNode.ChildByFieldName("body")
	if body == nil {
		return names
	}
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "assignment":
			if left := n.ChildByFieldName("left"); left != nil {
				collectPyAssignTargets(left, content, names)
			}
		case "for_statement":
			if left := n.ChildByFieldName("left"); left != nil {
				collectPyAssignTargets(left, content, names)
			}
		case "named_expression": // walrus: `(transform := get())`
			if nameNode := n.ChildByFieldName("name"); nameNode != nil {
				names[nodeText(nameNode, content)] = true
			}
		case "as_pattern": // `with open(...) as fh:` / `except E as e:`
			if alias := n.ChildByFieldName("alias"); alias != nil {
				for i := 0; i < int(alias.NamedChildCount()); i++ {
					if alias.NamedChild(i).Type() == "identifier" {
						names[nodeText(alias.NamedChild(i), content)] = true
					}
				}
			}
		case "function_definition", "class_definition":
			if nameNode := n.ChildByFieldName("name"); nameNode != nil {
				names[nodeText(nameNode, content)] = true
			}
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(body)
	return names
}

// collectPyParamNames binds one parameter-list entry, unwrapping the
// default/typed/splat wrappers tree-sitter-python uses around a plain
// identifier: default_parameter/typed_default_parameter/typed_parameter carry
// the identifier under a "name" field (falling back to the first identifier
// child if a grammar revision drops the field, same defensive pattern
// imports.go already uses for import specifiers); list_splat_pattern (*args)
// and dictionary_splat_pattern (**kwargs) wrap it as their one child.
func collectPyParamNames(node *sitter.Node, content []byte, names map[string]bool) {
	switch node.Type() {
	case "identifier":
		names[nodeText(node, content)] = true
	case "default_parameter", "typed_default_parameter", "typed_parameter":
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			collectPyParamNames(nameNode, content, names)
			return
		}
		for i := 0; i < int(node.NamedChildCount()); i++ {
			if node.NamedChild(i).Type() == "identifier" {
				names[nodeText(node.NamedChild(i), content)] = true
				return
			}
		}
	case "list_splat_pattern", "dictionary_splat_pattern":
		if node.NamedChildCount() > 0 {
			collectPyParamNames(node.NamedChild(0), content, names)
		}
	}
}

// collectPyAssignTargets binds an assignment/for-loop target, descending into
// tuple/list unpacking (`a, b = 1, 2`) so every unpacked name is bound, not
// just the first. An attribute or subscript target (`obj.x = 1`, `arr[0] = 1`)
// is deliberately NOT a binding — it mutates something that already exists
// rather than introducing a new local name, so it does not shadow anything.
func collectPyAssignTargets(node *sitter.Node, content []byte, names map[string]bool) {
	switch node.Type() {
	case "identifier":
		names[nodeText(node, content)] = true
	case "pattern_list", "tuple_pattern", "list_pattern":
		for i := 0; i < int(node.NamedChildCount()); i++ {
			collectPyAssignTargets(node.NamedChild(i), content, names)
		}
	}
}

// extractCalls walks a function/method body for call sites and returns the entity
// IDs of the callees it can confirm, deduped. scope is the enclosing class chain
// and classMethods the current class's method set (both empty for module-level
// functions), used to resolve self/cls calls. locals (localValueNames) gates
// bare and obj.method() resolution before either runs — a call through a
// locally-shadowed name is a call through that VALUE, not a definition
// reference.
func (p *Parser) extractCalls(fnNode, body *sitter.Node, content []byte, filePath string, scope []string, classMethods map[string]bool) []string {
	if body == nil {
		return nil
	}
	locals := localValueNames(fnNode, content)
	var calls []string
	seen := make(map[string]bool)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "call" {
			if id := p.callTargetID(n.ChildByFieldName("function"), content, filePath, scope, classMethods, locals); id != "" && !seen[id] {
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

// callTargetID resolves a call's `function` node to a callee entity ID, or "" when
// the target cannot be confirmed (inert). locals (localValueNames) gates both
// the bare-identifier and the obj.method() branches: a shadowed name is a call
// through that VALUE, not a definition reference — not even to an "external:"
// marker.
func (p *Parser) callTargetID(fn *sitter.Node, content []byte, filePath string, scope []string, classMethods, locals map[string]bool) string {
	if fn == nil {
		return ""
	}
	switch fn.Type() {
	case "identifier":
		name := nodeText(fn, content)
		if locals[name] {
			return "" // shadowed by a parameter/local — a call through that VALUE, not a definition reference
		}
		return p.callNameToEntityID(name, filePath)
	case "attribute":
		obj := fn.ChildByFieldName("object")
		attr := fn.ChildByFieldName("attribute")
		if obj == nil || attr == nil {
			return ""
		}
		objText := nodeText(obj, content)
		method := nodeText(attr, content)
		// self.method() / cls.method() — resolve only to a method of THIS class;
		// inherited/mixin methods (defined in another file) stay inert. self/cls
		// are fixed receiver names, not subject to the locals shadow check: a
		// parameter literally named "self" other than the true self parameter
		// would BE the self parameter, so there is no separate shadow to guard.
		if objText == "self" || objText == "cls" {
			if len(scope) > 0 && classMethods[method] {
				return ast.NewScopedCodeEntity(p.org, "python", p.project, ast.TypeMethod, scope, method, filePath).ID
			}
			return ""
		}
		if locals[objText] {
			return "" // objText is a local value, not an imported module/alias
		}
		// module.func() where the receiver head is an imported module/alias.
		return p.resolveImportedCallee(objText+"."+method, filePath)
	}
	return ""
}

// callNameToEntityID resolves a bare `name()` call: a local module-level function
// resolves to its own entity ID; otherwise it is resolved as an imported callee.
func (p *Parser) callNameToEntityID(name, filePath string) string {
	if name == "" {
		return ""
	}
	// A locally-defined function shadows an import of the same name.
	if p.localFuncs[name] {
		return ast.NewCodeEntity(p.org, "python", p.project, ast.TypeFunction, name, filePath).ID
	}
	return p.resolveImportedCallee(name, filePath)
}

// resolveImportedCallee resolves an imported call target (bare `name` or dotted
// `mod.attr`) to the callee's entity ID: a top-level FUNCTION in the resolved
// in-tree module, an `external:` marker for an out-of-tree module, or "" (inert)
// when the module resolves in-tree but does not define the origin as a function
// (a class instantiation, or an attribute access on an imported object). This is
// the fail-inert guard: an in-tree module is parsed and the origin confirmed to be
// a function before a `.function.` edge is fabricated.
func (p *Parser) resolveImportedCallee(key, filePath string) string {
	mod, origin, level, ok := lookupBinding(key, p.imports)
	if !ok || origin == "" {
		return ""
	}
	defRel, inTree := p.moduleToRelPath(mod, filePath, level)
	if !inTree {
		return "external:" + key
	}
	if p.moduleFuncs(defRel)[origin] {
		return ast.NewCodeEntity(p.org, "python", p.project, ast.TypeFunction, origin, defRel).ID
	}
	return "" // resolved module, but origin is not a top-level function → inert
}
