package codecontext

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/c360studio/semstreams/pkg/fusion"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/fusion/fusiontest"
)

// TestExactSeedDecorator pins D4: symbol-mode resolves keep only byte-exact
// display-name matches (methods by their bare name), zero survivors yield an
// empty seed set (the engine's miss path), and NL mode passes through unfiltered.
func TestExactSeedDecorator(t *testing.T) {
	g := fusiontest.NewMemGraph()
	exact := "o.semsource.golang.s.function.SystemSlug"
	lookalike := "o.semsource.golang.s.function.systemSlug"
	method := "o.semsource.golang.s.method.Engine-Fuse"
	g.AddEntity(exact, map[string]any{ast.DcTitle: "SystemSlug", ast.CodeType: "function", ast.CodePath: "a.go"})
	g.AddEntity(lookalike, map[string]any{ast.DcTitle: "systemSlug", ast.CodeType: "function", ast.CodePath: "b.go"})
	g.AddEntity(method, map[string]any{ast.DcTitle: "Fuse", ast.CodeType: "method", ast.CodePath: "c.go"})
	g.SetResolve("SystemSlug", lookalike, exact) // folded recall, lookalike ranked first
	g.SetResolve("Fuse", method)
	g.SetResolve("systemslug", lookalike, exact)

	c := exactSeedClient{g}
	ctx := context.Background()

	ids, err := c.Resolve(ctx, fusion.ResolveQuery{Query: "SystemSlug", Mode: fusion.ResolveModeSymbol})
	if err != nil || !slices.Equal(ids, []string{exact}) {
		t.Errorf("symbol resolve = %v, %v; want exactly [%s]", ids, err, exact)
	}

	ids, err = c.Resolve(ctx, fusion.ResolveQuery{Query: "Fuse", Mode: fusion.ResolveModeSymbol})
	if err != nil || !slices.Equal(ids, []string{method}) {
		t.Errorf("method resolve = %v, %v; want the method kept by bare name", ids, err)
	}

	ids, err = c.Resolve(ctx, fusion.ResolveQuery{Query: "systemslug", Mode: fusion.ResolveModeSymbol})
	if err != nil || len(ids) != 0 {
		t.Errorf("case-sloppy resolve = %v, %v; want zero survivors (miss path)", ids, err)
	}

	ids, err = c.Resolve(ctx, fusion.ResolveQuery{Query: "systemslug", Mode: fusion.ResolveModeNL})
	if err != nil || len(ids) != 2 {
		t.Errorf("nl resolve = %v, %v; want pass-through", ids, err)
	}
}

// TestFuse_VerbEngineSelection pins the D4 verb split end-to-end through serve:
// impact answers drop the case lookalike; search keeps folded recall. It also
// pins D5: the impact response NAMES a direct dependent via the relations facet.
func TestFuse_VerbEngineSelection(t *testing.T) {
	g := fusiontest.NewMemGraph()
	store := fusiontest.NewMemStore()
	exact := "o.semsource.golang.s.function.SystemSlug"
	lookalike := "o.semsource.golang.s.function.systemSlug"
	caller := "o.semsource.golang.s.function.ScopedSystemSlug"
	g.AddEntity(exact, map[string]any{ast.DcTitle: "SystemSlug", ast.CodeType: "function", ast.CodePath: "a.go"})
	g.AddEntity(lookalike, map[string]any{ast.DcTitle: "systemSlug", ast.CodeType: "function", ast.CodePath: "b.go"})
	g.AddEntity(caller, map[string]any{ast.DcTitle: "ScopedSystemSlug", ast.CodeType: "function", ast.CodePath: "a.go"})
	g.AddEdge(caller, ast.CodeCalls, exact)
	g.SetResolve("SystemSlug", lookalike, exact)

	resolver := fusion.NewBodyResolver(fusion.MapStoreResolver{graph.BodyStoreInstance: store})
	c := &Component{
		name: "code-context", lensKind: "code", subjectRoot: "code.v1.",
		graph:       g,
		engine:      fusion.NewEngine(g, resolver),
		exactEngine: fusion.NewEngine(exactSeedClient{g}, resolver),
		logger:      slog.Default(), running: true, startTime: time.Now(),
	}

	body, _ := json.Marshal(fusion.Request{Query: "SystemSlug"})
	impact := decodeResp(t, mustServe(t, c, "impact", body))
	if len(impact.Nodes) != 1 || impact.Nodes[0].Name != "SystemSlug" {
		t.Fatalf("impact nodes = %+v, want only the exact match", impact.Nodes)
	}
	callers := impact.Nodes[0].Relations["caller"]
	found := false
	for _, ref := range callers {
		if ref.Name == "ScopedSystemSlug" {
			found = true
		}
	}
	if !found {
		t.Errorf("impact response names no direct dependent: relations = %+v", impact.Nodes[0].Relations)
	}
	if impact.Impact == nil || impact.Impact.Nodes != 1 {
		t.Errorf("impact closure = %+v, want the single exact-match dependent counted", impact.Impact)
	}

	search := decodeResp(t, mustServe(t, c, "search", body))
	names := make([]string, 0, len(search.Nodes))
	for _, n := range search.Nodes {
		names = append(names, n.Name)
	}
	if !slices.Contains(names, "systemSlug") {
		t.Errorf("search nodes = %v, want folded recall to keep the lookalike", names)
	}
}

// TestDefaultWants_ImpactIncludesRelations pins D5's want-set change.
func TestDefaultWants_ImpactIncludesRelations(t *testing.T) {
	wants := defaultWants("impact")
	if !slices.Contains(wants, fusion.WantRelations) {
		t.Errorf("defaultWants(impact) = %v, want WantRelations included", wants)
	}
}
