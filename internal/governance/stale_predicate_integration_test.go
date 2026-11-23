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

	"github.com/c360studio/semengine/component"
	semgraph "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/projection"
	"github.com/c360studio/semengine/types"

	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/natstest"
	astsource "github.com/c360studio/semsource/processor/ast-source"
	"github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/ontology"
	sourcevocab "github.com/c360studio/semsource/source/vocabulary"
)

// TestIntegration_ReingestDropsRemovedPredicates is the #336 repro. The REAL
// ast-source indexes a Go function that calls X and carries a doc comment;
// the file is then edited to drop both, and the watching ast-source
// re-ingests it. (A restart over an edit made while semsource was down is the
// known gap of the bounded form, owner ruling task 5.2: semengine#730.) The stored entity must no longer carry the calls edge or
// the doc comment: a predicate the producer stops emitting must not survive.
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestIntegration_ReingestDropsRemovedPredicates(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t,
		natstest.WithJetStream(),
		natstest.WithStreams(natstest.Stream{Name: "GRAPH", Subjects: []string{"graph.ingest.entity"}}),
	)
	if _, err := BootstrapStandalone(nil); err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}
	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	ingest := startGraphIngest(t, ctx, tc.Client, reg, metric.NewMetricsRegistry())
	t.Cleanup(func() { _ = stopWithin(5*time.Second, ingest.Stop) })

	root := t.TempDir()
	src := filepath.Join(root, "svc.go")
	write := func(body string) {
		if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("package svc\n\nfunc X() {}\n\n// Run does the work.\nfunc Run() { X() }\n")

	runPass(t, ctx, tc.Client, root, true)
	runID, rev1, before := waitForFunction(t, ctx, tc.Client, "Run", 0)
	if !hasPredicate(before, ast.CodeCalls) || !hasPredicate(before, ast.CodeDocComment) {
		t.Fatalf("first pass: Run lacks calls or doc comment; triples = %+v", before.Triples)
	}

	after := editUntil(t, ctx, tc.Client, rev1, func() { write("package svc\n\nfunc X() {}\n\nfunc Run() {}\n") },
		func(st *semgraph.EntityState) bool {
			return !hasPredicate(st, ast.CodeCalls) && !hasPredicate(st, ast.CodeDocComment)
		})
	// Prove the stored state is the second pass's: Run moved from line 6 to
	// line 5, and a re-emitted predicate replaces its set.
	if got := tripleObject(after.Triples, ast.CodeStartLine); fmt.Sprint(got) != "5" {
		t.Fatalf("stored state is not the second pass: %s = %v, want 5", ast.CodeStartLine, got)
	}

	for _, pred := range []string{ast.CodeCalls, ast.CodeDocComment} {
		if hasPredicate(after, pred) {
			t.Errorf("%s still carries %s after the edit removed it", runID, pred)
		}
	}
}

// editUntil applies edit and reads Run's newer revisions until done holds or
// 30s pass, returning the last state read. The edit is re-applied on every
// read: the watcher starts after the seed publishes, so an edit landing
// before it is watching produces no event, and a repeated identical edit is
// harmless (the watch path republishes the same content).
func editUntil(
	t *testing.T, ctx context.Context, client *natsclient.Client, after uint64,
	edit func(), done func(*semgraph.EntityState) bool,
) *semgraph.EntityState {
	t.Helper()
	var st *semgraph.EntityState
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		edit()
		if got, rev, ok := readFunction(ctx, client, "Run"); ok && rev > after {
			st = got
			if done(st) {
				return st
			}
		}
	}
	if st == nil {
		t.Fatalf("no revision of Run newer than %d reached ENTITY_STATES", after)
	}
	return st
}

// readFunction reads the stored Go function entity whose ID ends in
// "-<name>", without waiting.
func readFunction(ctx context.Context, client *natsclient.Client, name string) (*semgraph.EntityState, uint64, bool) {
	bucket, err := client.GetKeyValueBucket(ctx, semgraph.BucketEntityStates)
	if err != nil {
		return nil, 0, false
	}
	keys, _ := bucket.Keys(ctx)
	for _, k := range keys {
		if !strings.Contains(k, ".function.") || !strings.HasSuffix(k, "-"+name) {
			continue
		}
		entry, err := bucket.Get(ctx, k)
		if err != nil {
			return nil, 0, false
		}
		var st semgraph.EntityState
		if json.Unmarshal(entry.Value(), &st) != nil {
			return nil, 0, false
		}
		return &st, entry.Revision(), true
	}
	return nil, 0, false
}

