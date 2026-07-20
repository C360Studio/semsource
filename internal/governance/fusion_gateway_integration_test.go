//go:build integration

package governance

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/pkg/fusion/fusionnats"
	"github.com/c360studio/semstreams/storage"
	"github.com/c360studio/semstreams/storage/objectstore"
	"github.com/c360studio/semstreams/vocabulary/cco"

	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/fusion/lens/code"
	"github.com/c360studio/semsource/source/ontology"
)

// TestIntegration_FusionNatsClientAgainstLiveGraph validates the fusionnats
// retrieval client's wire-format mappings against a REAL graph subsystem
// (graph-ingest, graph-index, graph-query) — the highest-risk, can't-unit-test
// surface. It ingests two code entities with a call edge, then asserts the
// client correctly speaks graph.index.query.status, graph.query.batch/entity,
// graph.query.relationships, and graph.query.prefix end to end.
func TestIntegration_FusionNatsClientAgainstLiveGraph(t *testing.T) {
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

	const (
		caller = "acme.semsource.golang.gw.function.dispatch"
		callee = "acme.semsource.golang.gw.function.onevent"
	)
	// Caller carries the call edge (subject=caller → object=callee).
	publishSemsourceEntity(t, ctx, tc.Client, caller, semsourcegraph.IndexingProfileContent, []message.Triple{
		{Subject: caller, Predicate: ast.DcTitle, Object: "Dispatch"},
		{Subject: caller, Predicate: ast.CodeType, Object: "function"},
		{Subject: caller, Predicate: ast.CodeCalls, Object: callee},
	})
	publishSemsourceEntity(t, ctx, tc.Client, callee, semsourcegraph.IndexingProfileContent, []message.Triple{
		{Subject: callee, Predicate: ast.DcTitle, Object: "OnEvent"},
		{Subject: callee, Predicate: ast.CodeType, Object: "function"},
	})
	waitForEntityState(t, ctx, tc.Client, caller, 5*time.Second)
	waitForEntityState(t, ctx, tc.Client, callee, 5*time.Second)

	gc := fusionnats.New(tc.Client, 0)

	t.Run("Status resolves the readiness envelope", func(t *testing.T) {
		// graph.index.query.status must decode; readiness may still be building.
		if _, err := gc.Status(ctx); err != nil {
			t.Fatalf("Status: %v", err)
		}
	})

	t.Run("Entities batch returns triples", func(t *testing.T) {
		ents, err := gc.Entities(ctx, []string{caller, callee})
		if err != nil {
			t.Fatalf("Entities: %v", err)
		}
		if len(ents) != 2 {
			t.Fatalf("expected 2 entities, got %d", len(ents))
		}
		byID := map[string]*fusion.Entity{}
		for _, e := range ents {
			byID[e.ID] = e
		}
		if byID[caller] == nil || byID[caller].First(ast.DcTitle) != "Dispatch" {
			t.Fatalf("caller entity/title missing: %+v", byID[caller])
		}
	})

	t.Run("Entity absent is clean nil", func(t *testing.T) {
		e, err := gc.Entity(ctx, "acme.semsource.golang.gw.function.ghost")
		if err != nil || e != nil {
			t.Fatalf("absent entity should be (nil,nil), got (%v,%v)", e, err)
		}
	})

	t.Run("Neighbors outgoing = callee, incoming = caller", func(t *testing.T) {
		preds := []string{ast.CodeCalls}
		// graph-index lands asynchronously; poll until the edge appears.
		out := pollNeighbors(t, ctx, gc, caller, preds, fusion.Outgoing)
		if len(out) != 1 || out[0].Target != callee {
			t.Fatalf("outgoing should be the callee %q, got %+v", callee, out)
		}
		in := pollNeighbors(t, ctx, gc, callee, preds, fusion.Incoming)
		if len(in) != 1 || in[0].Target != caller {
			t.Fatalf("incoming should be the caller %q (source as Target), got %+v", caller, in)
		}
	})

	t.Run("Resolve prefix returns the ingested IDs", func(t *testing.T) {
		ids, err := gc.Resolve(ctx, fusion.ResolveQuery{Query: "acme.semsource.golang.gw", Mode: fusion.ResolveModePrefix, Limit: 10})
		if err != nil {
			t.Fatalf("Resolve prefix: %v", err)
		}
		if !containsID(ids, caller) || !containsID(ids, callee) {
			t.Fatalf("prefix resolve missing ingested IDs, got %v", ids)
		}
	})
}

