package ast

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// PrecedingJSDoc returns the JSDoc/TSDoc body that documents node, taking
// into account TS-family structural quirks:
//
//   - decorator siblings are skipped past (`@Foo class Bar` style)
//   - an `export_statement` parent wrapper is walked through so that
//     `/** ... */ export class Foo {}` still resolves to the right comment.
//
// Returns "" when no `/** ... */` comment is found before the declaration.
// Tree-sitter TypeScript names the comment node type "comment".
func PrecedingJSDoc(node *sitter.Node, content []byte) string {
	cur := node
	if parent := cur.Parent(); parent != nil && parent.Type() == "export_statement" {
		cur = parent
	}
	for {
		prev := cur.PrevNamedSibling()
		if prev == nil {
			return ""
		}
		if prev.Type() == "decorator" {
			cur = prev
			continue
		}
		if prev.Type() == "comment" {
			text := string(content[prev.StartByte():prev.EndByte()])
			if strings.HasPrefix(text, "/**") {
				return CleanDocCommentBlock(text)
			}
		}
		return ""
	}
}

// RenderTSFunctionSignature builds a "name(params): returnType" signature
// string from a tree-sitter-typescript function/method node, slicing source
// bytes between the name field and the end of the parameter list (and return
// type when present). Body content is excluded. Whitespace is collapsed.
// Returns "" when expected fields are missing.
func RenderTSFunctionSignature(node *sitter.Node, source []byte) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return ""
	}
	end := uint32(0)
	if params := node.ChildByFieldName("parameters"); params != nil {
		end = params.EndByte()
	}
	if returnType := node.ChildByFieldName("return_type"); returnType != nil && returnType.EndByte() > end {
		end = returnType.EndByte()
	}
	if end <= nameNode.StartByte() {
		return ""
	}
	return strings.Join(strings.Fields(string(source[nameNode.StartByte():end])), " ")
}

// RenderTSArrowSignature builds a `name(params): returnType` signature for an
// arrow function bound to an identifier. valueNode must be an `arrow_function`
// node. Returns "" when the expected fields are missing.
func RenderTSArrowSignature(name string, valueNode *sitter.Node, source []byte) string {
	if valueNode == nil || name == "" {
		return ""
	}
	parts := []string{name}
	params := valueNode.ChildByFieldName("parameters")
	if params == nil {
		return ""
	}
	parts = append(parts, string(source[params.StartByte():params.EndByte()]))
	if returnType := valueNode.ChildByFieldName("return_type"); returnType != nil {
		parts = append(parts, string(source[returnType.StartByte():returnType.EndByte()]))
	}
	return strings.Join(strings.Fields(strings.Join(parts, "")), " ")
}

// IsDestructuringPattern reports whether a declarator's name position holds a
// binding pattern rather than a plain identifier.
func IsDestructuringPattern(nameNode *sitter.Node) bool {
	switch nameNode.Type() {
	case "object_pattern", "array_pattern":
		return true
	default:
		return false
	}
}

// CollectPatternBindings walks a JS/TS destructuring pattern and appends the
// nodes that actually introduce names into scope. Shared by the ts and svelte
// parsers, which both reach this grammar.
//
// Only binding positions are descended into, which is what separates a name
// from the syntax around it:
//   - pair_pattern descends into `value` only — in `{key: local}` the key names
//     a field on the source object, while `local` is what gets declared;
//   - assignment patterns descend into `left` only, so a default value's
//     expression (`{a = someCall()}`) contributes no spurious entities;
//   - object_pattern/array_pattern iterate NAMED children, which skips
//     punctuation and array holes for free.
func CollectPatternBindings(node *sitter.Node, out *[]*sitter.Node) {
	switch node.Type() {
	case "identifier", "shorthand_property_identifier_pattern":
		*out = append(*out, node)
	case "pair_pattern":
		descendPatternField(node, "value", out)
	case "object_assignment_pattern", "assignment_pattern":
		descendPatternField(node, "left", out)
	case "object_pattern", "array_pattern", "rest_pattern":
		for i := 0; i < int(node.NamedChildCount()); i++ {
			CollectPatternBindings(node.NamedChild(i), out)
		}
	}
}

// descendPatternField recurses into a named field, falling back to the first
// named child so a grammar revision that drops the field name degrades to
// skipping the odd binding rather than dropping every symbol in the file.
func descendPatternField(node *sitter.Node, field string, out *[]*sitter.Node) {
	if child := node.ChildByFieldName(field); child != nil {
		CollectPatternBindings(child, out)
		return
	}
	if node.NamedChildCount() > 0 {
		CollectPatternBindings(node.NamedChild(0), out)
	}
}
