// Package code is the fusion code lens: it walks call and containment edges and
// hydrates verbatim source, turning the generic pkg/fusion engine into
// code_context. Since ADR-062 increment 4 the lens is stateless — bodies are
// offloaded to a storage.Store at ingest and Hydrate returns the entity's
// StorageReference HANDLE (read from triples), never a worktree read. The engine
// dereferences the handle, so a remote caller of a standalone service (ADR-0006)
// gets bodies without access to the ingesting host's disk.
package code

import (
	"context"
	"path"
	"strings"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/fusion"

	"github.com/c360studio/semsource/source/ast"
)

// Lens implements fusion.Lens for code. It is stateless: every domain fact —
// including where the verbatim body lives — comes off the entity's triples.
type Lens struct{}

// New creates a code lens.
func New() *Lens { return &Lens{} }

// Name identifies the lens.
func (*Lens) Name() string { return "code" }

// ResolveMode routes anything with whitespace to nl (semantic), a single
// path-like token to prefix, and a single identifier to symbol. The nl check is
// first so a question that merely ends in a code-extension word (e.g. "where is
// foo.go") is not misrouted to a path lookup.
func (*Lens) ResolveMode(query string) fusion.ResolveMode {
	q := strings.TrimSpace(query)
	if strings.ContainsAny(q, " \t\n") {
		return fusion.ResolveModeNL
	}
	if strings.ContainsAny(q, "/\\") || codeExtensions[strings.ToLower(path.Ext(q))] {
		return fusion.ResolveModePrefix
	}
	return fusion.ResolveModeSymbol
}

// Edges are the code relationships to expand: calls, containment, and the type
// dependency edges (extends/implements/references). computeImpact BFSes the
// incoming (reverse) direction of every predicate here, so adding the dependency
// edges lets code_impact and the relations facet surface a type's dependents —
// subclasses (extended_by), implementers (implemented_by), referrers
// (referenced_by) — instead of only its structural container chain. Before this,
// a class's impact held only its containers (file/folder/repo), never dependents.
//
// Two caveats:
//   - Impact also walks incoming CodeContains, so the impact *count* still mixes
//     containment ancestry with real dependents — it is not a pure dependency
//     closure. Separating impact edges from relation edges needs a fusion-engine
//     change (this one list drives impact, paths, AND relations) — semstreams ask.
//   - Dependency edges only resolve where the parser builds matching target ids.
//     That holds for Python (task #43); Java/TS/Go reference-id parity is a
//     separate per-language follow-up, so their targets may still dangle (inert —
//     the engine drops unresolved targets, so no wrong output, just no dependents).
func (*Lens) Edges() []fusion.EdgeSpec {
	return []fusion.EdgeSpec{
		{Predicate: ast.CodeCalls, OutgoingRole: "callee", IncomingRole: "caller"},
		{Predicate: ast.CodeContains, OutgoingRole: "contains", IncomingRole: "container"},
		{Predicate: ast.CodeExtends, OutgoingRole: "extends", IncomingRole: "extended_by"},
		{Predicate: ast.CodeImplements, OutgoingRole: "implements", IncomingRole: "implemented_by"},
		{Predicate: ast.CodeReferences, OutgoingRole: "references", IncomingRole: "referenced_by"},
	}
}

// Label is the symbol name.
func (*Lens) Label(e *fusion.Entity) string { return e.First(ast.DcTitle) }

// Kind is the code entity type (function, method, struct, …).
func (*Lens) Kind(e *fusion.Entity) string { return e.First(ast.CodeType) }

// Location is the file path and line range (citation/display only — the body
// rides the handle, pre-sliced, so the engine does no line math).
func (*Lens) Location(e *fusion.Entity) fusion.Locator {
	return fusion.Locator{
		Path:  e.First(ast.CodePath),
		Lines: [2]int{e.FirstInt(ast.CodeStartLine), e.FirstInt(ast.CodeEndLine)},
	}
}

// Hydrate returns the handle to the entity's verbatim body, read from the body
// triples the producer stamped at ingest (CodeBodyStore + CodeBodyKey). It is
// (nil, nil) when no body was offloaded — a body is a best-effort facet, and the
// engine degrades the node rather than failing.
func (*Lens) Hydrate(_ context.Context, e *fusion.Entity) (*message.StorageReference, error) {
	instance := e.First(ast.CodeBodyStore)
	key := e.First(ast.CodeBodyKey)
	if instance == "" || key == "" {
		return nil, nil
	}
	return &message.StorageReference{
		StorageInstance: instance,
		Key:             key,
		ContentType:     "text/plain; charset=utf-8",
	}, nil
}

// codeExtensions marks a query as a file path.
var codeExtensions = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".mjs": true, ".cjs": true, ".svelte": true, ".java": true, ".py": true,
}