// TestIntegration_FusionPipelineEndToEnd drives the WHOLE fusion gateway against
// a live graph: real graph-ingest/index/query, a real fusion engine over the
// fusionnats client, and the code lens hydrating verbatim source by dereferencing
// its StorageReference handle against a real ObjectStore (ADR-062 increment 4 —
// the location-independent body path, no worktree). It ingests a caller→callee
// pair (the caller stamped with body handle triples), offloads the body, and
// asserts the fused response carries the readiness envelope, the ObjectStore-
// dereferenced verbatim source, the callee relation, and the BFO/CCO class.
func TestIntegration_FusionPipelineEndToEnd(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		// The GRAPH stream must bind ONLY the entity ingest subject, NOT a
		// wildcard like "graph.ingest.>": a wildcard also matches graph-query's
		// request/reply forwards and races a PubAck onto the reply inbox, silently
		// zeroing batch/prefix results (docs/upstream/semstreams-asks.md #6).
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

	// Offload the verbatim body to a real ObjectStore and address it by handle —
	// the producer side of the hydration contract. The engine's BodyResolver
	// Gets it back through the same store.
	store, err := objectstore.NewStoreWithConfig(ctx, tc.Client, objectstore.Config{
		BucketName:   semsourcegraph.BodyStoreBucket,
		InstanceName: semsourcegraph.BodyStoreInstance,
	})
	if err != nil {
		t.Fatalf("objectstore: %v", err)
	}
	const dispatchBody = "func Dispatch() {\n\tOnEvent()\n}"
	const bodyKey = "body:dispatch"
	if err := store.Put(ctx, bodyKey, []byte(dispatchBody)); err != nil {
		t.Fatalf("put body: %v", err)
	}

	const (
		caller = "acme.semsource.golang.gw.function.dispatch"
		callee = "acme.semsource.golang.gw.function.onevent"
	)
	publishSemsourceEntity(t, ctx, tc.Client, caller, semsourcegraph.IndexingProfileContent, []message.Triple{
		{Subject: caller, Predicate: ast.DcTitle, Object: "Dispatch"},
		{Subject: caller, Predicate: ast.CodeType, Object: "function"},
		{Subject: caller, Predicate: ast.CodePath, Object: "svc.go"},
		{Subject: caller, Predicate: ast.CodeStartLine, Object: 3},
		{Subject: caller, Predicate: ast.CodeEndLine, Object: 5},
		{Subject: caller, Predicate: ast.CodeCalls, Object: callee},
		// Body handle triples: the code lens reads these into a StorageReference.
		{Subject: caller, Predicate: ast.CodeBodyStore, Object: semsourcegraph.BodyStoreInstance},
		{Subject: caller, Predicate: ast.CodeBodyKey, Object: bodyKey},
		// Stamp SoftwareCode, NOT Algorithm: a `function` entity's ID-derived
		// fallback is cco.Algorithm, so stamping Algorithm would let the class
		// assertion pass via the fallback even if the stamped triple were dropped.
		{Subject: caller, Predicate: ontology.ClassPredicate, Object: cco.SoftwareCode},
	})
	publishSemsourceEntity(t, ctx, tc.Client, callee, semsourcegraph.IndexingProfileContent, []message.Triple{
		{Subject: callee, Predicate: ast.DcTitle, Object: "OnEvent"},
		{Subject: callee, Predicate: ast.CodeType, Object: "function"},
		{Subject: callee, Predicate: ast.CodePath, Object: "svc.go"},
		{Subject: callee, Predicate: ast.CodeStartLine, Object: 7},
		{Subject: callee, Predicate: ast.CodeEndLine, Object: 7},
		{Subject: callee, Predicate: ontology.ClassPredicate, Object: cco.Algorithm},
	})
	waitForEntityState(t, ctx, tc.Client, caller, 5*time.Second)
	waitForEntityState(t, ctx, tc.Client, callee, 5*time.Second)

	engine := fusion.NewEngine(
		fusionnats.New(tc.Client, 0),
		fusion.NewBodyResolver(fusion.MapStoreResolver{semsourcegraph.BodyStoreInstance: storage.Store(store)}),
	)
	// Use the REAL code lens for edges/fields/hydration, but force prefix
	// resolution so the seed resolves deterministically via graph.query.prefix
	// (the symbol path needs graph-embedding, validated elsewhere). The query is
	// the shared ID prefix, matching exactly these entities.
	lens := prefixLens{code.New()}
	const prefix = "acme.semsource.golang.gw"

	var resp fusion.Response
	var dispatch *fusion.Node
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r, err := engine.Fuse(ctx, fusion.Request{
			Query: prefix,
			Want:  []fusion.Want{fusion.WantBody, fusion.WantRelations},
		}, lens)
		if err != nil {
			if fuseErrIsRetryable(t, err) {
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}
		resp = r
		dispatch = findNode(resp.Nodes, "Dispatch")
		if resp.Index.Ready && dispatch != nil && len(dispatch.Relations["callee"]) == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !resp.Index.Ready {
		t.Fatalf("expected ready index, got %+v", resp.Index)
	}
	if dispatch == nil {
		t.Fatalf("no Dispatch node in fused response: %+v", resp.Nodes)
	}
	if dispatch.Body != dispatchBody {
		t.Fatalf("verbatim body mismatch (want ObjectStore deref): %q", dispatch.Body)
	}
	if got := dispatch.Relations["callee"]; len(got) != 1 || got[0].Name != "OnEvent" {
		t.Fatalf("expected callee OnEvent, got %+v", dispatch.Relations)
	}
	// SoftwareCode is the stamped class (≠ the function→Algorithm fallback), so
	// this proves the entity.ontology.class triple flowed end to end.
	if dispatch.Class != cco.SoftwareCode {
		t.Fatalf("expected stamped SoftwareCode class, got %q", dispatch.Class)
	}
	// The honesty envelope: prefix/structural fusion is deterministic — no
	// embedding or LLM in the path. This is the core ADR-0004 claim.
	if resp.Provenance != fusion.ProvenanceDeterministic {
		t.Fatalf("expected deterministic provenance, got %q", resp.Provenance)
	}
}

// findNode returns the node with the given name, or nil.
func findNode(nodes []fusion.Node, name string) *fusion.Node {
	for i := range nodes {
		if nodes[i].Name == name {
			return &nodes[i]
		}
	}
	return nil
}

// prefixLens wraps the real code lens but forces prefix resolution, so the
// end-to-end test resolves seeds via graph.query.prefix (deterministic, no
// graph-embedding) while still exercising the real lens's edges, field
// extraction, and handle hydration.
type prefixLens struct{ *code.Lens }

func (prefixLens) ResolveMode(string) fusion.ResolveMode { return fusion.ResolveModePrefix }

// pollNeighbors retries Neighbors until it returns edges or the deadline passes
// (graph-index builds the OUTGOING/INCOMING indexes asynchronously). The
// deadline is sized for a cold CI runner, not a warm laptop: the 5s original
// flaked once in four runs on the first day test-integration gated CI (PR #98,
// same code green on the surrounding three runs).
func pollNeighbors(t *testing.T, ctx context.Context, gc *fusionnats.Client, id string, preds []string, dir fusion.Direction) []fusion.Edge {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		edges, err := gc.Neighbors(ctx, id, preds, dir)
		if err != nil {
			t.Fatalf("Neighbors: %v", err)
		}
		if len(edges) > 0 {
			return edges
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("Neighbors(%s, %v) never returned an edge", id, dir)
	return nil
}

func containsID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
