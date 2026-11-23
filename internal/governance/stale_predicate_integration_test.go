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
// the file is then edited to drop both, and a fresh ast-source pass
// re-ingests it. The stored entity must no longer carry the calls edge or
// the doc comment: a predicate the producer stops emitting must not survive.
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

	runPass(t, ctx, tc.Client, root)
	runID, rev1, before := waitForFunction(t, ctx, tc.Client, "Run", 0)
	if !hasPredicate(before, ast.CodeCalls) || !hasPredicate(before, ast.CodeDocComment) {
		t.Fatalf("first pass: Run lacks calls or doc comment; triples = %+v", before.Triples)
	}

	write("package svc\n\nfunc X() {}\n\nfunc Run() {}\n")
	runPass(t, ctx, tc.Client, root)
	// The stream write and the reconcile that follows it land as separate
	// revisions, so read revisions until the state converges or time runs out.
	var after *semgraph.EntityState
	for rev, deadline := rev1, time.Now().Add(20*time.Second); ; {
		_, rev, after = waitForFunction(t, ctx, tc.Client, "Run", rev)
		converged := !hasPredicate(after, ast.CodeCalls) && !hasPredicate(after, ast.CodeDocComment)
		if converged || time.Now().After(deadline) {
			break
		}
	}
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

// runPass starts a non-watching ast-source over root, which publishes one
// full index pass, and stops it once that pass has been published.
func runPass(t *testing.T, ctx context.Context, client *natsclient.Client, root string) {
	t.Helper()
	cfg, err := json.Marshal(map[string]any{
		"watch_paths": []map[string]any{
			{"path": root, "org": "acme", "project": "svc", "languages": []string{"go"}},
		},
		"watch_enabled":  false,
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
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
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
	runPass(t, ctx, tc.Client, root)
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

	write("package svc\n\nfunc X() {}\n\nfunc Run() {}\n")
	runPass(t, ctx, tc.Client, root)
	var after *semgraph.EntityState
	for deadline := time.Now().Add(20 * time.Second); ; {
		_, rev, after = waitForFunction(t, ctx, tc.Client, "Run", rev)
		if (!hasPredicate(after, ast.CodeCalls) && hasPredicate(after, ast.CodeSupersedes)) || time.Now().After(deadline) {
			break
		}
	}
	if hasPredicate(after, ast.CodeCalls) {
		t.Fatalf("republish did not converge: %s still carries %s", runID, ast.CodeCalls)
	}
	for _, pred := range []string{ast.CodeSupersedes, sourcevocab.EntityLifecycleStale, ontology.ClassPredicate} {
		if !hasPredicate(after, pred) {
			t.Errorf("%s lost %s on republish; triples = %+v", runID, pred, after.Triples)
		}
	}
}
