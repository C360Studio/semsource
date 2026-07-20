package docs

import (
	"context"
	"testing"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/vocabulary/cco"

	"github.com/c360studio/semsource/source/fusion/fusiontest"
	"github.com/c360studio/semsource/source/ontology"
	source "github.com/c360studio/semsource/source/vocabulary"
)

func TestResolveMode(t *testing.T) {
	l := New()
	if l.ResolveMode("docs/retry.md") != fusion.ResolveModePrefix {
		t.Error("path should resolve via prefix")
	}
	if l.ResolveMode("how does retry work") != fusion.ResolveModeNL {
		t.Error("NL should resolve via semantic")
	}
	if l.ResolveMode("retry") != fusion.ResolveModeNL {
		t.Error("single term should resolve via semantic (doc content), not symbol")
	}
}

func TestFieldExtractionAndHydrate(t *testing.T) {
	l := New()
	e := &fusion.Entity{Triples: []message.Triple{
		{Predicate: source.DocType, Object: "document"},
		{Predicate: source.DcTitle, Object: "Retry Policy"},
		{Predicate: source.DocFilePath, Object: "docs/retry.md"},
		{Predicate: source.DocBodyStore, Object: "objectstore"},
		{Predicate: source.DocBodyKey, Object: "body:retry"},
	}}
	if l.Label(e) != "Retry Policy" || l.Kind(e) != "document" {
		t.Fatalf("label/kind: %q/%q", l.Label(e), l.Kind(e))
	}
	if loc := l.Location(e); loc.Path != "docs/retry.md" || loc.Lines != [2]int{0, 0} {
		t.Fatalf("location: %+v", loc)
	}
	ref, err := l.Hydrate(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	if ref == nil || ref.StorageInstance != "objectstore" || ref.Key != "body:retry" {
		t.Fatalf("hydrate handle = %+v; want objectstore/body:retry", ref)
	}
}

// TestDocsLensViaEngine proves the SAME engine serves docs — body dereferenced
// from the body store through the handle, no worktree, no engine change — which
// is the generality claim of ADR-0004.
func TestDocsLensViaEngine(t *testing.T) {
	g := fusiontest.NewMemGraph()
	store := fusiontest.NewMemStore()
	body := "# Retry Policy\n\nUse exponential backoff with jitter."
	store.Set("body:retry", body)

	id := "o.semsource.web.s.doc.abc123"
	g.AddEntity(id, map[string]any{
		source.DocType:          "document",
		source.DcTitle:          "Retry Policy",
		source.DocFilePath:      "docs/retry.md",
		source.DocBodyStore:     "objectstore",
		source.DocBodyKey:       "body:retry",
		ontology.ClassPredicate: cco.Document,
	})
	g.SetResolve("how does retry work", id)

	engine := fusion.NewEngine(g, fusion.NewBodyResolver(fusion.MapStoreResolver{"objectstore": store}))
	resp, err := engine.Fuse(context.Background(),
		fusion.Request{Query: "how does retry work", Want: []fusion.Want{fusion.WantBody}},
		New())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 doc node, got %d", len(resp.Nodes))
	}
	n := resp.Nodes[0]
	if n.Name != "Retry Policy" || n.Kind != "document" || n.Body != body || n.Path != "docs/retry.md" {
		t.Fatalf("doc node mismatch: %+v", n)
	}
	if n.Class != cco.Document {
		t.Fatalf("expected Document class, got %q", n.Class)
	}
	// NL query → semantic resolve → embedding provenance.
	if resp.Provenance != fusion.ProvenanceEmbedding {
		t.Fatalf("expected embedding provenance for NL doc query, got %q", resp.Provenance)
	}
}

// TestLabelReadsOnlyTheTitle pins the removal of the summary fallback. Label
// used to try source.doc.summary when dc.terms.title was missing, which meant a
// producer could skip the title and still get a plausible-looking result list —
// the ingest bug stayed invisible. Every document and every passage is stamped
// with a title at ingest, so a missing title is now a visible blank rather than
// a silently substituted one, and a stray summary triple must not paper over it.
func TestLabelReadsOnlyTheTitle(t *testing.T) {
	l := New()

	titled := &fusion.Entity{Triples: []message.Triple{
		{Predicate: source.DocType, Object: "document"},
		{Predicate: source.DcTitle, Object: "Retry Policy"},
	}}
	if got := l.Label(titled); got != "Retry Policy" {
		t.Errorf("Label with %s present = %q, want %q", source.DcTitle, got, "Retry Policy")
	}

	// The retired predicate is named by its wire string: the constant is gone,
	// but a stale producer or an archived entity can still carry the triple.
	summaryOnly := &fusion.Entity{Triples: []message.Triple{
		{Predicate: source.DocType, Object: "document"},
		{Predicate: "source.doc.summary", Object: "Retry Policy"},
	}}
	if got := l.Label(summaryOnly); got != "" {
		t.Errorf("Label with only a retired source.doc.summary triple = %q, want %q (no summary fallback)", got, "")
	}
}

