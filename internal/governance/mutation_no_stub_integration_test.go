//go:build integration

package governance

import (
	"context"
	"errors"
	"testing"
	"time"

	semsourcegraph "github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/projection"

	"github.com/c360studio/semstreams/metric"
)

// Beta.160 invariant (task 4.1 of semstreams-beta160-migration): mutating a
// missing entity returns entity_not_found and never creates a stub. Every
// SemSource append/reconcile path (supersession's lifecycle group is the live
// caller) depends on entities being BORN with their envelope first — this test
// fails if the substrate ever quietly auto-vivifies again, or if our contract
// lets a mutation through to an unborn subject.
func TestIntegration_MutationToMissingEntity_ReturnsNotFoundAndNoStub(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tc := natsclient.NewTestClient(t, natsclient.WithKV(),
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
	ingest := startGraphIngest(t, ctx, tc.Client, reg, metric.NewMetricsRegistry())
	t.Cleanup(func() { _ = ingest.Stop(5 * time.Second) })

	mut, err := projection.NewMutationClient(projection.MutationClientConfig{
		NATS:      tc.Client,
		Contracts: []projection.Contract{semsourcegraph.SourceEntityContract()},
		Timeout:   10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewMutationClient() error = %v", err)
	}

	const missingID = "acme.semsource.golang.workspace.function.never-born"
	// One shared timestamp: the client requires triple and metadata
	// timestamps to agree (tuple identity).
	now := time.Now()
	_, err = mut.Reconcile(ctx, projection.ReconcileMutation{
		Contract: semsourcegraph.SourceEntityContract().Name,
		Group:    semsourcegraph.GroupLifecycle,
		EntityID: missingID,
		Desired: []message.Triple{{
			Subject:    missingID,
			Predicate:  "entity.lifecycle.stale",
			Object:     "test",
			Source:     "no-stub-test",
			Timestamp:  now,
			Confidence: 1.0,
		}},
		Metadata: projection.MutationMetadata{Source: "no-stub-test", Timestamp: now},
	})
	if err == nil {
		t.Fatal("Reconcile against a missing entity succeeded; want entity_not_found")
	}
	var ce *errs.ClassifiedError
	if !errors.As(err, &ce) {
		t.Fatalf("error is not classified: %v", err)
	}

	// The failed mutation must not have vivified the entity.
	bucket, err := tc.Client.GetKeyValueBucket(ctx, "ENTITY_STATES")
	if err != nil {
		t.Fatalf("open ENTITY_STATES: %v", err)
	}
	if _, err := tc.Client.NewKVStore(bucket).Get(ctx, missingID); err == nil {
		t.Fatalf("entity %s exists after a failed mutation; a stub was created", missingID)
	}
}
