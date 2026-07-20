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
	semgraph "github.com/c360studio/semstreams/graph"
	queryclient "github.com/c360studio/semstreams/graph/query"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/pkg/fusion/fusionnats"
	"github.com/c360studio/semstreams/pkg/fusion/fusionvocab"

	"github.com/c360studio/semsource/entityid"
	semsourcegraph "github.com/c360studio/semsource/graph"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	"github.com/c360studio/semsource/processor/supersession"
	"github.com/c360studio/semsource/source/fusion/lens/code"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// TestIntegration_StalenessLifecycle proves the entity-staleness spec end to
// end over a real graph stack + a real ast-source component:
//
//  1. deleting a watched file triggers (via ast-source's fsnotify fast path)
//     the staleness lifecycle pass, which marks the deleted symbol's entity
//     entity.lifecycle.stale=file_deleted, and the marker's negative salience
//     demotes it below a live sibling in fusion ranking (both retained);
//  2. recreating the file and re-running the lifecycle pass clears the
//     marker;
//  3. a remove_source-shaped trigger (no root_path) marks every in-scope
//     entity source_removed unconditionally, regardless of file presence.
func TestIntegration_StalenessLifecycle(t *testing.T) {
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

	const org, project = "acme", "svc"
	root := t.TempDir()
	livePath := filepath.Join(root, "live.go")
	deletedPath := filepath.Join(root, "deleted.go")
	if err := os.WriteFile(livePath, []byte("package pkg\n\nfunc Live() int {\n\treturn 1\n}\n"), 0o644); err != nil {
		t.Fatalf("write live.go: %v", err)
	}
	if err := os.WriteFile(deletedPath, []byte("package pkg\n\nfunc Deleted() int {\n\treturn 2\n}\n"), 0o644); err != nil {
		t.Fatalf("write deleted.go: %v", err)
	}

	astCfg, err := json.Marshal(map[string]any{
		"watch_paths": []map[string]any{
			{"path": root, "org": org, "project": project, "languages": []string{"go"}},
		},
		"watch_enabled":  true,
		"index_interval": "", // manual lifecycle triggering below; no periodic sweep noise
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

	scfg, _ := json.Marshal(map[string]any{"max_entities": 1000})
	sdiscovered, err := supersession.NewComponent(scfg, component.Dependencies{NATSClient: tc.Client})
	if err != nil {
		t.Fatalf("supersession NewComponent: %v", err)
	}
	scomp := sdiscovered.(*supersession.Component)
	if err := scomp.Start(ctx); err != nil {
		t.Fatalf("supersession Start: %v", err)
	}
	t.Cleanup(func() { _ = scomp.Stop(5 * time.Second) })

	qc, err := queryclient.NewClient(ctx, tc.Client, nil)
	if err != nil {
		t.Fatalf("query client: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })

	system := entityid.ScopedSystemSlug(project, "")
	prefix := org + "." + entityid.PlatformSemsource + ".golang." + system

	// Wait until both symbols are indexed by the real ast-source component.
	var liveEntity, deletedEntity *semgraph.EntityState
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		ents := prefixAll(ctx, qc, prefix)
		liveEntity = pickByNameVersion(ents, "Live", "")
		deletedEntity = pickByNameVersion(ents, "Deleted", "")
		if liveEntity != nil && deletedEntity != nil {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if liveEntity == nil || deletedEntity == nil {
		t.Fatalf("not all symbols indexed: live=%v deleted=%v", liveEntity != nil, deletedEntity != nil)
	}

	// --- (1) delete → ast-source's fsnotify fast path auto-triggers the
	// lifecycle pass, marking the deleted symbol's entity. ---
	if err := os.Remove(deletedPath); err != nil {
		t.Fatalf("remove deleted.go: %v", err)
	}
	waitTriple(t, ctx, qc, deletedEntity.ID, source.EntityLifecycleStale, source.LifecycleReasonFileDeleted, 20*time.Second)

	// The marker must never appear on the live sibling.
	if fresh, ok := fetchEntity(ctx, qc, liveEntity.ID); ok && countObject(fresh, source.EntityLifecycleStale, source.LifecycleReasonFileDeleted) > 0 {
		t.Errorf("live entity %s must not carry the staleness marker", liveEntity.ID)
	}

	// Demotion: the marker's negative salience must rank the stale entity
	// below its live sibling, while both stay retained (bounded reorder).
	// WithSignals attaches predicate-salience ranking (matching production
	// code-context wiring); without it, ranking is pure resolve-order +
	// lexical and never reflects the entity.lifecycle.stale weight at all.
	engine := fusion.NewEngine(fusionnats.New(tc.Client, 0), fusion.NewBodyResolver(fusion.MapStoreResolver{})).
		WithSignals(fusionvocab.New())
	lens := prefixLens{code.New()}
	var liveRank, deletedRank int
	rankDeadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(rankDeadline) {
		resp, ferr := engine.Fuse(ctx, fusion.Request{Query: prefix, Want: []fusion.Want{fusion.WantRelations}}, lens)
		if ferr != nil {
			if fuseErrIsRetryable(t, ferr) {
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}
		liveRank = rankByHandle(resp.Nodes, liveEntity.ID)
		deletedRank = rankByHandle(resp.Nodes, deletedEntity.ID)
		if resp.Index.Ready && liveRank >= 0 && deletedRank >= 0 {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if liveRank < 0 || deletedRank < 0 {
		t.Fatalf("both entities must be retained and rankable: liveRank=%d deletedRank=%d", liveRank, deletedRank)
	}
	if liveRank >= deletedRank {
		t.Errorf("live entity (rank %d) should outrank the stale entity (rank %d)", liveRank, deletedRank)
	}

	// --- (2) recreate the file + re-run the lifecycle pass → marker cleared. ---
	if err := os.WriteFile(deletedPath, []byte("package pkg\n\nfunc Deleted() int {\n\treturn 3\n}\n"), 0o644); err != nil {
		t.Fatalf("recreate deleted.go: %v", err)
	}
	// Give the watcher's initial re-index a moment before re-running the
	// pass, so the CodePath predicate the pass groups by is definitely
	// present again (a race here just costs an extra pass, never a false
	// clear — the pass only clears entities it finds ALREADY marked AND
	// present on disk).
	time.Sleep(300 * time.Millisecond)
	if _, err := semsourcegraph.PublishLifecycleTrigger(ctx, tc.Client, semsourcegraph.LifecycleRunRequest{
		Org:      org,
		Systems:  []string{system},
		RootPath: root,
		Reason:   semsourcegraph.LifecycleReasonFileDeleted,
	}); err != nil {
		t.Fatalf("trigger lifecycle pass (recreate): %v", err)
	}
	waitPredicateAbsent(t, ctx, qc, deletedEntity.ID, source.EntityLifecycleStale, 20*time.Second)

	// --- (3) remove_source shape: no root_path marks every in-scope entity
	// unconditionally, regardless of file presence. ---
	if _, err := semsourcegraph.PublishLifecycleTrigger(ctx, tc.Client, semsourcegraph.LifecycleRunRequest{
		Org:     org,
		Systems: []string{system},
		Reason:  semsourcegraph.LifecycleReasonSourceRemoved,
	}); err != nil {
		t.Fatalf("trigger lifecycle pass (source_removed): %v", err)
	}
	waitTriple(t, ctx, qc, liveEntity.ID, source.EntityLifecycleStale, source.LifecycleReasonSourceRemoved, 20*time.Second)
	waitTriple(t, ctx, qc, deletedEntity.ID, source.EntityLifecycleStale, source.LifecycleReasonSourceRemoved, 20*time.Second)
}

// waitPredicateAbsent polls until entity id no longer carries predicate, or
// fails the test after timeout.
func waitPredicateAbsent(t *testing.T, ctx context.Context, qc queryclient.Client, id, predicate string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if e, ok := fetchEntity(ctx, qc, id); ok {
			present := false
			for i := range e.Triples {
				if e.Triples[i].Predicate == predicate {
					present = true
					break
				}
			}
			if !present {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("entity %s still carries predicate %s after %s", id, predicate, timeout)
}
