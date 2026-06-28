//go:build integration

package governance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/vocabulary/cco"

	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/fusion"
	"github.com/c360studio/semsource/source/fusion/lens/code"
	"github.com/c360studio/semsource/source/fusion/natsgraph"
	"github.com/c360studio/semsource/source/ontology"
)

// TestIntegration_NatsGraphClientAgainstLiveGraph validates the fusion natsgraph
// client's wire-format mappings against a REAL graph subsystem (graph-ingest,
// graph-index, graph-query) — the highest-risk, can't-unit-test surface. It
// ingests two code entities with a call edge, then asserts the client correctly
// speaks graph.query.batch, graph.index.query.{outgoing,incoming}, and
// graph.query.prefix end to end.
func TestIntegration_NatsGraphClientAgainstLiveGraph(t *testing.T) {
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

	gc := natsgraph.New(tc.Client)

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
		ids, err := gc.Resolve(ctx, "acme.semsource.golang.gw", fusion.ResolvePrefix, 10)
		if err != nil {
			t.Fatalf("Resolve prefix: %v", err)
		}
		if !containsID(ids, caller) || !containsID(ids, callee) {
			t.Fatalf("prefix resolve missing ingested IDs, got %v", ids)
		}
	})
}

// TestIntegration_FusionPipelineEndToEnd drives the WHOLE fusion gateway against
// a live graph: a ready graph.query.status, real graph-ingest/index/query,
// a real fusion engine over the natsgraph client, and the code lens hydrating
// verbatim source from disk. It ingests a caller→callee pair and asserts the
// fused response carries the readiness envelope, verbatim source, the callee
// relation, and the BFO/CCO class — i.e. the full code_context shape, no fake.
func TestIntegration_FusionPipelineEndToEnd(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		// The GRAPH stream must bind ONLY the entity ingest subject, NOT a
		// wildcard like "graph.ingest.>". graph-query forwards batch/prefix
		// queries to graph-ingest over graph.ingest.query.{batch,prefix} as core
		// request/reply. A "graph.ingest.>" stream also matches those request
		// subjects, so the broker sends a PubAck to the query's reply inbox the
		// instant the message lands in the stream; that PubAck wins the race
		// against graph-ingest's real reply, graph-query unmarshals it (a
		// JetStream ack, no entities), and batch/prefix silently return zero
		// results. This is the read-path twin of the curator footgun guarded by
		// warnIfHostStreamCapturesRPCReplySubjects in cmd/semsource/run.go.
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

	// Serve graph.query.status as a ready stub (source-manifest's readiness is
	// covered elsewhere; here we isolate the fusion path).
	statusSub, err := tc.Client.SubscribeForRequests(ctx, "graph.query.status", func(_ context.Context, _ []byte) ([]byte, error) {
		return []byte(`{"phase":"ready"}`), nil
	})
	if err != nil {
		t.Fatalf("subscribe status stub: %v", err)
	}
	t.Cleanup(func() { _ = statusSub.Unsubscribe() })

	// A real worktree for the code lens to hydrate from.
	root := t.TempDir()
	src := "package svc\n\nfunc Dispatch() {\n\tOnEvent()\n}\n\nfunc OnEvent() {}\n"
	if err := os.WriteFile(filepath.Join(root, "svc.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
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
		// Stamp SoftwareCode, NOT Algorithm: a `function` entity's ID-derived
		// fallback is cco.Algorithm, so stamping Algorithm would let the class
		// assertion pass via the fallback even if the stamped triple were
		// dropped. SoftwareCode can only reach the response through the real
		// entity.ontology.class read path — making the assertion load-bearing.
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

	engine := fusion.NewEngine(natsgraph.New(tc.Client))
	// Use the REAL code lens for edges/fields/hydration, but force prefix
	// resolution so the seed resolves deterministically via graph.query.prefix
	// (the symbol path needs graph-embedding, validated elsewhere). The query is
	// the caller's full ID, which prefix-matches exactly that one entity.
	lens := prefixLens{code.New(root)}

	// Query the shared ID prefix (matches both entities); poll until status is
	// ready AND graph-index has surfaced the call edge on the Dispatch node.
	const prefix = "acme.semsource.golang.gw"

	var resp fusion.Response
	var dispatch *fusion.Node
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, err := engine.Fuse(ctx, fusion.Request{
			Query: prefix, Repo: root,
			Want: []fusion.Want{fusion.WantBody, fusion.WantRelations},
		}, lens)
		if err != nil {
			t.Fatalf("Fuse: %v", err)
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
	if dispatch.Body != "func Dispatch() {\n\tOnEvent()\n}" {
		t.Fatalf("verbatim body mismatch: %q", dispatch.Body)
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
// extraction, and disk hydration.
type prefixLens struct{ *code.Lens }

func (prefixLens) ResolveMode(string) fusion.ResolveMode { return fusion.ResolvePrefix }

// pollNeighbors retries Neighbors until it returns edges or the deadline passes
// (graph-index builds the OUTGOING/INCOMING indexes asynchronously).
func pollNeighbors(t *testing.T, ctx context.Context, gc *natsgraph.Client, id string, preds []string, dir fusion.Direction) []fusion.Edge {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
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
