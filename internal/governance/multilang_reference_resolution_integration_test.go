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

// TestIntegration_MultiLangCrossFileReferenceResolution is the task #46 live proof:
// the REAL ast-source component indexes cross-file Java, TypeScript, and Go trees,
// and the fusion code lens surfaces each base type's dependents via its reverse
// role — extended_by (Java/TS) and embedded_by (Go). Before #46 these targets
// dangled (wrong entity-type segment / raw project / referrer-relative path), so
// the reverse closure was empty; this asserts the edge now resolves end to end
// through graph-ingest → graph-index → the fusion relations facet.
func TestIntegration_MultiLangCrossFileReferenceResolution(t *testing.T) {
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
	// Java: cross-file extends within a package (Dog.java extends Animal.java).
	write("java/a/Animal.java", "package a;\npublic class Animal {}\n")
	write("java/a/Dog.java", "package a;\npublic class Dog extends Animal {}\n")
	// TS: cross-module extends via a relative import.
	write("ts/base.ts", "export class Base {}\n")
	write("ts/client.ts", "import { Base } from './base';\nexport class Derived extends Base {}\n")
	// Go: same-package cross-file struct embed.
	write("golang/base.go", "package p\ntype Animal struct{}\n")
	write("golang/derived.go", "package p\ntype Dog struct{ Animal }\n")

	astCfg, err := json.Marshal(map[string]any{
		"watch_paths": []map[string]any{
			{"path": filepath.Join(root, "java"), "org": "acme", "project": "ml", "languages": []string{"java"}},
			{"path": filepath.Join(root, "ts"), "org": "acme", "project": "ml", "languages": []string{"typescript"}},
			{"path": filepath.Join(root, "golang"), "org": "acme", "project": "ml", "languages": []string{"go"}},
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

	cases := []struct {
		name    string
		prefix  string
		base    string
		derived string
		role    string
	}{
		{"java", "acme.semsource.java.ml", "Animal", "Dog", "extended_by"},
		{"typescript", "acme.semsource.typescript.ml", "Base", "Derived", "extended_by"},
		{"golang", "acme.semsource.golang.ml", "Animal", "Dog", "embedded_by"},
	}

	for _, tcCase := range cases {
		t.Run(tcCase.name, func(t *testing.T) {
			var dependents []fusion.Ref
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				resp, ferr := engine.Fuse(ctx, fusion.Request{
					Query: tcCase.prefix,
					Want:  []fusion.Want{fusion.WantRelations},
				}, lens)
				if ferr != nil {
					if fuseErrIsRetryable(t, ferr) {
						time.Sleep(100 * time.Millisecond)
						continue
					}
				}
				if resp.Index.Ready {
					if base := findNode(resp.Nodes, tcCase.base); base != nil {
						if got := base.Relations[tcCase.role]; len(got) > 0 {
							dependents = got
							break
						}
					}
				}
				time.Sleep(100 * time.Millisecond)
			}
			if len(dependents) == 0 {
				t.Fatalf("%s: no %s dependents for %s — cross-file edge did not resolve live",
					tcCase.name, tcCase.role, tcCase.base)
			}
			found := false
			for _, d := range dependents {
				if d.Name == tcCase.derived {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s: %s.%s = %+v, want to contain %s",
					tcCase.name, tcCase.base, tcCase.role, dependents, tcCase.derived)
			}
		})
	}
}
