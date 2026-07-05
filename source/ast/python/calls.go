package python

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/c360studio/semsource/source/ast"
)

// Call-graph extraction (task #45, code-call-graph). Reuses the task #44 import
// resolver (lookupBinding + moduleToRelPath) to point a call at the entity ID of
// its callee's DEFINITION, so `code.relationship.calls` edges connect instead of
// dangling. Only unambiguous shapes resolve; everything else stays inert.

// extractLocalFunctions pre-scans a module's top-level function definitions into a
// name set, so a bare `foo()` call can be distinguished from a builtin or a class
// instantiation: only a name defined here (or imported) becomes a call edge.
func extractLocalFunctions(root *sitter.Node, content []byte) map[string]bool {
	funcs := make(map[string]bool)
	add := func(fn *sitter.Node) {
		if nameNode := fn.ChildByFieldName("name"); nameNode != nil {
			funcs[string(content[nameNode.StartByte():nameNode.EndByte()])] = true
		}
	}
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		switch child.Type() {
		case "function_definition":
			add(child)
		case "decorated_definition":
			for j := 0; j < int(child.NamedChildCount()); j++ {
				if child.NamedChild(j).Type() == "function_definition" {
					add(child.NamedChild(j))
				}
			}
		}
	}
	return funcs
}

// extractCalls walks a function/method body for call sites and returns the entity
// IDs of the callees it can resolve, deduped. Unresolvable calls — builtins, bare
// undefined names, class instantiations, and attribute calls on a receiver other
// than self/cls — emit nothing (inert, never a wrong edge). scope is the enclosing
// class chain (for self/cls method resolution); nil for module-level functions.
func (p *Parser) extractCalls(body *sitter.Node, content []byte, filePath string, scope []string) []string {
	if body == nil {
		return nil
	}
	var calls []string
	seen := make(map[string]bool)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "call" {
			if id := p.callTargetID(n.ChildByFieldName("function"), content, filePath, scope); id != "" && !seen[id] {
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
// it cannot be resolved to an in-tree definition or a known external.
func (p *Parser) callTargetID(fn *sitter.Node, content []byte, filePath string, scope []string) string {
	if fn == nil {
		return ""
	}
	switch fn.Type() {
	case "identifier":
		return p.callNameToEntityID(string(content[fn.StartByte():fn.EndByte()]), filePath)
	case "attribute":
		obj := fn.ChildByFieldName("object")
		attr := fn.ChildByFieldName("attribute")
		if obj == nil || attr == nil {
			return ""
		}
		objText := string(content[obj.StartByte():obj.EndByte()])
		method := string(content[attr.StartByte():attr.EndByte()])
		// self.method() / cls.method() — a sibling method in the current class.
		if (objText == "self" || objText == "cls") && len(scope) > 0 {
			return ast.NewScopedCodeEntity(p.org, "python", p.project, ast.TypeMethod, scope, method, filePath).ID
		}
		// module.func() where the receiver (or its head) is an imported module/alias.
		dotted := objText + "." + method
		if mod, origin, level, ok := lookupBinding(dotted, p.imports); ok && origin != "" {
			if defRel, ok2 := p.moduleToRelPath(mod, filePath, level); ok2 {
				return ast.NewCodeEntity(p.org, "python", p.project, ast.TypeFunction, origin, defRel).ID
			}
			return "external:" + dotted
		}
		return ""
	}
	return ""
}

// callNameToEntityID resolves a bare `name()` call: a local module-level function
// resolves to its own entity ID; an imported name resolves to the callee's
// definition in the imported module's file (in-tree) or an external: marker
// (out-of-tree); anything else — a builtin, an instantiation, an unbound name —
// is inert.
func (p *Parser) callNameToEntityID(name, filePath string) string {
	if name == "" {
		return ""
	}
	// A locally-defined function shadows an import of the same name.
	if p.localFuncs[name] {
		return ast.NewCodeEntity(p.org, "python", p.project, ast.TypeFunction, name, filePath).ID
	}
	if mod, origin, level, ok := lookupBinding(name, p.imports); ok {
		if origin != "" {
			if defRel, ok2 := p.moduleToRelPath(mod, filePath, level); ok2 {
				return ast.NewCodeEntity(p.org, "python", p.project, ast.TypeFunction, origin, defRel).ID
			}
		}
		return "external:" + name
	}
	return ""
}
