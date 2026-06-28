package code

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/vocabulary/cco"

	"github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/fusion"
	"github.com/c360studio/semsource/source/fusion/fusiontest"
	"github.com/c360studio/semsource/source/ontology"
)

func TestResolveMode(t *testing.T) {
	l := New("/tmp")
	cases := map[string]fusion.ResolveMode{
		"OnEvent":             fusion.ResolveSymbol,
		"PlanManager.OnEvent": fusion.ResolveSymbol,
		"pkg/comp.go":         fusion.ResolvePrefix,
		"comp.go":             fusion.ResolvePrefix,
		"where is dispatch":   fusion.ResolveSemantic,
	}
	for q, want := range cases {
		if got := l.ResolveMode(q); got != want {
			t.Errorf("ResolveMode(%q) = %v; want %v", q, got, want)
		}
	}
}

func TestFieldExtraction(t *testing.T) {
	l := New("/tmp")
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

func TestHydrate(t *testing.T) {
	root := t.TempDir()
	body := "func Foo() int {\n\treturn 1\n}"
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package p\n\n"+body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := New(root)
	e := &fusion.Entity{Triples: []message.Triple{
		{Predicate: ast.CodePath, Object: "a.go"},
		{Predicate: ast.CodeStartLine, Object: 3},
		{Predicate: ast.CodeEndLine, Object: 5},
	}}
	got, _ := l.Hydrate(context.Background(), e)
	if got != body {
		t.Fatalf("Hydrate = %q; want %q", got, body)
	}
}

// TestCodeLensViaEngine drives the real fusion engine with the code lens: it
// must return verbatim source plus callers/callees — the code_context shape.
func TestCodeLensViaEngine(t *testing.T) {
	root := t.TempDir()
	src := "package svc\n\nfunc Dispatch() {\n\tOnEvent()\n}\n\nfunc OnEvent() {}\n"
	if err := os.WriteFile(filepath.Join(root, "svc.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	g := fusiontest.NewMemGraph()
	dispatch := "o.semsource.golang.s.function.dispatch"
	onEvent := "o.semsource.golang.s.function.onevent"
	g.AddEntity(dispatch, map[string]any{
		ast.DcTitle: "Dispatch", ast.CodeType: "function", ast.CodePath: "svc.go",
		ast.CodeStartLine: 3, ast.CodeEndLine: 5, ontology.ClassPredicate: cco.Algorithm,
	})
	g.AddEntity(onEvent, map[string]any{
		ast.DcTitle: "OnEvent", ast.CodeType: "function", ast.CodePath: "svc.go",
		ast.CodeStartLine: 7, ast.CodeEndLine: 7, ontology.ClassPredicate: cco.Algorithm,
	})
	g.AddEdge(dispatch, ast.CodeCalls, onEvent)
	g.SetResolve("Dispatch", dispatch)

	resp, err := fusion.NewEngine(g).Fuse(context.Background(),
		fusion.Request{Query: "Dispatch", Want: []fusion.Want{fusion.WantBody, fusion.WantRelations}},
		New(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(resp.Nodes))
	}
	n := resp.Nodes[0]
	if n.Name != "Dispatch" || n.Body != "func Dispatch() {\n\tOnEvent()\n}" {
		t.Fatalf("node body mismatch: name=%q body=%q", n.Name, n.Body)
	}
	if got := n.Relations["callee"]; len(got) != 1 || got[0].Name != "OnEvent" {
		t.Fatalf("expected callee OnEvent, got %+v", n.Relations)
	}
	if n.Class != cco.Algorithm {
		t.Fatalf("expected Algorithm class, got %q", n.Class)
	}
}
