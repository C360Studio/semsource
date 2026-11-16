package codecontext

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/pkg/fusion"

	"github.com/c360studio/semsource/entityid"
	handlerdoc "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/fusion/fusiontest"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// wireGraph refuses what the real fusionnats client refuses: a prefix query
// holding a character an entity-ID segment cannot. It also records every
// resolve, so a test can assert no path ever reaches prefix search.
type wireGraph struct {
	*fusiontest.MemGraph
	resolves    []fusion.ResolveQuery
	entitiesErr error
}

func (g *wireGraph) Resolve(ctx context.Context, q fusion.ResolveQuery) (fusion.Resolution, error) {
	g.resolves = append(g.resolves, q)
	if q.Mode == fusion.ResolveModePrefix && strings.ContainsAny(q.Query, `/\`) {
		return fusion.Resolution{}, errors.New("invalid entity ID contract input")
	}
	return g.MemGraph.Resolve(ctx, q)
}

func (g *wireGraph) Entities(ctx context.Context, ids []string) (fusion.Hydration, error) {
	if g.entitiesErr != nil {
		return fusion.Hydration{}, g.entitiesErr
	}
	return g.MemGraph.Entities(ctx, ids)
}

func prefix(q string) fusion.ResolveQuery {
	return fusion.ResolveQuery{Query: q, Mode: fusion.ResolveModePrefix, Limit: 40}
}

// TestPathSeed_CodeFile pins #276 for the code lens: a path resolves to the file
// entity at exactly that path, through the base-name lookup, never prefix search.
func TestPathSeed_CodeFile(t *testing.T) {
	g := &wireGraph{MemGraph: fusiontest.NewMemGraph()}
	want := "o.p.s.golang.file.pkg-comp-go"
	other := "o.p.s.golang.file.other-comp-go"
	fn := "o.p.s.golang.function.pkg-comp-go-New"
	g.AddEntity(want, map[string]any{ast.DcTitle: "comp.go", ast.CodeType: "file", ast.CodePath: "pkg/comp.go"})
	g.AddEntity(other, map[string]any{ast.DcTitle: "comp.go", ast.CodeType: "file", ast.CodePath: "other/comp.go"})
	g.AddEntity(fn, map[string]any{ast.DcTitle: "comp.go", ast.CodeType: "function", ast.CodePath: "pkg/comp.go"})
	g.SetResolve("comp.go", other, fn, want)
	c := pathSeedClient{RetrievalClient: g, lens: "code"}

	for _, q := range []string{"pkg/comp.go", "./pkg/comp.go", `pkg\comp.go`, " pkg/comp.go "} {
		res, err := c.Resolve(context.Background(), prefix(q))
		if err != nil || !slices.Equal(fusion.SeedIDs(res.Seeds), []string{want}) {
			t.Errorf("Resolve(%q) = %v, %v; want exactly [%s]", q, fusion.SeedIDs(res.Seeds), err, want)
		}
	}
	for _, r := range g.resolves {
		if r.Mode == fusion.ResolveModePrefix {
			t.Errorf("a path reached prefix search: %+v", r)
		}
	}

	res, err := c.Resolve(context.Background(), prefix("missing/comp.go"))
	if err != nil || len(res.Seeds) != 0 {
		t.Errorf("absent path = %v, %v; want zero seeds and no error (the miss path)", res.Seeds, err)
	}
}

// TestPathSeed_Document pins #276 for the docs lens: a path resolves to the
// document entity a boot source system gives that logical path, and an
// entity-ID prefix still goes to prefix search.
func TestPathSeed_Document(t *testing.T) {
	g := &wireGraph{MemGraph: fusiontest.NewMemGraph()}
	auth := entityid.Authority{Org: "o", Platform: "p-1"}
	retry := handlerdoc.DocumentEntityID(auth, "docs-b", "docs/retry.md")
	readme := handlerdoc.DocumentEntityID(auth, "docs-a", "README.md")
	g.AddEntity(retry, map[string]any{source.DocFilePath: "docs/retry.md"})
	g.AddEntity(readme, map[string]any{source.DocFilePath: "README.md"})
	g.SetResolve("o.p-1.docs-a.web", readme)
	c := pathSeedClient{RetrievalClient: g, lens: "docs", authority: auth, systems: []string{"docs-a", "docs-b"}}
	ctx := context.Background()

	cases := map[string][]string{
		"docs/retry.md":   {retry},
		"./docs/retry.md": {retry},
		"README.md":       {readme},
		"retry.md":        {},
	}
	for q, want := range cases {
		res, err := c.Resolve(ctx, prefix(q))
		if err != nil || !slices.Equal(fusion.SeedIDs(res.Seeds), want) {
			t.Errorf("Resolve(%q) = %v, %v; want %v", q, fusion.SeedIDs(res.Seeds), err, want)
		}
	}
	if len(g.resolves) != 0 {
		t.Errorf("a document path reached a graph resolve: %+v", g.resolves)
	}

	res, err := c.Resolve(ctx, prefix("o.p-1.docs-a.web"))
	if err != nil || !slices.Equal(fusion.SeedIDs(res.Seeds), []string{readme}) {
		t.Errorf("entity-ID prefix = %v, %v; want it passed through to prefix search", fusion.SeedIDs(res.Seeds), err)
	}

	unconfigured := pathSeedClient{RetrievalClient: g, lens: "docs"}
	res, err = unconfigured.Resolve(ctx, prefix("docs/retry.md"))
	if err != nil || len(res.Seeds) != 0 {
		t.Errorf("no authority = %v, %v; want zero seeds and no error", res.Seeds, err)
	}
}

// TestPathSeed_BackendFailureStaysAFailure: a hydration failure must not read
// as "no such file".
func TestPathSeed_BackendFailureStaysAFailure(t *testing.T) {
	down := errors.New("graph down")
	g := &wireGraph{MemGraph: fusiontest.NewMemGraph(), entitiesErr: down}
	g.SetResolve("comp.go", "o.p.s.golang.file.pkg-comp-go")
	auth := entityid.Authority{Org: "o", Platform: "p"}
	for _, c := range []pathSeedClient{
		{RetrievalClient: g, lens: "code"},
		{RetrievalClient: g, lens: "docs", authority: auth, systems: []string{"s"}},
	} {
		if _, err := c.Resolve(context.Background(), prefix("pkg/comp.go")); !errors.Is(err, down) {
			t.Errorf("%s lens: err = %v; want the backend failure", c.lens, err)
		}
	}
}

// TestPathSeed_OtherModesPassThrough: symbol and NL resolves are untouched.
func TestPathSeed_OtherModesPassThrough(t *testing.T) {
	g := &wireGraph{MemGraph: fusiontest.NewMemGraph()}
	g.SetResolve("pkg/comp.go", "x")
	c := pathSeedClient{RetrievalClient: g, lens: "code"}
	for _, mode := range []fusion.ResolveMode{fusion.ResolveModeSymbol, fusion.ResolveModeNL} {
		res, err := c.Resolve(context.Background(), fusion.ResolveQuery{Query: "pkg/comp.go", Mode: mode})
		if err != nil || !slices.Equal(fusion.SeedIDs(res.Seeds), []string{"x"}) {
			t.Errorf("%s = %v, %v; want pass-through", mode, fusion.SeedIDs(res.Seeds), err)
		}
	}
}
