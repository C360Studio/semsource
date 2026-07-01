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
		{Predicate: source.DocSummary, Object: "Retry Policy"},
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
		source.DocSummary:       "Retry Policy",
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