// runPass starts an ast-source over root, which publishes one full index
// pass and, with watch, then republishes edited files. It stops at cleanup.
func runPass(t *testing.T, ctx context.Context, client *natsclient.Client, root string, watch bool) {
	t.Helper()
	cfg, err := json.Marshal(map[string]any{
		"watch_paths": []map[string]any{
			{"path": root, "org": "acme", "project": "svc", "languages": []string{"go"}},
		},
		"watch_enabled":  watch,
		"index_interval": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	discovered, err := astsource.NewComponent(cfg, component.Dependencies{
		NATSClient: client,
		Platform:   types.PlatformMeta{Org: "acme", Platform: "test-a1b2c3"},
	})
	if err != nil {
		t.Fatalf("ast-source NewComponent: %v", err)
	}
	comp := discovered.(*astsource.Component)
	if err := comp.Initialize(); err != nil {
		t.Fatalf("ast-source Initialize: %v", err)
	}
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("ast-source Start: %v", err)
	}
	t.Cleanup(func() { _ = stopWithin(5*time.Second, comp.Stop) })
}

// waitForFunction returns the stored Go function entity whose ID ends in
// ".<name>", once its ENTITY_STATES revision is newer than after.
func waitForFunction(
	t *testing.T, ctx context.Context, client *natsclient.Client, name string, after uint64,
) (string, uint64, *semgraph.EntityState) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		bucket, err := client.GetKeyValueBucket(ctx, semgraph.BucketEntityStates)
		if err == nil {
			keys, _ := bucket.Keys(ctx)
			for _, k := range keys {
				if !strings.Contains(k, ".function.") || !strings.HasSuffix(k, "-"+name) && !strings.HasSuffix(k, "."+name) {
					continue
				}
				entry, err := bucket.Get(ctx, k)
				if err != nil || entry.Revision() <= after {
					continue
				}
				var st semgraph.EntityState
				if err := json.Unmarshal(entry.Value(), &st); err != nil {
					t.Fatalf("unmarshal %s: %v", k, err)
				}
				return k, entry.Revision(), &st
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no function %q newer than revision %d reached ENTITY_STATES", name, after)
	return "", 0, nil
}

// TestIntegration_ReingestKeepsOtherSourcesStatements pins the other half of
// the #336 reconcile: a republish removes only semsource's own group
// statements. A supersession lineage edge, the lifecycle stale marker and the
// ontology class (outside the group) all survive it.
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestIntegration_ReingestKeepsOtherSourcesStatements(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t,
		natstest.WithJetStream(),
		natstest.WithStreams(natstest.Stream{Name: "GRAPH", Subjects: []string{"graph.ingest.entity"}}),
	)
	if _, err := BootstrapStandalone(nil); err != nil {
		t.Fatalf("BootstrapStandalone() error = %v", err)
	}
	reg := payloadregistry.New()
	if err := semsourcegraph.RegisterPayloads(reg); err != nil {
		t.Fatalf("RegisterPayloads() error = %v", err)
	}
	ingest := startGraphIngest(t, ctx, tc.Client, reg, metric.NewMetricsRegistry())
	t.Cleanup(func() { _ = stopWithin(5*time.Second, ingest.Stop) })

	root := t.TempDir()
	src := filepath.Join(root, "svc.go")
	write := func(body string) {
		if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("package svc\n\nfunc X() {}\n\nfunc Run() { X() }\n")
	runPass(t, ctx, tc.Client, root, true)
	runID, rev, _ := waitForFunction(t, ctx, tc.Client, "Run", 0)

	// Another source's lineage edge, through the stream lane as supersession
	// publishes it, and the stale marker through the lifecycle group.
	now := time.Now()
	lineage := &semsourcegraph.EntityPayload{
		ID:                  runID,
		IndexingProfileHint: semsourcegraph.IndexingProfileContent,
		TripleData: []message.Triple{{
			Subject: runID, Predicate: ast.CodeSupersedes, Object: "acme.test-a1b2c3.svc.golang.function.old-Run",
			Datatype: message.EntityReferenceDatatype, Source: "supersession", Timestamp: now, Confidence: 1.0,
		}},
	}
	data, err := json.Marshal(message.NewBaseMessage(semsourcegraph.EntityType, lineage, "supersession"))
	if err != nil {
		t.Fatal(err)
	}
	if err := tc.Client.PublishToStream(ctx, "graph.ingest.entity", data); err != nil {
		t.Fatal(err)
	}
	mut, err := projection.NewMutationClient(projection.MutationClientConfig{
		NATS: tc.Client, Contracts: []projection.Contract{semsourcegraph.SourceEntityContract()},
	})
	if err != nil {
		t.Fatal(err)
	}
	marker := message.Triple{Subject: runID, Predicate: sourcevocab.EntityLifecycleStale, Object: "test", Source: "lifecycle", Timestamp: now, Confidence: 1.0}
	var markErr error
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if _, markErr = mut.Reconcile(ctx, projection.ReconcileMutation{
			Contract: semsourcegraph.SourceEntityContract().Name, Group: semsourcegraph.GroupLifecycle,
			EntityID: runID, Desired: []message.Triple{marker},
			Metadata: projection.MutationMetadata{Source: "lifecycle", Timestamp: now},
		}); markErr == nil {
			break
		}
	}
	if markErr != nil {
		t.Fatalf("set stale marker: %v", markErr)
	}

	after := editUntil(t, ctx, tc.Client, rev, func() { write("package svc\n\nfunc X() {}\n\nfunc Run() {}\n") },
		func(st *semgraph.EntityState) bool {
			return !hasPredicate(st, ast.CodeCalls) && hasPredicate(st, ast.CodeSupersedes)
		})
	if hasPredicate(after, ast.CodeCalls) {
		t.Fatalf("republish did not converge: %s still carries %s", runID, ast.CodeCalls)
	}
	for _, pred := range []string{ast.CodeSupersedes, sourcevocab.EntityLifecycleStale, ontology.ClassPredicate} {
		if !hasPredicate(after, pred) {
			t.Errorf("%s lost %s on republish; triples = %+v", runID, pred, after.Triples)
		}
	}
}
