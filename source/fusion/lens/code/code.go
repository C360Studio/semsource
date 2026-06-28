// Package code is the fusion code lens: it walks call and containment edges and
// hydrates verbatim source from the worktree, turning the generic fusion engine
// into code_context. Construct one per worktree with New(root).
package code

import (
	"context"
	"path"
	"strings"

	"github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/fusion"
)

// Lens implements fusion.Lens for code. It is request-scoped: New is given the
// worktree root so Hydrate can read source files by their relative path.
type Lens struct {
	root string
	src  *sourceReader
}

// New creates a code lens rooted at the given worktree absolute path.
func New(root string) *Lens {
	return &Lens{root: root, src: newSourceReader(root)}
}

// Name identifies the lens.
func (*Lens) Name() string { return "code" }

// ResolveMode routes anything with whitespace to semantic (NL), a single
// path-like token to prefix, and a single identifier to symbol. The NL check is
// first so a question that merely ends in a code-extension word (e.g. "where is
// foo.go") is not misrouted to a path lookup.
func (*Lens) ResolveMode(query string) fusion.ResolveMode {
	q := strings.TrimSpace(query)
	if strings.ContainsAny(q, " \t\n") {
		return fusion.ResolveSemantic
	}
	if strings.ContainsAny(q, "/\\") || codeExtensions[strings.ToLower(path.Ext(q))] {
		return fusion.ResolvePrefix
	}
	return fusion.ResolveSymbol
}

// Edges are the code relationships to expand: calls (callee/caller) and
// containment (contains/container).
func (*Lens) Edges() []fusion.EdgeSpec {
	return []fusion.EdgeSpec{
		{Predicate: ast.CodeCalls, OutgoingRole: "callee", IncomingRole: "caller"},
		{Predicate: ast.CodeContains, OutgoingRole: "contains", IncomingRole: "container"},
	}
}

// Label is the symbol name.
func (*Lens) Label(e *fusion.Entity) string { return e.First(ast.DcTitle) }

// Kind is the code entity type (function, method, struct, …).
func (*Lens) Kind(e *fusion.Entity) string { return e.First(ast.CodeType) }

// Location is the file path and line range.
func (*Lens) Location(e *fusion.Entity) fusion.Locator {
	return fusion.Locator{
		Path:  e.First(ast.CodePath),
		Lines: [2]int{e.FirstInt(ast.CodeStartLine), e.FirstInt(ast.CodeEndLine)},
	}
}

// Hydrate reads the verbatim source for the entity's line range from the
// worktree. A missing path or unreadable file yields "" (no body), never an
// error — a body is a best-effort facet.
func (l *Lens) Hydrate(_ context.Context, e *fusion.Entity) (string, error) {
	p := e.First(ast.CodePath)
	if p == "" {
		return "", nil
	}
	return l.src.extract(p, e.FirstInt(ast.CodeStartLine), e.FirstInt(ast.CodeEndLine)), nil
}

// codeExtensions marks a query as a file path.
var codeExtensions = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".mjs": true, ".cjs": true, ".svelte": true, ".java": true, ".py": true,
}
