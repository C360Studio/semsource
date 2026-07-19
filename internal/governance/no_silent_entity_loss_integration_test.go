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
	queryclient "github.com/c360studio/semstreams/graph/query"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"

	semsourcegraph "github.com/c360studio/semsource/graph"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

// TestIntegration_NoSilentEntityLoss_AuditShapes is the change's end-to-end
// proof: the REAL ast-source component indexes a tree containing every shape
// the 2026-07-19 audit proved was silently dropped (SvelteKit "+page" route
// files, "[slug]" directories, "$"-identifiers, "_"-prefixed directories),
// and every produced entity actually lands in the governed graph — with the
// source reporting zero errors. Before this change these entities passed the
// producer gate and were Termed by graph-ingest while status looked healthy.
func TestIntegration_NoSilentEntityLoss_AuditShapes(t *testing.T) {
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
	// The audit's silently-dropped shapes, plus one clean control per language.
	write("app/src/routes/+page.svelte", "<script lang=\"ts\">\n  let count = 1;\n</script>\n<h1>{count}</h1>\n")
	write("app/src/routes/[slug]/+page.ts", "export function load() {\n  return {};\n}\n")
	write("app/src/app.ts", "export const clicks$ = 1;\nexport const control = 2;\n")
	write("gosrc/_examples/demo.go", "package demo\n\n// Demo exists in an underscore-prefixed dir.\nfunc Demo() string { return \"d\" }\n")
	write("gosrc/main.go", "package demo\n\nfunc Normal() string { return \"n\" }\n")

	astCfg, err := json.Marshal(map[string]any{
		"watch_paths": []map[string]any{
			{"path": filepath.Join(root, "app"), "org": "acme", "project": "zl", "languages": []string{"typescript", "svelte"}},
			{"path": filepath.Join(root, "gosrc"), "org": "acme", "project": "zl", "languages": []string{"go"}},
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

	// Expected IDs computed through the SAME production constructors the
	// parsers use — exact-match assertions, no substring guessing.
	expected := []string{
		semsourceast.NewCodeEntity("acme", "svelte", "zl", semsourceast.TypeComponent, "+page", "src/routes/+page.svelte").ID,
		semsourceast.NewCodeEntity("acme", "svelte", "zl", semsourceast.TypeFile, "", "src/routes/+page.svelte").ID,
		semsourceast.NewCodeEntity("acme", "typescript", "zl", semsourceast.TypeFunction, "load", "src/routes/[slug]/+page.ts").ID,
		semsourceast.NewCodeEntity("acme", "typescript", "zl", semsourceast.TypeConst, "clicks$", "src/app.ts").ID,
		semsourceast.NewCodeEntity("acme", "typescript", "zl", semsourceast.TypeConst, "control", "src/app.ts").ID,
		semsourceast.NewCodeEntity("acme", "golang", "zl", semsourceast.TypeFunction, "Demo", "_examples/demo.go").ID,
		semsourceast.NewCodeEntity("acme", "golang", "zl", semsourceast.TypeFunction, "Normal", "main.go").ID,
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
		t.Errorf("entity never landed in the graph: %s", id)
	}

	// Delivery-truth: the source must report zero errors — silent loss with a
	// healthy status was the audit's core finding.
	if health := astComp.Health(); health.ErrorCount != 0 {
		t.Errorf("ast-source Health().ErrorCount = %d, want 0", health.ErrorCount)
	}
}
