//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/types"

	"github.com/c360studio/semsource/entityid"
	semsourcegraph "github.com/c360studio/semsource/graph"
	dochandler "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/internal/natstest"
	"github.com/c360studio/semsource/internal/testutil"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	docsource "github.com/c360studio/semsource/processor/doc-source"
	"github.com/c360studio/semsource/processor/supersession"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// TestIntegration_ASTSweepDoesNotMarkLiveDocsOfSharedProject is the #286
// probe, kept: an AST source at <root> and a docs source at <root>/docs share
// one project, so one system slug. The AST source's periodic sweeps must not
// mark the live document stale — its path is relative to the docs root and
// means nothing under the AST root.
//
// Synchronized, not slept: the test observes the AST source's own lifecycle
// requests on the trigger subject, and once two have been sent it runs one
// more pass synchronously with the AST-shaped request. The pass gate
// serializes passes, so by the time that reply arrives the sweeps before it
// have been judged.
// spec: openspec/specs/entity-staleness/spec.md#Requirement: A staleness sweep judges only the entities its own root produced
func TestIntegration_ASTSweepDoesNotMarkLiveDocsOfSharedProject(t *testing.T) {
	ctx := context.Background()
	tc, deps := startLifecycleStack(t, ctx)

	const org, project = "acme", "svc"
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "live.go"), []byte("package pkg\n\nfunc Live() int {\n\treturn 1\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "a.md"), []byte("# A\n\nStill here.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	startDocSource(t, ctx, deps, org, project, docs)

	system := entityid.ScopedSystemSlug(project, "")
	docID := dochandler.DocumentEntityID(fixtureAuthority(), system, "a.md")
	if waitForEntityState(t, ctx, tc.Client, docID, 30*time.Second) == nil {
		t.Fatalf("document %s never reached ENTITY_STATES", docID)
	}

	// The stamp and the sweep must agree on the owner key, or no sweep would
	// ever judge this document.
	if stored, ok := fetchEntity(ctx, tc.Client, docID); !ok ||
		tripleObject(stored.Triples, source.EntityLifecycleOwner) != semsourcegraph.LifecycleOwner("doc-source-svc", docs) {
		t.Fatalf("document %s does not carry its root's owner fact", docID)
	}

	// Count the AST source's own lifecycle requests. A plain subscriber sees
	// every request without answering it; supersession still replies.
	var astSweeps atomic.Int32
	observer, err := tc.Client.Subscribe(ctx, semsourcegraph.LifecycleTriggerSubject, func(_ context.Context, m *nats.Msg) {
		var req semsourcegraph.LifecycleRunRequest
		if json.Unmarshal(m.Data, &req) == nil && req.RootPath == root {
			astSweeps.Add(1)
		}
	})
	if err != nil {
		t.Fatalf("observe lifecycle trigger: %v", err)
	}
	t.Cleanup(func() { _ = observer.Unsubscribe() })

	astCfg, _ := json.Marshal(map[string]any{
		"watch_paths":    []map[string]any{{"path": root, "org": org, "project": project, "languages": []string{"go"}}},
		"instance_name":  "ast-source-svc",
		"watch_enabled":  true,
		"index_interval": "1s",
	})
	ad, err := astsource.NewComponent(astCfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	astComp := ad.(*astsource.Component)
	if err := astComp.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := astComp.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stopWithin(5*time.Second, astComp.Stop) })

	if !testutil.WaitFor(t, 30*time.Second, func() bool { return astSweeps.Load() >= 2 }) {
		t.Fatalf("the AST source sent %d lifecycle sweeps, want at least 2", astSweeps.Load())
	}
	resp, err := semsourcegraph.PublishLifecycleTrigger(ctx, tc.Client, semsourcegraph.LifecycleRunRequest{
		Org: org, Systems: []string{system}, RootPath: root,
		Reason: semsourcegraph.LifecycleReasonPathMissing, Owner: semsourcegraph.LifecycleOwner("ast-source-svc", root),
	})
	if err != nil {
		t.Fatalf("synchronous AST-shaped pass: %v", err)
	}
	if resp.Marked != 0 {
		t.Errorf("AST-shaped pass marked %d entities, want 0: every file under %s exists", resp.Marked, root)
	}
	requireNotStale(t, ctx, tc, docID, filepath.Join(docs, "a.md"))

	// Positive control, the reverse direction: a docs-shaped pass under the
	// wrong root does mark the document, so the assertion above can see a
	// marker when one is written — and it never touches the code entities.
	resp, err = semsourcegraph.PublishLifecycleTrigger(ctx, tc.Client, semsourcegraph.LifecycleRunRequest{
		Org: org, Systems: []string{system}, RootPath: filepath.Join(root, "nowhere"),
		Reason: semsourcegraph.LifecycleReasonPathMissing, Owner: semsourcegraph.LifecycleOwner("doc-source-svc", docs),
	})
	if err != nil {
		t.Fatalf("docs-shaped control pass: %v", err)
	}
	if resp.Marked == 0 {
		t.Fatal("control: a docs pass under a root without a.md marked nothing; the stale assertion cannot see markers")
	}
	e, ok := fetchEntity(ctx, tc.Client, docID)
	if !ok || !hasStaleMarker(e.Triples) {
		t.Fatalf("control: document %s carries no stale marker after a docs pass under the wrong root", docID)
	}
	if resp.Entities != resp.Marked {
		t.Errorf("control pass judged %d entities but marked %d: only the document and its passages answer to a docs root",
			resp.Entities, resp.Marked)
	}
}

