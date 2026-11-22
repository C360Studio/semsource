package java

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/source/ast"
)

// Java overload identity (fix-ast-id-collisions, design decisions 2 and 3).
//
// A name plus a scope is not an identity in Java: `visit(Text)` and
// `visit(Count)`, or the constructor `Foo(int)` and the method `void Foo(int)`,
// would otherwise share one entity ID and the graph would merge them. Every
// method and constructor therefore carries `-<6 hex>`, the first three bytes of
// SHA-256 over its erased signature. The discriminator goes on EVERY callable,
// never only on colliding ones, so adding an overload never re-identifies its
// siblings; parameter names are not part of the input, so renaming one keeps
// the ID.

// callableSig is one method or constructor's overload identity.
type callableSig struct {
	// key is the hash input: kind plus erased parameter types. Two declarations
	// with an equal key in one hierarchy are an override, not an overload.
	key     string
	suffix  string // "-" + 6 hex chars of SHA-256(key)
	params  int    // declared parameter count, excluding a receiver parameter
	varargs bool   // last parameter is `T...`
}

// accepts reports whether a call with argc arguments can bind this callable by
// arity alone. A varargs callable with k parameters accepts k-1 or more.
func (s callableSig) accepts(argc int) bool {
	if s.varargs {
		return argc >= s.params-1
	}
	return argc == s.params
}

// callableSignature computes a declaration's overload identity. typeVars maps
// the enclosing types' type variables to their first bound; the declaration's
// own type parameters shadow them.
func callableSignature(decl *sitter.Node, content []byte, ctor bool, typeVars map[string]string) callableSig {
	vars := typeVarBounds(decl.ChildByFieldName("type_parameters"), content, typeVars)
	var types []string
	sig := callableSig{}
	if params := decl.ChildByFieldName("parameters"); params != nil {
		for i := 0; i < int(params.NamedChildCount()); i++ {
			param := params.NamedChild(i)
			switch param.Type() {
			case "formal_parameter":
				if isReceiverParameter(param, content) {
					continue
				}
				text := sigTypeText(param.ChildByFieldName("type"), content, vars)
				// C-style `int x[]` puts the dimensions on the parameter.
				if dims := param.ChildByFieldName("dimensions"); dims != nil {
					text += strippedText(dims, content)
				}
				types = append(types, text)
			case "spread_parameter":
				types = append(types, sigTypeText(spreadElementType(param), content, vars)+"[]")
				sig.varargs = true
			}
		}
	}
	kind := "method"
	if ctor {
		kind = "ctor"
	}
	sig.params = len(types)
	sig.key = kind + "(" + strings.Join(types, ",") + ")"
	sum := sha256.Sum256([]byte(sig.key))
	sig.suffix = "-" + hex.EncodeToString(sum[:3])
	return sig
}

// callableID builds a method or constructor entity ID: the scoped instance the
// other entity kinds use, plus the overload discriminator. The definition and
// every call edge build it here, so an edge byte-matches its target.
func (p *Parser) callableID(scope []string, name, relPath, suffix string) string {
	instance := ast.BuildScopedInstanceID(relPath, scope, name, ast.TypeMethod) + suffix
	return p.authority.Build(entityid.SystemSlug(p.project), "java", string(ast.TypeMethod), instance)
}

// isReceiverParameter reports an explicit receiver parameter (`Foo this`),
// which is not part of the signature and takes no argument.
func isReceiverParameter(param *sitter.Node, content []byte) bool {
	name := param.ChildByFieldName("name")
	return name != nil && string(content[name.StartByte():name.EndByte()]) == "this"
}

// spreadElementType returns a varargs parameter's element type: its first named
// child that is not the modifiers or the declarator.
func spreadElementType(param *sitter.Node) *sitter.Node {
	for i := 0; i < int(param.NamedChildCount()); i++ {
		switch child := param.NamedChild(i); child.Type() {
		case "modifiers", "variable_declarator":
		default:
			return child
		}
	}
	return nil
}

// typeVarBounds extends outer with the type variables a type_parameters node
// declares, each mapped to its first bound's erased text (`Object` when
// unbounded). Returns outer itself when node declares nothing.
func typeVarBounds(node *sitter.Node, content []byte, outer map[string]string) map[string]string {
	if node == nil || node.NamedChildCount() == 0 {
		return outer
	}
	vars := make(map[string]string, len(outer)+int(node.NamedChildCount()))
	for k, v := range outer {
		vars[k] = v
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		tp := node.NamedChild(i)
		if tp.Type() != "type_parameter" {
			continue
		}
		name, bound := "", "Object"
		for j := 0; j < int(tp.NamedChildCount()); j++ {
			child := tp.NamedChild(j)
			switch child.Type() {
			case "type_identifier":
				name = string(content[child.StartByte():child.EndByte()])
			case "type_bound":
				if child.NamedChildCount() > 0 {
					// Resolved against the variables declared so far, so
					// `<T, U extends T>` erases U to T's bound.
					bound = sigTypeText(child.NamedChild(0), content, vars)
				}
			}
		}
		if name != "" {
			vars[name] = bound
		}
	}
	return vars
}

// sigTypeText renders a declared type for the overload hash: type arguments and
// annotations removed, qualifiers as written, array dimensions kept, and a bare
// type variable replaced by its first bound.
func sigTypeText(n *sitter.Node, content []byte, typeVars map[string]string) string {
	if n == nil {
		return ""
	}
	switch n.Type() {
	case "type_identifier":
		name := string(content[n.StartByte():n.EndByte()])
		if bound, ok := typeVars[name]; ok {
			return bound
		}
		return name
	case "array_type":
		return sigTypeText(n.ChildByFieldName("element"), content, typeVars) +
			strippedText(n.ChildByFieldName("dimensions"), content)
	case "generic_type":
		// The base type is the first named child; the type arguments are erased.
		if n.NamedChildCount() > 0 {
			return sigTypeText(n.NamedChild(0), content, typeVars)
		}
	case "annotated_type":
		// The annotations precede the type, which is the last named child.
		if c := n.NamedChildCount(); c > 0 {
			return sigTypeText(n.NamedChild(int(c)-1), content, typeVars)
		}
	}
	return strippedText(n, content)
}

// strippedText concatenates a node's leaf tokens, dropping type arguments,
// annotations and comments, so whitespace and generics never reach the hash.
func strippedText(n *sitter.Node, content []byte) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Type() {
		case "type_arguments", "annotation", "marker_annotation", "line_comment", "block_comment":
			return
		}
		if n.ChildCount() == 0 {
			b.WriteString(strings.TrimSpace(string(content[n.StartByte():n.EndByte()])))
			return
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(n)
	return b.String()
}

// argCount counts a call site's arguments: the named children of its
// argument_list, comments excluded. Returns -1 when there is no list.
func argCount(call *sitter.Node) int {
	args := call.ChildByFieldName("arguments")
	if args == nil {
		return -1
	}
	n := 0
	for i := 0; i < int(args.NamedChildCount()); i++ {
		switch args.NamedChild(i).Type() {
		case "line_comment", "block_comment":
		default:
			n++
		}
	}
	return n
}
