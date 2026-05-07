package ast

import (
	"slices"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// PrecedingDocComment looks at the named sibling immediately preceding node
// and returns its cleaned text if it is a `/** ... */` style block comment
// (Javadoc / JSDoc / TSDoc convention). Returns "" when the previous sibling
// is missing, is not a block comment, or does not start with `/**`.
//
// blockCommentTypes lists the tree-sitter node type names that should be
// treated as block comments for the host language. Tree-sitter Java uses
// "block_comment"; tree-sitter TypeScript/JavaScript uses "comment".
func PrecedingDocComment(node *sitter.Node, content []byte, blockCommentTypes ...string) string {
	if node == nil {
		return ""
	}
	prev := node.PrevNamedSibling()
	if prev == nil {
		return ""
	}
	if !slices.Contains(blockCommentTypes, prev.Type()) {
		return ""
	}
	text := string(content[prev.StartByte():prev.EndByte()])
	if !strings.HasPrefix(text, "/**") {
		return ""
	}
	return CleanDocCommentBlock(text)
}

// CleanDocCommentBlock strips `/**` / `*/` delimiters and per-line leading
// `*` markers from a Javadoc/JSDoc-style block comment, returning the
// natural-language body with surrounding blank lines trimmed. Per-line
// indentation is preserved after the leading `*` so embedded code or `@param`
// alignment remains readable.
func CleanDocCommentBlock(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "/**")
	s = strings.TrimSuffix(s, "*/")

	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		trimmed = strings.TrimPrefix(trimmed, "*")
		trimmed = strings.TrimPrefix(trimmed, " ")
		out = append(out, strings.TrimRight(trimmed, " \t"))
	}

	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// CombineDocComment merges a Javadoc/JSDoc body with auxiliary metadata
// (annotations, modifiers, decorators). Doc text leads, metadata follows
// after a blank line. Either side may be empty.
func CombineDocComment(docBody, metadata string) string {
	docBody = strings.TrimSpace(docBody)
	metadata = strings.TrimSpace(metadata)
	switch {
	case docBody == "" && metadata == "":
		return ""
	case docBody == "":
		return metadata
	case metadata == "":
		return docBody
	default:
		return docBody + "\n\n" + metadata
	}
}
