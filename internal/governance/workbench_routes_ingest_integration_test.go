//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	queryclient "github.com/c360studio/semstreams/graph/query"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"

	semsourcegraph "github.com/c360studio/semsource/graph"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

// TestIntegration_WorkbenchRoutesIngest indexes THIS repo's real ui/src/routes
// tree (the audit's motivating case: SemSource's own workbench "+page.svelte"
// and "+layout.svelte" were silently rejected by graph-ingest) and asserts the
// route components land in the governed graph with zero source errors.
func TestIntegration_WorkbenchRoutesIngest(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	routesDir := filepath.Join(repoRoot, "ui", "src", "routes")

	ctx := context.Background()
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
	mr := metric.NewMetricsRegistry()

	ingest := startGraphIngest(t, ctx, tc.Client, reg, mr)
	t.Cleanup(func() { _ = ingest.Stop(5 * time.Second) })
	index := startGraphIndex(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = index.Stop(5 * time.Second) })
	q := startGraphQuery(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = q.Stop(5 * time.Second) })

	astCfg, err := json.Marshal(map[string]any{
		"watch_paths": []map[string]any{
			{"path": routesDir, "org": "c360", "project": "workbench", "languages": []string{"svelte"}},
		},
		"watch_enabled":  false,
		"index_interval": "",
		"stream_name":    "GRAPH",
	})
	if err != nil {
		t.Fatalf("marshal ast-source config: %v", err)
	}
	discovered, err := astsource.NewComponent(astCfg, component.Dependencies{NATSClient: tc.Client})
	if err != nil {
		t.Fatalf("ast-source NewComponent: %v", err)
	}
	astComp := discovered.(*astsource.Component)
	if err := astComp.Initialize(); err != nil {
		t.Fatalf("ast-source Initialize: %v", err)
	}
	if err := astComp.Start(ctx); err != nil {
		t.Fatalf("ast-source Start: %v", err)
	}
	t.Cleanup(func() { _ = astComp.Stop(5 * time.Second) })

	qc, err := queryclient.NewClient(ctx, tc.Client, nil)
	if err != nil {
		t.Fatalf("queryclient.NewClient: %v", err)
	}

	expected := []string{
		semsourceast.NewCodeEntity("c360", "svelte", "workbench", semsourceast.TypeComponent, "+page", "+page.svelte").ID,
		semsourceast.NewCodeEntity("c360", "svelte", "workbench", semsourceast.TypeComponent, "+layout", "+layout.svelte").ID,
	}

	deadline := time.Now().Add(30 * time.Second)
	missing := map[string]bool{}
	for {
		missing = map[string]bool{}
		for _, id := range expected {
			if _, ok := fetchEntity(ctx, qc, id); !ok {
				missing[id] = true
			}
		}
		if len(missing) == 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	for id := range missing {
		t.Errorf("workbench route entity never landed: %s", id)
	}
	if health := astComp.Health(); health.ErrorCount != 0 {
		t.Errorf("ast-source Health().ErrorCount = %d, want 0", health.ErrorCount)
	}
}
