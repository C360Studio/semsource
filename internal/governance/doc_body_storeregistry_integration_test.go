//go:build integration

package governance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/pkg/fusion/fusionnats"
	"github.com/c360studio/semstreams/storage/objectstore"
	"github.com/c360studio/semstreams/storage/storeregistry"

	semsourcegraph "github.com/c360studio/semsource/graph"
	dochandler "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/source/fusion/lens/docs"
)

// TestIntegration_DocBodyOffload_ResolvesViaStoreRegistry proves the ADR-063
// store-registry unification for documents end to end. The REAL doc handler
// offloads a passage to the CONTENT ObjectStore and wires it to a SINGLE blob two
// ways — EntityState.StorageRef (graph-embedding's fetch path) and the
// DocBodyStore/DocBodyKey handle triples (the fusion docs lens) — and BOTH
// consumers resolve that one blob through the SHARED StoreRegistry:
//
//   - Assertion A mirrors graph-embedding's StorageRef path (shouldFetchViaStorageRef
//     → queueEmbeddingWithStorageRef): the registry resolves StorageRef.StorageInstance
//     and Get returns the verbatim body. A registry miss here is precisely the
//     content_unresolved case graph-embedding reports loudly — so a pass means that
//     metric stays 0 for offloaded doc bodies.
//   - Assertion B drives the fusion docs engine over the registry resolver (the
//     code-context wiring adopted in this slice) and asserts the hydrated node body
//     is the verbatim passage.
func TestIntegration_DocBodyOffload_ResolvesViaStoreRegistry(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		// Bind ONLY the entity ingest subject, never a wildcard — a wildcard also
		// matches graph-query's request/reply forwards and races a PubAck onto the
		// reply inbox (docs/upstream/semstreams-asks.md #6).
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

	// One CONTENT store, registered in the shared registry exactly as the
	// ComponentManager populates it after the objectstore component Starts (ADR-063).
	store, err := objectstore.NewStoreWithConfig(ctx, tc.Client, objectstore.Config{
		BucketName:   semsourcegraph.BodyStoreBucket,
		InstanceName: semsourcegraph.BodyStoreInstance,
	})
	if err != nil {
		t.Fatalf("objectstore: %v", err)
	}
	storeReg := storeregistry.New()
	if err := storeReg.Register(semsourcegraph.BodyStoreInstance, store); err != nil {
		t.Fatalf("register store: %v", err)
	}

	// Drive the REAL doc producer over the same store: it offloads the passage to
	// CONTENT and sets BOTH the StorageRef and the body-handle triples.
	const docContent = "# Retry Policy\n\nUse exponential backoff with jitter.\n"
	docDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(docDir, "retry.md"), []byte(docContent), 0o644); err != nil {
		t.Fatal(err)
	}
	h := dochandler.New(dochandler.WithBodyStore(store, semsourcegraph.BodyStoreInstance))
	states, err := h.IngestEntityStates(ctx, docSourceConfig{typ: "docs", path: docDir}, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("state count: got %d, want 1", len(states))
	}
	state := states[0]

	// ---- Assertion A: the graph-embedding StorageRef→registry fetch contract. ----
	if state.StorageRef == nil {
		t.Fatal("doc producer did not set EntityState.StorageRef")
	}
	resolved, ok := storeReg.Store(state.StorageRef.StorageInstance)
	if !ok {
		t.Fatalf("registry does not resolve StorageInstance %q — graph-embedding would report content_unresolved",
			state.StorageRef.StorageInstance)
	}
	body, err := resolved.Get(ctx, state.StorageRef.Key)
	if err != nil || string(body) != docContent {
		t.Fatalf("registry fetch of offloaded body = %q (err %v); want %q", body, err, docContent)
	}

	// Publish the produced entity (triples + StorageRef) into the live graph.
	publishSemsourceEntity(t, ctx, tc.Client, state.ID,
		semsourcegraph.IndexingProfileContent, state.Triples, state.StorageRef)
	waitForEntityState(t, ctx, tc.Client, state.ID, 5*time.Second)

	// ---- Assertion B: the fusion docs lens hydrates via the registry resolver. ----
	engine := fusion.NewEngine(
		fusionnats.New(tc.Client, 0),
		fusion.NewBodyResolver(storeReg), // the shared registry, not a MapStoreResolver
	)
	lens := docsPrefixLens{docs.New()}

	var node *fusion.Node
	var last fusion.Response
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, ferr := engine.Fuse(ctx, fusion.Request{
			Query: state.ID, Want: []fusion.Want{fusion.WantBody},
		}, lens)
		if ferr != nil {
			t.Fatalf("Fuse: %v", ferr)
		}
		last = resp
		if n := findNode(resp.Nodes, "Retry Policy"); n != nil && n.Body != "" {
			node = n
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if node == nil {
		t.Fatalf("doc node with a hydrated body never appeared — ready=%v state=%s nodes=%+v",
			last.Index.Ready, last.Index.State, last.Nodes)
	}
	if node.Body != docContent {
		t.Fatalf("hydrated doc body via registry resolver = %q; want %q", node.Body, docContent)
	}
}

// docsPrefixLens wraps the real docs lens but forces prefix resolution so the
// seed resolves deterministically via graph.query.prefix (the docs lens's default
// NL/semantic path needs graph-embedding, validated separately).
type docsPrefixLens struct{ *docs.Lens }

func (docsPrefixLens) ResolveMode(string) fusion.ResolveMode { return fusion.ResolveModePrefix }

// docSourceConfig is a minimal handler.SourceConfig for driving the doc handler.
type docSourceConfig struct {
	typ  string
	path string
}

func (c docSourceConfig) GetType() string           { return c.typ }
func (c docSourceConfig) GetPath() string           { return c.path }
func (docSourceConfig) GetPaths() []string          { return nil }
func (docSourceConfig) GetURL() string              { return "" }
func (docSourceConfig) GetBranch() string           { return "" }
func (docSourceConfig) IsWatchEnabled() bool        { return false }
func (docSourceConfig) GetKeyframeMode() string     { return "" }
func (docSourceConfig) GetKeyframeInterval() string { return "" }
func (docSourceConfig) GetSceneThreshold() float64  { return 0 }
