//go:build integration

package governance

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/pkg/fusion/fusionnats"

	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/ast/golang"
	"github.com/c360studio/semsource/source/fusion/lens/code"
)

// exactByTitle mirrors processor/code-context's exactSeedClient (unexported
// there): symbol-mode resolves keep only byte-exact DcTitle matches. Duplicated
// minimally here so the integration proof exercises the same policy against the
// REAL graph stack without exporting a product seam for tests.
type exactByTitle struct {
	fusion.RetrievalClient
}

func (c exactByTitle) Resolve(ctx context.Context, q fusion.ResolveQuery) ([]string, error) {
	ids, err := c.RetrievalClient.Resolve(ctx, q)
	if err != nil || q.Mode != fusion.ResolveModeSymbol || len(ids) == 0 {
		return ids, err
	}
	entities, err := c.RetrievalClient.Entities(ctx, ids)
	if err != nil {
		return nil, err
	}
	want := strings.TrimSpace(q.Query)
	keep := make(map[string]bool, len(entities))
	for _, e := range entities {
		if e.First(ast.DcTitle) == want {
			keep[e.ID] = true
		}
	}
	exact := make([]string, 0, len(ids))
	for _, id := range ids {
		if keep[id] {
			exact = append(exact, id)
		}
	}
	return exact, nil
}

// TestIntegration_GoCallGraphImpact is the go-callgraph-recall acceptance
// (audit Q7 shape): a SanitizeInstance-style cross-package Go caller must show
// up in the impact closure AND be NAMED in the response's reverse relations,
// while a case-lookalike symbol in another package contributes nothing. The
// pipeline is real end to end — the Go parser resolves the fixture's edges,
// graph-ingest/index/query serve them, and the fusion engine walks them.
func TestIntegration_GoCallGraphImpact(t *testing.T) {
	ctx := context.Background()

	// Fixture: an in-repo cross-package caller of SanitizeInstance, plus a
	// case-lookalike (sanitizeInstance) with its own caller in a third package.
	// If lookalikes leaked into the seed set, their caller would inflate the
	// closure (the Q7 failure); if cross-package resolution failed, the closure
	// would be empty.
	root := t.TempDir()
	files := map[string]string{
		"go.mod":               "module example.com/app\n",
		"entityid/entityid.go": "package entityid\n\nfunc SanitizeInstance() {}\n",
		"handler/handler.go": "package handler\n\nimport \"example.com/app/entityid\"\n\n" +
			"func Run() {\n\tentityid.SanitizeInstance()\n}\n",
		"lk/lk.go": "package lk\n\nfunc sanitizeInstance() {}\n\nfunc Helper() { sanitizeInstance() }\n",
	}
	for rel, src := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	parser := golang.NewParser("acme", "app", root)
	var entities []*ast.CodeEntity
	byName := map[string]*ast.CodeEntity{}
	for rel := range files {
		if !strings.HasSuffix(rel, ".go") {
			continue
		}
		res, err := parser.ParseFile(ctx, filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		entities = append(entities, res.Entities...)
		for _, e := range res.Entities {
			byName[e.Name] = e
		}
	}

	// Pre-assert the parser half: the cross-package call byte-matches the
	// definition ID (clearer failure than an empty closure downstream).
	def, caller := byName["SanitizeInstance"], byName["Run"]
	if def == nil || caller == nil {
		t.Fatalf("fixture entities missing: %v", byName)
	}
	if !slices.Contains(caller.Calls, def.ID) {
		t.Fatalf("cross-package call unresolved: Run.Calls = %v, want %s", caller.Calls, def.ID)
	}

	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "GRAPH",
			Subjects: []string{"graph.ingest.entity"},
		}),
	)
	if _, err := BootstrapStandalone(ctx, tc.Client, nil); err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}
	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	metricsRegistry := metric.NewMetricsRegistry()
	ingest := startGraphIngest(t, ctx, tc.Client, reg, metricsRegistry)
	t.Cleanup(func() { _ = ingest.Stop(5 * time.Second) })
	index := startGraphIndex(t, ctx, tc.Client, metricsRegistry)
	t.Cleanup(func() { _ = index.Stop(5 * time.Second) })
	query := startGraphQuery(t, ctx, tc.Client, metricsRegistry)
	t.Cleanup(func() { _ = query.Stop(5 * time.Second) })

	for _, e := range entities {
		publishSemsourceEntity(t, ctx, tc.Client, e.ID, e.IndexingProfile(), e.Triples())
	}
	for _, e := range entities {
		waitForEntityState(t, ctx, tc.Client, e.ID, 5*time.Second)
	}

	engine := fusion.NewEngine(exactByTitle{fusionnats.New(tc.Client, 0)}, nil)
	lens := code.New()

	var resp fusion.Response
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		r, err := engine.Fuse(ctx, fusion.Request{
			Query: "SanitizeInstance",
			Want:  []fusion.Want{fusion.WantImpact, fusion.WantRelations},
		}, lens)
		if err != nil {
			t.Fatalf("Fuse: %v", err)
		}
		resp = r
		if resp.Index.Ready && len(resp.Nodes) > 0 && resp.Impact != nil && resp.Impact.Nodes > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !resp.Index.Ready {
		t.Fatalf("index never ready: %+v", resp.Index)
	}
	if len(resp.Nodes) != 1 || resp.Nodes[0].Name != "SanitizeInstance" {
		t.Fatalf("seeds = %+v, want only the byte-exact SanitizeInstance", resp.Nodes)
	}

	// The closure must be exactly the cross-package caller: 1 node. The
	// lookalike's caller (lk.Helper) appearing would mean case-folded seeds
	// leaked (Q7's failure mode); 0 would mean cross-package edges are still
	// hollow.
	if resp.Impact == nil || resp.Impact.Nodes != 1 {
		t.Fatalf("impact = %+v, want exactly the one cross-package caller", resp.Impact)
	}

	// And the dependent is NAMED (D5): Run appears as a caller ref.
	callers := resp.Nodes[0].Relations["caller"]
	found := false
	for _, ref := range callers {
		if ref.Name == "Run" {
			found = true
		}
	}
	if !found {
		t.Fatalf("impact response names no caller: relations = %+v", resp.Nodes[0].Relations)
	}
}
