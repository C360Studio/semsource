//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/pkg/fusion/fusionnats"
	"github.com/c360studio/semstreams/pkg/fusion/fusionvocab"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semsource/entityid"
	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/internal/sourcelifecycle"
	"github.com/c360studio/semsource/internal/sourcespawn"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
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
//  3. the legacy source_removed trigger is refused while the source is live;
//     the real journal/coordinator waits for checked producer retirement before
//     marking retained entities through the local, effect-fenced owner;
//  4. a later file-presence sweep cannot clear sticky source_removed markers.
//
// Desired configuration uses the stateful memConfigStore fixture seam. This
// qualifies real journal, consumer-tail, graph and mutation effects, not full
// ConfigManager persistence or OS process replacement (the qualification suite
// covers those). Retained convergence remains pending on SemStreams #1444.
func TestIntegration_StalenessLifecycle(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
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
	mr := metric.NewMetricsRegistry()

	ingest := startGraphIngest(t, ctx, tc.Client, reg, mr)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, ingest.Stop) })
	index := startGraphIndex(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, index.Stop) })
	q := startGraphQuery(t, ctx, tc.Client, mr)
	t.Cleanup(func() { _ = stopWithin(5*time.Second, q.Stop) })

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
		"instance_name":  "ast-source-svc",
		"watch_enabled":  true,
		"index_interval": "", // manual lifecycle triggering below; no periodic sweep noise
	})
	if err != nil {
		t.Fatalf("marshal ast-source config: %v", err)
	}
	discovered, err := astsource.NewComponent(astCfg, component.Dependencies{NATSClient: tc.Client, Platform: component.PlatformMeta{Org: "acme", Platform: "test-a1b2c3"}})
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

	scfg, _ := json.Marshal(map[string]any{"max_entities": 1000})
	sdiscovered, err := supersession.NewComponent(scfg, component.Dependencies{NATSClient: tc.Client, Platform: component.PlatformMeta{Org: "acme", Platform: "test-a1b2c3"}})
	if err != nil {
		t.Fatalf("supersession NewComponent: %v", err)
	}
	scomp := sdiscovered.(*supersession.Component)
	if err := scomp.Start(ctx); err != nil {
		t.Fatalf("supersession Start: %v", err)
	}
	t.Cleanup(func() { _ = stopWithin(5*time.Second, scomp.Stop) })

	qc := tc.Client

	system := entityid.ScopedSystemSlug(project, "")
	prefix := org + ".test-a1b2c3." + system + ".golang"

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

	// --- (3) an active producer cannot be removed through the legacy RPC. ---
	if _, err := semsourcegraph.PublishLifecycleTrigger(ctx, tc.Client, semsourcegraph.LifecycleRunRequest{
		Org: org, Systems: []string{system}, Reason: semsourcegraph.LifecycleReasonSourceRemoved,
	}); err == nil || !strings.Contains(err.Error(), "authorized source lifecycle owner") {
		t.Fatalf("legacy source_removed must explicitly refuse: %v", err)
	}
	assertStalenessRemovalAbsent(ctx, t, qc, liveEntity.ID, deletedEntity.ID)

	owner, journal := stalenessRemovalOwner(ctx, t, tc.Client, astCfg, scomp)
	release, err := owner.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	record, prepareErr := owner.PrepareRemoval(ctx, "ast-source-svc")
	if prepareErr == nil {
		prepareErr = owner.PersistRemoval(ctx, record)
	}
	release()
	if prepareErr != nil {
		t.Fatal(prepareErr)
	}
	var blocked *sourceintent.Blocker
	if err := owner.ReconcileOnce(ctx); !errors.As(err, &blocked) || blocked.Code != sourceintent.CodeRetirement {
		t.Fatalf("enabled boot must block removal effects: %v", err)
	}
	assertStalenessRemovalAbsent(ctx, t, qc, liveEntity.ID, deletedEntity.ID)

	// The fixture advances its immutable boot admission only after Stop proves
	// this real producer and all accepted publication work have retired.
	if err := stopWithin(5*time.Second, astComp.Stop); err != nil {
		t.Fatalf("retire AST producer: %v", err)
	}
	release, err = owner.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, bindErr := owner.BindBoot(ctx)
	release()
	if bindErr != nil {
		t.Fatal(bindErr)
	}
	awaitStalenessRetainedPass(ctx, t, owner, journal)
	waitTriple(t, ctx, qc, liveEntity.ID, source.EntityLifecycleStale, source.LifecycleReasonSourceRemoved, 20*time.Second)
	waitTriple(t, ctx, qc, deletedEntity.ID, source.EntityLifecycleStale, source.LifecycleReasonSourceRemoved, 20*time.Second)

	// --- (4) present files do not re-authorize a removed source. ---
	summary, err := semsourcegraph.PublishLifecycleTrigger(ctx, tc.Client, semsourcegraph.LifecycleRunRequest{
		Org: org, Systems: []string{system}, RootPath: root, Reason: semsourcegraph.LifecycleReasonFileDeleted,
	})
	if err != nil || summary == nil || summary.Cleared != 0 {
		t.Fatalf("legacy sweep cleared removed source: %+v %v", summary, err)
	}
	waitTriple(t, ctx, qc, liveEntity.ID, source.EntityLifecycleStale, source.LifecycleReasonSourceRemoved, 20*time.Second)
	waitTriple(t, ctx, qc, deletedEntity.ID, source.EntityLifecycleStale, source.LifecycleReasonSourceRemoved, 20*time.Second)
	// Selective source_removed reactivation is a distinct blocked acceptance
	// (#1445); file recreation above proves only the file_deleted contract.
}

