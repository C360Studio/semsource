package codecontext

import (
	"context"
	"errors"
	"fmt"
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
	unreadable  map[string]fusion.UnhydratedReason
}

func (g *wireGraph) Resolve(ctx context.Context, q fusion.ResolveQuery) (fusion.Resolution, error) {
	g.resolves = append(g.resolves, q)
	if q.Mode == fusion.ResolveModePrefix && strings.ContainsAny(q.Query, `/\`) {
		return fusion.Resolution{}, errors.New("invalid entity ID contract input")
	}
	res, err := g.MemGraph.Resolve(ctx, q)
	if q.Limit > 0 && len(res.Seeds) > q.Limit {
		res.Seeds = res.Seeds[:q.Limit] // the real name index truncates at the limit
	}
	return res, err
}

func (g *wireGraph) Entities(ctx context.Context, ids []string) (fusion.Hydration, error) {
	if g.entitiesErr != nil {
		return fusion.Hydration{}, g.entitiesErr
	}
	h, err := g.MemGraph.Entities(ctx, ids)
	for i, u := range h.Unhydrated {
		if reason, ok := g.unreadable[u.Handle]; ok {
			h.Unhydrated[i].Reason = reason
		}
	}
	return h, err
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

// TestPathSeed_PathBeyondNameLimit: the file asked for ranks after the caller's
// limit among same-named files and must still be found; the limit applies after
// the path filter.
func TestPathSeed_PathBeyondNameLimit(t *testing.T) {
	g := &wireGraph{MemGraph: fusiontest.NewMemGraph()}
	ids := make([]string, 0, 41)
	for i := range 40 {
		id := fmt.Sprintf("o.p.s.golang.file.d%02d-comp-go", i)
		g.AddEntity(id, map[string]any{ast.DcTitle: "comp.go", ast.CodeType: "file", ast.CodePath: fmt.Sprintf("d%02d/comp.go", i)})
		ids = append(ids, id)
	}
	target := "o.p.s.golang.file.target-comp-go"
	g.AddEntity(target, map[string]any{ast.DcTitle: "comp.go", ast.CodeType: "file", ast.CodePath: "target/comp.go"})
	g.SetResolve("comp.go", append(ids, target)...)
	c := pathSeedClient{RetrievalClient: g, lens: "code"}

	res, err := c.Resolve(context.Background(), prefix("target/comp.go"))
	if err != nil || !slices.Equal(fusion.SeedIDs(res.Seeds), []string{target}) {
		t.Errorf("Resolve = %v, %v; want [%s] from past the 40th name match", fusion.SeedIDs(res.Seeds), err, target)
	}
}

// TestPathSeed_UnreadableCandidateIsNotAMiss: a candidate the graph could not
// read stays a seed, so the engine reports it unhydrated instead of a miss; only
// a candidate reported not found is dropped.
func TestPathSeed_UnreadableCandidateIsNotAMiss(t *testing.T) {
	auth := entityid.Authority{Org: "o", Platform: "p"}
	healthy := handlerdoc.DocumentEntityID(auth, "a", "docs/x.md")
	broken := handlerdoc.DocumentEntityID(auth, "b", "docs/x.md")
	for _, reason := range []fusion.UnhydratedReason{fusion.UnhydratedError, fusion.UnhydratedUnknown} {
		t.Run(string(reason), func(t *testing.T) {
			g := &wireGraph{MemGraph: fusiontest.NewMemGraph(), unreadable: map[string]fusion.UnhydratedReason{broken: reason}}
			ctx := context.Background()

			only := pathSeedClient{RetrievalClient: g, lens: "docs", authority: auth, systems: []string{"b"}}
			res, err := only.Resolve(ctx, prefix("docs/x.md"))
			if err != nil || !slices.Equal(fusion.SeedIDs(res.Seeds), []string{broken}) {
				t.Errorf("unreadable only = %v, %v; want the unreadable candidate kept", fusion.SeedIDs(res.Seeds), err)
			}

			g.AddEntity(healthy, map[string]any{source.DocFilePath: "docs/x.md"})
			mixed := pathSeedClient{RetrievalClient: g, lens: "docs", authority: auth, systems: []string{"a", "b"}}
			res, err = mixed.Resolve(ctx, prefix("docs/x.md"))
			if err != nil || !slices.Equal(fusion.SeedIDs(res.Seeds), []string{healthy, broken}) {
				t.Errorf("mixed = %v, %v; want the match and the unreadable candidate", fusion.SeedIDs(res.Seeds), err)
			}
		})
	}
}
