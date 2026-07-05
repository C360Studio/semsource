//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/pkg/fusion/fusionnats"

	semsourcegraph "github.com/c360studio/semsource/graph"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	"github.com/c360studio/semsource/source/fusion/lens/code"
)

// TestIntegration_PythonCallGraphCrossFile is the task #45 live proof: the REAL
// ast-source component indexes a cross-file Python package, and the fusion code
// lens surfaces the caller under the callee's `caller` role — i.e. the call edge
// (`from pkg.util import helper` → `helper()`) resolved to helper's definition ID
// end to end through graph-ingest → graph-index → the relations facet. Before #45
// the parser emitted no call edges at all, so this role was always empty.
func TestIntegration_PythonCallGraphCrossFile(t *testing.T) {
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
	metricsRegistry := metric.NewMetricsRegistry()

	ingest := startGraphIngest(t, ctx, tc.Client, reg, metricsRegistry)
	t.Cleanup(func() { _ = ingest.Stop(5 * time.Second) })
	index := startGraphIndex(t, ctx, tc.Client, metricsRegistry)
	t.Cleanup(func() { _ = index.Stop(5 * time.Second) })
	query := startGraphQuery(t, ctx, tc.Client, metricsRegistry)
	t.Cleanup(func() { _ = query.Stop(5 * time.Second) })

	root := t.TempDir()
	write := func(rel, src string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("pkg/__init__.py", "")
	write("pkg/util.py", "def helper():\n    return 1\n")
	write("pkg/app.py", "from pkg.util import helper\n\ndef run():\n    return helper()\n")

	astCfg, err := json.Marshal(map[string]any{
		"watch_paths": []map[string]any{
			{"path": root, "org": "acme", "project": "ml", "languages": []string{"python"}},
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

	engine := fusion.NewEngine(fusionnats.New(tc.Client, 0), fusion.NewBodyResolver(fusion.MapStoreResolver{}))
	lens := prefixLens{code.New()}

	var callers []fusion.Ref
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, ferr := engine.Fuse(ctx, fusion.Request{
			Query: "acme.semsource.python.ml",
			Want:  []fusion.Want{fusion.WantRelations},
		}, lens)
		if ferr != nil {
			t.Fatalf("Fuse: %v", ferr)
		}
		if resp.Index.Ready {
			if helper := findNode(resp.Nodes, "helper"); helper != nil {
				if got := helper.Relations["caller"]; len(got) > 0 {
					callers = got
					break
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(callers) == 0 {
		t.Fatal("no callers for helper — cross-file call edge did not resolve live")
	}
	found := false
	for _, c := range callers {
		if c.Name == "run" {
			found = true
		}
	}
	if !found {
		t.Fatalf("helper.caller = %+v, want to contain run", callers)
	}
}
