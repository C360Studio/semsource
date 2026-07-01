//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/pkg/fusion/fusionnats"
	"github.com/c360studio/semstreams/storage"
	"github.com/c360studio/semstreams/storage/objectstore"

	semsourcegraph "github.com/c360studio/semsource/graph"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	"github.com/c360studio/semsource/source/fusion/lens/code"
)

// TestIntegration_ProducerToConsumerEndToEnd composes the two halves of the body
// hydration path that are validated separately elsewhere: the REAL ast-source
// component ingests a Go file — its producer offloads each symbol's verbatim
// source to the CONTENT ObjectStore and stamps code.body.store/key triples — and
// the fusion engine, over the SAME store, resolves "Dispatch" by name and
// dereferences the handle back to the verbatim body. No hand-stamped triples, no
// manual Put: the producer → consumer seam end to end, including the byName path
// the label-alias fix restored.
func TestIntegration_ProducerToConsumerEndToEnd(t *testing.T) {
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

	// A real Go source tree for ast-source to index.
	root := t.TempDir()
	src := "package svc\n\n// Dispatch fans an event out.\nfunc Dispatch() {\n\tOnEvent()\n}\n\nfunc OnEvent() {}\n"
	if err := os.WriteFile(filepath.Join(root, "svc.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	// Drive the REAL ast-source component. Its Start runs the initial index, whose
	// producer offloads bodies to the CONTENT store and stamps the handle triples.
	astCfg, err := json.Marshal(map[string]any{
		"watch_paths": []map[string]any{
			{"path": root, "org": "acme", "project": "svc", "languages": []string{"go"}},
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

	// The engine derefs handles over the SAME CONTENT store the producer wrote.
	store, err := objectstore.NewStoreWithConfig(ctx, tc.Client, objectstore.Config{
		BucketName:   bodyBucket,
		InstanceName: bodyInstance,
	})
	if err != nil {
		t.Fatalf("objectstore: %v", err)
	}
	engine := fusion.NewEngine(
		fusionnats.New(tc.Client, 0),
		fusion.NewBodyResolver(fusion.MapStoreResolver{bodyInstance: storage.Store(store)}),
	)
	lens := code.New()

	// Poll until the graph is ready, byName resolves "Dispatch", and its body has
	// been dereferenced — all three are async after ast-source Start.
	var dispatch *fusion.Node
	var last fusion.Response
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		resp, ferr := engine.Fuse(ctx, fusion.Request{
			Query: "Dispatch", Want: []fusion.Want{fusion.WantBody},
		}, lens)
		if ferr != nil {
			t.Fatalf("Fuse: %v", ferr)
		}
		last = resp
		if d := findNode(resp.Nodes, "Dispatch"); d != nil && d.Body != "" {
			dispatch = d
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if dispatch == nil {
		names := make([]string, 0, len(last.Nodes))
		for i := range last.Nodes {
			names = append(names, fmt.Sprintf("%s(bodyLen=%d)", last.Nodes[i].Name, len(last.Nodes[i].Body)))
		}
		t.Fatalf("Dispatch node with a hydrated body never appeared — ready=%v state=%s nodes=%v misses=%+v",
			last.Index.Ready, last.Index.State, names, last.Misses)
	}
	// The body was offloaded by ast-source's producer and dereferenced by the
	// engine over the shared store — proving the full seam. It must be the real
	// verbatim source, not a placeholder.
	if !strings.Contains(dispatch.Body, "func Dispatch()") {
		t.Fatalf("hydrated body is not the verbatim source: %q", dispatch.Body)
	}
}