// waitPredicateAbsent polls until entity id no longer carries predicate, or
// fails the test after timeout.
func waitPredicateAbsent(t *testing.T, ctx context.Context, qc *natsclient.Client, id, predicate string, timeout time.Duration) {
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

func assertStalenessRemovalAbsent(ctx context.Context, t *testing.T, client *natsclient.Client, ids ...string) {
	t.Helper()
	for _, id := range ids {
		entity, ok := fetchEntity(ctx, client, id)
		if !ok || entity == nil {
			t.Fatalf("retained entity missing: %s", id)
		}
		if countObject(entity, source.EntityLifecycleStale, source.LifecycleReasonSourceRemoved) != 0 {
			t.Fatalf("active entity was removed: %s", id)
		}
	}
}
func stalenessRemovalOwner(ctx context.Context, t *testing.T, client *natsclient.Client, astCfg json.RawMessage, projector *supersession.Component) (*sourcelifecycle.Coordinator, *sourcelifecycle.KVJournal) {
	t.Helper()
	store := newMemConfigStore()
	if err := store.PutComponentToKV(ctx, "ast-source-svc", types.ComponentConfig{Name: "ast-source", Type: types.ComponentTypeProcessor, Enabled: true, Config: astCfg}); err != nil {
		t.Fatal(err)
	}
	journal, err := sourcelifecycle.OpenJournal(ctx, client, fixtureAuthority(), "acme", 1)
	if err != nil {
		t.Fatal(err)
	}
	local := &sourcelifecycle.LocalProjector{}
	if err := local.Bind(projector); err != nil {
		t.Fatal(err)
	}
	owner, err := sourcelifecycle.NewCoordinator(sourcelifecycle.CoordinatorConfig{
		Authority: fixtureAuthority(), Namespace: "acme", Journal: journal, Store: store,
		Tail: sourcelifecycle.NATSTail{Client: client}, Projector: local,
		Inputs: []sourceintent.StreamInput{{Stream: "GRAPH", Consumer: "graph-ingest-graph-ingest-entity", Filters: []string{"graph.ingest.entity"}}},
		RepairManifest: func(ctx context.Context, handle string) error {
			return sourcemanifest.RepairRemovalManifest(ctx, store, sourcespawn.Options{Org: "acme"}, handle)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	release, err := owner.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := owner.BindBoot(ctx); err != nil {
		t.Fatal(err)
	}
	return owner, journal
}
func awaitStalenessRetainedPass(ctx context.Context, t *testing.T, owner *sourcelifecycle.Coordinator, journal *sourcelifecycle.KVJournal) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		last = owner.ReconcileOnce(ctx)
		var blocked *sourceintent.Blocker
		if errors.As(last, &blocked) && blocked.Code == sourceintent.CodeAppliedTailUnproven {
			record, _, err := journal.Get(ctx, "ast-source-svc")
			if err != nil {
				t.Fatal(err)
			}
			if record.Phase != sourceintent.Pending || record.Progress.CompletedCount < 2 || record.Effect == nil || record.Effect.Outcome != sourceintent.EffectVerified {
				t.Fatalf("retained projection overstated/lost durable evidence: %+v", record)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("retained pass did not converge with honest #1444 pending state: %v", last)
}
