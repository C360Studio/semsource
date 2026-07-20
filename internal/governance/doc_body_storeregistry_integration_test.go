//go:build integration

package governance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	"github.com/c360studio/semsource/handler"
	dochandler "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/source/fusion/lens/docs"
	source "github.com/c360studio/semsource/source/vocabulary"
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

	// Drive the REAL doc producer over the same store. It emits a parent document
	// plus one entity per passage; each PASSAGE is offloaded to CONTENT and gets
	// both a StorageRef and the body-handle triples. The document is deliberately
	// large enough to split — with a single passage you cannot tell "the parent
	// holds no body" from "the parent's body happens to equal the only passage".
	docContent := "# Retry Policy\n\n" + strings.Repeat("Use exponential backoff with jitter. ", 20) +
		"\n\n## Deadlines\n\n" + strings.Repeat("Every call carries a deadline from its caller. ", 20) + "\n"
	docDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(docDir, "retry.md"), []byte(docContent), 0o644); err != nil {
		t.Fatal(err)
	}
	h := dochandler.New(dochandler.WithBodyStore(store, semsourcegraph.BodyStoreInstance))
	states, err := h.IngestEntityStates(ctx, docSourceConfig{typ: "docs", path: docDir}, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}

	var parent *handler.EntityState
	var passages []*handler.EntityState
	for _, st := range states {
		if docTypeOfState(st) == "passage" {
			passages = append(passages, st)
			continue
		}
		parent = st
	}
	if parent == nil {
		t.Fatal("no parent document entity emitted")
	}
	if len(passages) < 2 {
		t.Fatalf("want a multi-passage document, got %d passages from %d states", len(passages), len(states))
	}

	// ---- Assertion A: the graph-embedding StorageRef→registry fetch contract. ----
	// Every passage must resolve; a passage whose ref the registry cannot resolve
	// is exactly the content_unresolved case graph-embedding reports loudly, so a
	// pass here means that metric stays 0 for offloaded doc bodies.
	var rebuilt string
	for _, p := range passages {
		if p.StorageRef == nil {
			t.Fatalf("passage %s carries no StorageRef — it would never be embedded", p.ID)
		}
		resolved, ok := storeReg.Store(p.StorageRef.StorageInstance)
		if !ok {
			t.Fatalf("registry does not resolve StorageInstance %q for %s", p.StorageRef.StorageInstance, p.ID)
		}
		body, gErr := resolved.Get(ctx, p.StorageRef.Key)
		if gErr != nil {
			t.Fatalf("registry fetch for %s: %v", p.ID, gErr)
		}
		rebuilt += string(body)
	}
	// The stored passages must account for the whole document: anything missing
	// here is document text that no query can ever reach.
	if rebuilt != docContent {
		t.Fatalf("passage bodies do not reconstruct the document (%d bytes stored vs %d on disk)",
			len(rebuilt), len(docContent))
	}

	// The parent holds no body, so it cannot reintroduce the diluted whole-file
	// vector or return the same prose a second time.
	if parent.StorageRef != nil {
		t.Fatalf("parent %s still carries a StorageRef %+v; the parent holds no body", parent.ID, parent.StorageRef)
	}

	// Publish parent and passages (triples + StorageRefs) into the live graph.
	for _, st := range states {
		publishSemsourceEntity(t, ctx, tc.Client, st.ID,
			semsourcegraph.IndexingProfileContent, st.Triples, st.StorageRef)
	}
	for _, st := range states {
		waitForEntityState(t, ctx, tc.Client, st.ID, 5*time.Second)
	}
	state := passages[0]
	passageLabel := labelOfState(state)

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
			if fuseErrIsRetryable(t, ferr) {
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}
		last = resp
		if n := findNode(resp.Nodes, passageLabel); n != nil && n.Body != "" {
			node = n
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if node == nil {
		t.Fatalf("doc node with a hydrated body never appeared — ready=%v state=%s nodes=%+v",
			last.Index.Ready, last.Index.State, last.Nodes)
	}
	wantBody, gErr := store.Get(ctx, state.StorageRef.Key)
	if gErr != nil {
		t.Fatalf("read expected passage body: %v", gErr)
	}
	if node.Body != string(wantBody) {
		t.Fatalf("hydrated passage body via registry resolver = %q; want %q", node.Body, wantBody)
	}
	// The passage, not the file: a hydrated body equal to the whole document
	// would mean the split never reached the retrieval path.
	if node.Body == docContent {
		t.Fatal("hydrated body is the entire document; retrieval is still whole-file")
	}
}

// docTypeOfState reads the emitted DocType fact off a produced entity state.
func docTypeOfState(st *handler.EntityState) string {
	for i := range st.Triples {
		if st.Triples[i].Predicate == source.DocType {
			v, _ := st.Triples[i].Object.(string)
			return v
		}
	}
	return ""
}

// labelOfState reads the emitted title, which is what the lens uses as a node label.
func labelOfState(st *handler.EntityState) string {
	for i := range st.Triples {
		if st.Triples[i].Predicate == source.DcTitle {
			v, _ := st.Triples[i].Object.(string)
			return v
		}
	}
	return ""
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
