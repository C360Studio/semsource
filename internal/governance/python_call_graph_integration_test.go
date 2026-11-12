//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/fusion"
	"github.com/c360studio/semengine/pkg/fusion/fusionnats"
	"github.com/c360studio/semengine/types"

	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/natstest"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	"github.com/c360studio/semsource/source/fusion/lens/code"
)

// TestIntegration_PythonCallGraphCrossFile is the task #45 live proof: the REAL
// ast-source component indexes a cross-file Python package, and the fusion code
// lens surfaces the caller under the callee's `caller` role — i.e. the call edge
// (`from pkg.util import helper` → `helper()`) resolved to helper's definition ID
// end to end through graph-ingest → graph-index → the relations facet. Before #45
// the parser emitted no call edges at all, so this role was always empty.
// spec: openspec/specs/code-call-graph/spec.md#Requirement: Imported callees resolve to their defining module
func TestIntegration_PythonCallGraphCrossFile(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t,
		natstest.WithJetStream(),
		natstest.WithStreams(natstest.Stream{
			Name:     "GRAPH",
			Subjects: []string{"graph.ingest.entity"},
		}),
	)
	if _, err := BootstrapStandalone(nil); err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}

	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	metricsRegistry := metric.NewMetricsRegistry()

	ingest := startGraphIngest(t, ctx, tc.Client, reg, metricsRegistry)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, ingest.Stop) })
	index := startGraphIndex(t, ctx, tc.Client, metricsRegistry)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, index.Stop) })
	query := startGraphQuery(t, ctx, tc.Client, metricsRegistry)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, query.Stop) })

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
	})
	if err != nil {
		t.Fatalf("marshal ast-source config: %v", err)
	}
	discovered, err := astsource.NewComponent(astCfg, component.Dependencies{NATSClient: tc.Client, Platform: types.PlatformMeta{Org: "acme", Platform: "test-a1b2c3"}})
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
	t.Cleanup(func() { _ = stopWithin(5*time.Second, astComp.Stop) })

	gc, gcErr := fusionnats.New(tc.Client, 0)
	if gcErr != nil {
		t.Fatalf("fusionnats.New() error = %v", gcErr)
	}
	engine, engErr := fusion.NewEngine(gc, fusion.NewBodyResolver(fusion.MapStoreResolver{}), slog.Default())
	if engErr != nil {
		t.Fatalf("fusion.NewEngine() error = %v", engErr)
	}
	lens := prefixLens{code.New()}

	var callers []fusion.Ref
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, ferr := engine.Fuse(ctx, fusion.Request{
			Query: "acme.test-a1b2c3.ml.python",
			Want:  []fusion.Want{fusion.WantRelations},
		}, lens)
		if ferr != nil {
			if fuseErrIsRetryable(t, ferr) {
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}
		if !resp.Deferred {
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
