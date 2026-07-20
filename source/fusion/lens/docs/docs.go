// Package docs is the fusion docs lens: it serves documents through the same
// pkg/fusion engine as code, proving fusion is a domain-general primitive
// (ADR-0004).
//
// The unit of retrieval is a PASSAGE, not a file. doc-source emits a parent
// document entity plus one passage entity per structural section, so a hit
// returns the passage that answers the question rather than an averaged
// whole-file body. Edges declares passage containment, so a passage resolves to
// its parent document and a document resolves to its passages.
//
// Hydrate returns a StorageReference HANDLE read from the body triples, not
// inline content — the doc analogue of the code lens. handler/doc offloads each
// PASSAGE to the shared CONTENT store (key "doc:<sha256-of-passage>") and stamps
// the DocBodyStore/DocBodyKey handle triples; doc-source wires the store via
// WithBodyStore. Because the key hashes the passage rather than the file,
// editing one section rewrites one blob and leaves the rest untouched.
//
// The parent document carries no body at all: a whole-file body would restore
// the diluted, truncated vector passages exist to replace, and would return the
// same prose twice.
//
// Retrieval SCOPE is handled too (ask #16 / ADR-071, semstreams beta.141): the
// code-context gateway defaults each lens's NL seed resolution to its own domain
// prefixes (docs → the web domain), so a code-heavy corpus no longer dilutes
// doc_context's NL results. See processor/code-context defaultScope; the only
// unscoped fallback is when no org is known (standalone/tests).
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

// ResolveMode routes a path-like query (slash, or a bare doc filename) to prefix
// and everything else to nl (semantic) — documents are found by meaning over
// their content, not by symbol name.
func (*Lens) ResolveMode(query string) fusion.ResolveMode {
	q := strings.TrimSpace(query)
	if strings.ContainsAny(q, " \t\n") {
		return fusion.ResolveModeNL
	}
	if strings.ContainsAny(q, "/\\") || source.IsDocExtension(path.Ext(q)) {
		return fusion.ResolveModePrefix
	}
	return fusion.ResolveModeNL
}

// Edges declares passage containment, so a passage hit resolves to the document
// it came from and a document resolves to its passages.
//
// Restricted to the relations facet deliberately, mirroring how the code lens
// restricts CodeContains: containment belongs in the relations map, not in the
// impact or paths walks. A passage-to-parent edge inside the impact BFS would
// flood every doc-adjacent query with the parent's entire passage set, which is
// noise, not blast radius.
//
// Sibling expansion is one hop short of what the engine does for you: a passage
// seed yields its parent, but reaching that parent's other passages needs an
// outgoing hop then an incoming one, which no built-in walk performs. Callers
// re-seed on the returned parent handle. Emitting explicit sibling predicates
// would mint O(n²) triples per document to save a round trip.
func (*Lens) Edges() []fusion.EdgeSpec {
	return []fusion.EdgeSpec{
		{
			Predicate:    source.CodeBelongs,
			OutgoingRole: "parent_document",
			IncomingRole: "passage",
			Facets:       []fusion.Facet{fusion.FacetRelations},
		},
	}
}

// Label is the title, read from the canonical dc.terms.title. Every document
// and every passage is stamped with one at ingest, so there is no fallback.
func (*Lens) Label(e *fusion.Entity) string { return e.First(source.DcTitle) }

// Kind is the document type ("document").
func (*Lens) Kind(e *fusion.Entity) string { return e.First(source.DocType) }

// Location is the file path, plus a section anchor when the entity is a passage
// derived from a headed section, so a citation deep-links to the section rather
// than the top of the file. No line range: the body comes pre-sliced through the
// handle, and the locator is for display only.
func (*Lens) Location(e *fusion.Entity) fusion.Locator {
	return fusion.Locator{
		Path:     e.First(source.DocFilePath),
		Fragment: source.SectionAnchor(e.First(source.DocSection)),
	}
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