// TestEdgesDeclarePassageContainment pins the edge the engine walks to get from
// a passage to the document it came from, and back the other way.
func TestEdgesDeclarePassageContainment(t *testing.T) {
	edges := New().Edges()
	if len(edges) != 1 {
		t.Fatalf("Edges() returned %d specs, want exactly the containment edge", len(edges))
	}
	e := edges[0]
	if e.Predicate != source.CodeBelongs {
		t.Errorf("predicate = %q, want %q", e.Predicate, source.CodeBelongs)
	}
	if e.OutgoingRole == "" {
		t.Error("outgoing role is empty: a passage could not resolve to its parent document")
	}
	if e.IncomingRole == "" {
		t.Error("incoming role is empty: a document could not resolve to its passages")
	}
}

// TestEdgesKeepContainmentOutOfImpactAndPaths pins the facet restriction. A
// passage-to-parent edge inside the impact BFS would flood every doc-adjacent
// query with the parent's entire passage set — noise, not blast radius. The code
// lens restricts CodeContains for the same reason.
func TestEdgesKeepContainmentOutOfImpactAndPaths(t *testing.T) {
	edges := New().Edges()
	if len(edges) == 0 {
		t.Fatal("no edges declared")
	}
	for _, e := range edges {
		if len(e.Facets) == 0 {
			t.Fatalf("edge %q declares no facets; an empty facet set means ALL facets, "+
				"so containment would join the impact and paths walks", e.Predicate)
		}
		for _, f := range e.Facets {
			if f == fusion.FacetImpact {
				t.Errorf("edge %q participates in the impact walk", e.Predicate)
			}
			if f == fusion.FacetPaths {
				t.Errorf("edge %q participates in the paths walk", e.Predicate)
			}
		}
	}
}

// TestLocationCarriesSectionAnchor pins that a passage from a headed section
// cites its section rather than the top of the file, and that a parent document
// (no section) carries no fragment.
func TestLocationCarriesSectionAnchor(t *testing.T) {
	l := New()

	passage := &fusion.Entity{Triples: []message.Triple{
		{Predicate: source.DocType, Object: "passage"},
		{Predicate: source.DocFilePath, Object: "docs/retry.md"},
		{Predicate: source.DocSection, Object: "Build & Test Commands"},
	}}
	loc := l.Location(passage)
	if loc.Path != "docs/retry.md" {
		t.Errorf("path = %q, want docs/retry.md", loc.Path)
	}
	if loc.Fragment != "build--test-commands" {
		t.Errorf("fragment = %q, want the section anchor build--test-commands", loc.Fragment)
	}

	parent := &fusion.Entity{Triples: []message.Triple{
		{Predicate: source.DocType, Object: "document"},
		{Predicate: source.DocFilePath, Object: "docs/retry.md"},
	}}
	if got := l.Location(parent).Fragment; got != "" {
		t.Errorf("parent document fragment = %q, want empty", got)
	}
}

// TestLabelAndKindForPassages pins that a passage renders usefully as a
// neighbour reference: the engine builds every Ref from Label and Location, so a
// passage that labelled blank would degrade every relations listing that
// mentions it.
func TestLabelAndKindForPassages(t *testing.T) {
	l := New()
	p := &fusion.Entity{Triples: []message.Triple{
		{Predicate: source.DocType, Object: "passage"},
		{Predicate: source.DcTitle, Object: "Retry Policy § Backoff"},
		{Predicate: source.DocFilePath, Object: "docs/retry.md"},
	}}
	if got := l.Label(p); got != "Retry Policy § Backoff" {
		t.Errorf("label = %q, want the qualified passage title", got)
	}
	if got := l.Kind(p); got != "passage" {
		t.Errorf("kind = %q, want passage", got)
	}
}