func requireNotStale(t *testing.T, ctx context.Context, tc *natstest.Server, id, file string) {
	t.Helper()
	e, ok := fetchEntity(ctx, tc.Client, id)
	if !ok {
		t.Fatalf("entity %s not readable", id)
	}
	if hasStaleMarker(e.Triples) {
		t.Fatalf("live document %s was marked stale by another root's sweep; its file %s still exists", id, file)
	}
}

func hasStaleMarker(triples []message.Triple) bool {
	for _, tr := range triples {
		if tr.Predicate == source.EntityLifecycleStale {
			return true
		}
	}
	return false
}

func tripleObject(triples []message.Triple, predicate string) any {
	for _, tr := range triples {
		if tr.Predicate == predicate {
			return tr.Object
		}
	}
	return nil
}

// startLifecycleStack starts graph-ingest, graph-index, graph-query and the
// supersession (lifecycle) component on a fresh broker.
func startLifecycleStack(t *testing.T, ctx context.Context) (*natstest.Server, component.Dependencies) {
	t.Helper()
	tc := natstest.New(t, natstest.WithJetStream(),
		natstest.WithStreams(natstest.Stream{Name: "GRAPH", Subjects: []string{"graph.ingest.entity"}}))
	if _, err := BootstrapStandalone(nil); err != nil {
		t.Fatal(err)
	}
	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatal(err)
	}
	mr := metric.NewMetricsRegistry()
	ingest := startGraphIngest(t, ctx, tc.Client, reg, mr)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, ingest.Stop) })
	index := startGraphIndex(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, index.Stop) })
	q := startGraphQuery(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, q.Stop) })

	deps := component.Dependencies{NATSClient: tc.Client, Platform: types.PlatformMeta{Org: "acme", Platform: "test-a1b2c3"}}
	scfg, _ := json.Marshal(map[string]any{"max_entities": 1000})
	sd, err := supersession.NewComponent(scfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	scomp := sd.(*supersession.Component)
	if err := scomp.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stopWithin(5*time.Second, scomp.Stop) })
	return tc, deps
}

// startDocSource starts one non-watching doc-source instance, "doc-source-svc",
// over paths.
func startDocSource(t *testing.T, ctx context.Context, deps component.Dependencies, org, project string, paths ...string) {
	t.Helper()
	docCfg, _ := json.Marshal(map[string]any{
		"paths": paths, "org": org, "project": project,
		"watch_enabled": false, "instance_name": "doc-source-svc",
	})
	dd, err := docsource.NewComponent(docCfg, deps)
	if err != nil {
		t.Fatalf("doc-source NewComponent: %v", err)
	}
	docComp := dd.(*docsource.Component)
	if err := docComp.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := docComp.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stopWithin(5*time.Second, docComp.Stop) })
}

// spec: openspec/specs/entity-staleness/spec.md#Requirement: A staleness sweep judges only the entities its own root produced
// TestIntegration_TwoDocsRootsOfOneProjectDoNotCrossMark is the case the
// #293 review reproduced: one doc-source instance over two roots of one
// project, holding a.md and b.md. The IDs differ, so nothing collides, but a
// sweep scoped by system alone would stat b.md under root A and mark it.
// Root A's sweep must leave b.md alone and still mark its own missing a.md.
func TestIntegration_TwoDocsRootsOfOneProjectDoNotCrossMark(t *testing.T) {
	ctx := context.Background()
	tc, deps := startLifecycleStack(t, ctx)

	const org, project = "acme", "svc"
	base := t.TempDir()
	rootA, rootB := filepath.Join(base, "a"), filepath.Join(base, "b")
	for root, name := range map[string]string{rootA: "a.md", rootB: "b.md"} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte("# "+name+"\n\nStill here.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	startDocSource(t, ctx, deps, org, project, rootA, rootB)

	system := entityid.ScopedSystemSlug(project, "")
	idA := dochandler.DocumentEntityID(fixtureAuthority(), system, "a.md")
	idB := dochandler.DocumentEntityID(fixtureAuthority(), system, "b.md")
	for _, id := range []string{idA, idB} {
		if waitForEntityState(t, ctx, tc.Client, id, 30*time.Second) == nil {
			t.Fatalf("document %s never reached ENTITY_STATES", id)
		}
	}

	sweepA := func() *semsourcegraph.LifecycleRunResponse {
		t.Helper()
		resp, err := semsourcegraph.PublishLifecycleTrigger(ctx, tc.Client, semsourcegraph.LifecycleRunRequest{
			Org: org, Systems: []string{system}, RootPath: rootA,
			Reason: semsourcegraph.LifecycleReasonPathMissing, Owner: semsourcegraph.LifecycleOwner("doc-source-svc", rootA),
		})
		if err != nil {
			t.Fatalf("root A sweep: %v", err)
		}
		return resp
	}
	if resp := sweepA(); resp.Marked != 0 {
		t.Errorf("root A's sweep marked %d entities while every file exists", resp.Marked)
	}
	requireNotStale(t, ctx, tc, idB, filepath.Join(rootB, "b.md"))

	// Root A's own missing artifact is still marked; b.md still is not.
	if err := os.Remove(filepath.Join(rootA, "a.md")); err != nil {
		t.Fatal(err)
	}
	if resp := sweepA(); resp.Marked == 0 {
		t.Fatal("root A's sweep did not mark its own deleted a.md")
	}
	if e, ok := fetchEntity(ctx, tc.Client, idA); !ok || !hasStaleMarker(e.Triples) {
		t.Errorf("document %s carries no stale marker after its file was deleted", idA)
	}
	requireNotStale(t, ctx, tc, idB, filepath.Join(rootB, "b.md"))
}
