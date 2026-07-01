// Package docs is the fusion docs lens: it serves documents through the same
// pkg/fusion engine as code, proving fusion is a domain-general primitive
// (ADR-0004). doc-source emits flat documents today (no section/link edges), so
// Edges is empty; section/link edges plug in unchanged when doc-source emits them.
//
// Since ADR-062 increment 4, Hydrate returns a StorageReference HANDLE (read from
// body triples), not inline content — the doc analogue of the code lens. The doc
// body producer that offloads passages + stamps those triples is a tracked
// follow-up; until it lands, Hydrate yields no body and doc_context returns
// structure without verbatim text.
package docs

import (
	"context"
	"path"
	"strings"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/fusion"

	source "github.com/c360studio/semsource/source/vocabulary"
)

// Lens implements fusion.Lens for documents. It is stateless — every domain fact
// comes off the entity's triples.
type Lens struct{}

// New creates a docs lens.
func New() *Lens { return &Lens{} }

// Name identifies the lens.
func (*Lens) Name() string { return "docs" }

// docExtensions mark a single-token query as a document path.
var docExtensions = map[string]bool{".md": true, ".txt": true, ".rst": true, ".adoc": true}

// ResolveMode routes a path-like query (slash, or a bare doc filename) to prefix
// and everything else to nl (semantic) — documents are found by meaning over
// their content, not by symbol name.
func (*Lens) ResolveMode(query string) fusion.ResolveMode {
	q := strings.TrimSpace(query)
	if strings.ContainsAny(q, " \t\n") {
		return fusion.ResolveModeNL
	}
	if strings.ContainsAny(q, "/\\") || docExtensions[strings.ToLower(path.Ext(q))] {
		return fusion.ResolveModePrefix
	}
	return fusion.ResolveModeNL
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

// Hydrate returns the handle to the document body, read from the body triples a
// doc producer stamps at ingest. It is (nil, nil) when no body was offloaded — a
// body is a best-effort facet.
func (*Lens) Hydrate(_ context.Context, e *fusion.Entity) (*message.StorageReference, error) {
	instance := e.First(source.DocBodyStore)
	key := e.First(source.DocBodyKey)
	if instance == "" || key == "" {
		return nil, nil
	}
	return &message.StorageReference{
		StorageInstance: instance,
		Key:             key,
		ContentType:     "text/plain; charset=utf-8",
	}, nil
}
