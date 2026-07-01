package code

import (
	"context"
	"testing"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/vocabulary/cco"

	"github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/fusion/fusiontest"
	"github.com/c360studio/semsource/source/ontology"
)

func TestResolveMode(t *testing.T) {
	l := New()
	cases := map[string]fusion.ResolveMode{
		"OnEvent":             fusion.ResolveModeSymbol,
		"PlanManager.OnEvent": fusion.ResolveModeSymbol,
		"pkg/comp.go":         fusion.ResolveModePrefix,
		"comp.go":             fusion.ResolveModePrefix,
		"where is dispatch":   fusion.ResolveModeNL,
	}
	for q, want := range cases {
		if got := l.ResolveMode(q); got != want {
			t.Errorf("ResolveMode(%q) = %v; want %v", q, got, want)
		}
	}
}

func TestFieldExtraction(t *testing.T) {
	l := New()
	e := &fusion.Entity{Triples: []message.Triple{
		{Predicate: ast.DcTitle, Object: "OnEvent"},
		{Predicate: ast.CodeType, Object: "method"},
		{Predicate: ast.CodePath, Object: "c.go"},
		{Predicate: ast.CodeStartLine, Object: 10},
		{Predicate: ast.CodeEndLine, Object: 20},
	}}
	if l.Label(e) != "OnEvent" || l.Kind(e) != "method" {
		t.Fatalf("label/kind: %q/%q", l.Label(e), l.Kind(e))
	}
	loc := l.Location(e)
	if loc.Path != "c.go" || loc.Lines != [2]int{10, 20} {
		t.Fatalf("location: %+v", loc)
	}
}

// TestHydrate: the lens returns the StorageReference HANDLE built from the body
// triples the producer stamped — never bytes, never a filesystem read (ADR-062).
func TestHydrate(t *testing.T) {
	l := New()
	e := &fusion.Entity{Triples: []message.Triple{
		{Predicate: ast.CodePath, Object: "a.go"},
		{Predicate: ast.CodeBodyStore, Object: "objectstore"},
		{Predicate: ast.CodeBodyKey, Object: "sha256:abc123"},
	}}
	ref, err := l.Hydrate(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	if ref == nil || ref.StorageInstance != "objectstore" || ref.Key != "sha256:abc123" {
		t.Fatalf("Hydrate handle = %+v; want objectstore/sha256:abc123", ref)
	}
}

// TestHydrateNoBody: an entity without body triples yields no handle (nil, nil)
// so the engine degrades the node rather than erroring.
func TestHydrateNoBody(t *testing.T) {
	l := New()
	e := &fusion.Entity{Triples: []message.Triple{{Predicate: ast.CodePath, Object: "a.go"}}}
	if ref, err := l.Hydrate(context.Background(), e); ref != nil || err != nil {
		t.Fatalf("Hydrate = (%+v, %v); want (nil, nil)", ref, err)
	}
}

// TestCodeLensViaEngine drives the real fusion engine with the code lens: it
// must return verbatim source (dereferenced from the body store) plus
// callers/callees — the code_context shape.
func TestCodeLensViaEngine(t *testing.T) {
	dispatchBody := "func Dispatch() {\n\tOnEvent()\n}"

	g := fusiontest.NewMemGraph()
	store := fusiontest.NewMemStore()
	store.Set("body:dispatch", dispatchBody)

	dispatch := "o.semsource.golang.s.function.dispatch"
	onEvent := "o.semsource.golang.s.function.onevent"
	g.AddEntity(dispatch, map[string]any{
		ast.DcTitle: "Dispatch", ast.CodeType: "function", ast.CodePath: "svc.go",
		ast.CodeStartLine: 3, ast.CodeEndLine: 5, ontology.ClassPredicate: cco.Algorithm,
		ast.CodeBodyStore: "objectstore", ast.CodeBodyKey: "body:dispatch",
	})
	g.AddEntity(onEvent, map[string]any{
		ast.DcTitle: "OnEvent", ast.CodeType: "function", ast.CodePath: "svc.go",
		ast.CodeStartLine: 7, ast.CodeEndLine: 7, ontology.ClassPredicate: cco.Algorithm,
	})
	g.AddEdge(dispatch, ast.CodeCalls, onEvent)
	g.SetResolve("Dispatch", dispatch)

	engine := fusion.NewEngine(g, fusion.NewBodyResolver(fusion.MapStoreResolver{"objectstore": store}))
	resp, err := engine.Fuse(context.Background(),
		fusion.Request{Query: "Dispatch", Want: []fusion.Want{fusion.WantBody, fusion.WantRelations}},
		New())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(resp.Nodes))
	}
	n := resp.Nodes[0]
	if n.Name != "Dispatch" || n.Body != dispatchBody {
		t.Fatalf("node body mismatch: name=%q body=%q", n.Name, n.Body)
	}
	if got := n.Relations["callee"]; len(got) != 1 || got[0].Name != "OnEvent" {
		t.Fatalf("expected callee OnEvent, got %+v", n.Relations)
	}
	if n.Class != cco.Algorithm {
		t.Fatalf("expected Algorithm class, got %q", n.Class)
	}
}
