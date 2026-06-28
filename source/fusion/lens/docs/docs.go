// Package docs is the fusion docs lens: it serves document content through the
// same fusion engine as code, proving fusion is a domain-general primitive
// (ADR-0004). Unlike the code lens it needs no worktree root — a document's body
// lives inline in the graph (source.doc.content), so Hydrate reads a triple, not
// a file. doc-source emits flat documents today (no section/link edges), so
// Edges is empty; section/link edges plug in unchanged when doc-source emits them.
package docs

import (
	"context"
	"path"
	"strings"

	"github.com/c360studio/semsource/source/fusion"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// Lens implements fusion.Lens for documents. It is stateless — the body comes
// from the graph, not the filesystem.
type Lens struct{}

// New creates a docs lens.
func New() *Lens { return &Lens{} }

// Name identifies the lens.
func (*Lens) Name() string { return "docs" }

// docExtensions mark a single-token query as a document path.
var docExtensions = map[string]bool{".md": true, ".txt": true, ".rst": true, ".adoc": true}

// ResolveMode routes a path-like query (slash, or a bare doc filename) to prefix
// and everything else to semantic — documents are found by meaning over their
// content, not by symbol name.
func (*Lens) ResolveMode(query string) fusion.ResolveMode {
	q := strings.TrimSpace(query)
	if strings.ContainsAny(q, " \t\n") {
		return fusion.ResolveSemantic
	}
	if strings.ContainsAny(q, "/\\") || docExtensions[strings.ToLower(path.Ext(q))] {
		return fusion.ResolvePrefix
	}
	return fusion.ResolveSemantic
}

// Edges is empty: doc-source emits flat documents today. Section (source.doc.section)
// and cross-doc link edges plug in here, unchanged, once doc-source emits them.
func (*Lens) Edges() []fusion.EdgeSpec { return nil }

// Label is the document title, read from the canonical dc.terms.title; it falls
// back to the summary slot for entities emitted before doc-source carried a
// title predicate.
func (*Lens) Label(e *fusion.Entity) string {
	if t := e.First(source.DcTitle); t != "" {
		return t
	}
	return e.First(source.DocSummary)
}

// Kind is the document type ("document").
func (*Lens) Kind(e *fusion.Entity) string { return e.First(source.DocType) }

// Location is the document's file path (no line range — Fragment carries a
// section anchor once sections are emitted).
func (*Lens) Location(e *fusion.Entity) fusion.Locator {
	return fusion.Locator{Path: e.First(source.DocFilePath)}
}

// Hydrate returns the document body from the graph. It is "" when the content was
// offloaded to ObjectStore (no inline triple) — a body is a best-effort facet.
func (*Lens) Hydrate(_ context.Context, e *fusion.Entity) (string, error) {
	return e.First(source.DocContent), nil
}
