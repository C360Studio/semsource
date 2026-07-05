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
// Known inert limitations (documented, never wrong — a missing edge, not a bad
// one): a call in a parameter default (`def f(x=g())`) is outside the body walk;
// a bare call shadowed by a nested `def`, and a `self.x()` inside a class nested in
// a method, resolve against the module/outer scope; and `from pkg import sub;
// sub.f()` resolves against pkg's package file (where f is absent → inert) rather
// than pkg/sub.py. These need scope tracking or submodule probing and are deferred.

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

// extractCalls walks a function/method body for call sites and returns the entity
// IDs of the callees it can confirm, deduped. scope is the enclosing class chain
// and classMethods the current class's method set (both empty for module-level
// functions), used to resolve self/cls calls.
func (p *Parser) extractCalls(body *sitter.Node, content []byte, filePath string, scope []string, classMethods map[string]bool) []string {
	if body == nil {
		return nil
	}
	var calls []string
	seen := make(map[string]bool)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "call" {
			if id := p.callTargetID(n.ChildByFieldName("function"), content, filePath, scope, classMethods); id != "" && !seen[id] {
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
// the target cannot be confirmed (inert).
func (p *Parser) callTargetID(fn *sitter.Node, content []byte, filePath string, scope []string, classMethods map[string]bool) string {
	if fn == nil {
		return ""
	}
	switch fn.Type() {
	case "identifier":
		return p.callNameToEntityID(nodeText(fn, content), filePath)
	case "attribute":
		obj := fn.ChildByFieldName("object")
		attr := fn.ChildByFieldName("attribute")
		if obj == nil || attr == nil {
			return ""
		}
		objText := nodeText(obj, content)
		method := nodeText(attr, content)
		// self.method() / cls.method() — resolve only to a method of THIS class;
		// inherited/mixin methods (defined in another file) stay inert.
		if objText == "self" || objText == "cls" {
			if len(scope) > 0 && classMethods[method] {
				return ast.NewScopedCodeEntity(p.org, "python", p.project, ast.TypeMethod, scope, method, filePath).ID
			}
			return ""
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
