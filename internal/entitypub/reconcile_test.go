package entitypub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/projection"

	"github.com/c360studio/semsource/graph"
	semast "github.com/c360studio/semsource/source/ast"
)

type okClient struct{}

func (okClient) PublishToStream(context.Context, string, []byte) error { return nil }
func (okClient) PublishToStreamWithMsgID(context.Context, string, []byte, string) error {
	return nil
}

type fakeReconciler struct {
	mu   sync.Mutex
	reqs []projection.ReconcileMutation
	err  error
}

func (f *fakeReconciler) Reconcile(_ context.Context, r projection.ReconcileMutation) (projection.MutationReceipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, r)
	return projection.MutationReceipt{}, f.err
}

func (f *fakeReconciler) requests() []projection.ReconcileMutation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]projection.ReconcileMutation(nil), f.reqs...)
}

// publishSettled sends payload through a started publisher and waits until it
// is delivered, so every reconcile for it has run.
func publishSettled(t *testing.T, payload *graph.EntityPayload, opts ...Option) *Publisher {
	t.Helper()
	pub, err := New(okClient{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), opts...)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	pub.Start(ctx)
	t.Cleanup(func() { _ = pub.Stop(context.Background()) })
	if err := pub.Send(payload); err != nil {
		t.Fatal(err)
	}
	if err := pub.WaitSettled(ctx); err != nil {
		t.Fatal(err)
	}
	return pub
}

const reconcileID = "acme.test.svc.golang.function.svc-go-Run"

func reconcilePayload() *graph.EntityPayload {
	fixed := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	return &graph.EntityPayload{
		ID:                  reconcileID,
		IndexingProfileHint: graph.IndexingProfileContent,
		TripleData: []message.Triple{
			{Subject: reconcileID, Predicate: semast.CodeStartLine, Object: 5},
			{Subject: reconcileID, Predicate: semast.CodeBelongsTo, Object: "x", Source: "semsource", Timestamp: fixed},
			// Outside the group: kept off the reconcile.
			{Subject: reconcileID, Predicate: "entity.ontology.class", Object: "bfo:x"},
			// Another source's statement on a group predicate: never claimed.
			{Subject: reconcileID, Predicate: "code.lineage.supersedes", Object: "y", Source: "supersession", Timestamp: fixed},
		},
	}
}

// The option is absent: nothing but the stream publish happens.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestNoReconcileWithoutTheOption(t *testing.T) {
	pub := publishSettled(t, reconcilePayload())
	if pub.Reconciled() != 0 || pub.ReconcileFailed() != 0 {
		t.Errorf("reconcile ran without WithReconcile: reconciled=%d failed=%d", pub.Reconciled(), pub.ReconcileFailed())
	}
}

// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestReconcileSendsTheGroupSetStampedLikeTheStream(t *testing.T) {
	rec := &fakeReconciler{}
	pub := publishSettled(t, reconcilePayload(), WithReconcile(rec))

	reqs := rec.requests()
	if len(reqs) != 1 {
		t.Fatalf("reconcile calls = %d, want 1", len(reqs))
	}
	r := reqs[0]
	if r.Contract != graph.SourceEntityContract().Name || r.Group != graph.GroupSource || r.EntityID != reconcileID {
		t.Errorf("request = %s/%s/%s", r.Contract, r.Group, r.EntityID)
	}
	if r.Metadata.Source != "semsource" || !r.Metadata.Timestamp.IsZero() {
		// A metadata timestamp would conflict with statements that carry their own.
		t.Errorf("metadata = %+v, want source semsource and no timestamp", r.Metadata)
	}
	got := map[string]message.Triple{}
	for _, d := range r.Desired {
		got[d.Predicate] = d
	}
	if len(got) != 2 {
		t.Fatalf("desired = %v, want exactly start-line and belongs", r.Desired)
	}
	if d := got[semast.CodeStartLine]; d.Source != "semsource" || d.Timestamp.IsZero() ||
		!d.Timestamp.Equal(d.Timestamp.Truncate(time.Millisecond)) {
		t.Errorf("unstamped statement became %+v, want source semsource at the millisecond envelope time", d)
	}
	if d := got[semast.CodeBelongsTo]; d.Timestamp.Year() != 2026 || d.Timestamp.Month() != 11 || d.Timestamp.Day() != 1 {
		t.Errorf("a statement's own timestamp was overwritten: %+v", d)
	}
	if pub.Reconciled() != 1 || pub.Published() != 1 {
		t.Errorf("reconciled=%d published=%d, want 1/1", pub.Reconciled(), pub.Published())
	}
}

// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestSkipReconcilePayloadIsNeverReconciled(t *testing.T) {
	rec := &fakeReconciler{}
	payload := reconcilePayload()
	payload.SkipReconcile = true
	pub := publishSettled(t, payload, WithReconcile(rec))
	if n := len(rec.requests()); n != 0 {
		t.Errorf("skip-reconcile payload reconciled %d times", n)
	}
	if pub.Published() != 1 {
		t.Errorf("skip-reconcile payload not stream-published")
	}
}

// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestReconcileOutcomes(t *testing.T) {
	notFound := errs.ClassifiedCode(errs.ErrorInvalid, gtypes.ErrorCodeEntityNotFound, errors.New("absent"))
	conflict := &projection.MutationError{Kind: projection.MutationRevisionConflict, Err: errors.New("moved")}
	cases := []struct {
		name       string
		err        error
		wantFailed int64
		wantCalls  int
	}{
		{"birth (entity_not_found) is benign", fmt.Errorf("read: %w", notFound), 0, 1},
		{"persistent revision conflict re-reads a bounded number of times, then counts", conflict, 1, reconcileAttempts},
		{"transport error counts, no retry", errors.New("timeout"), 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &fakeReconciler{err: tc.err}
			pub := publishSettled(t, reconcilePayload(), WithReconcile(rec))
			if n := len(rec.requests()); n != tc.wantCalls {
				t.Errorf("reconcile calls = %d, want %d", n, tc.wantCalls)
			}
			if pub.ReconcileFailed() != tc.wantFailed {
				t.Errorf("reconcile_failed = %d, want %d", pub.ReconcileFailed(), tc.wantFailed)
			}
			if pub.Published() != 1 || pub.Failed() != 0 || pub.Lost() != 0 {
				t.Errorf("delivery counters moved: published=%d failed=%d lost=%d",
					pub.Published(), pub.Failed(), pub.Lost())
			}
		})
	}
}

// conflictOnce loses the first compare-and-set, as a reconcile does when the
// stream write it follows applies between its read and its write.
type conflictOnce struct{ fakeReconciler }

func (c *conflictOnce) Reconcile(ctx context.Context, r projection.ReconcileMutation) (projection.MutationReceipt, error) {
	_, _ = c.fakeReconciler.Reconcile(ctx, r)
	if len(c.requests()) == 1 {
		return projection.MutationReceipt{}, &projection.MutationError{Kind: projection.MutationRevisionConflict}
	}
	return projection.MutationReceipt{}, nil
}

// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestReconcileReReadsAfterLosingToItsOwnStreamWrite(t *testing.T) {
	rec := &conflictOnce{}
	pub := publishSettled(t, reconcilePayload(), WithReconcile(rec))
	if n := len(rec.requests()); n != 2 {
		t.Fatalf("reconcile calls = %d, want 2", n)
	}
	if pub.Reconciled() != 1 || pub.ReconcileFailed() != 0 {
		t.Errorf("reconciled=%d failed=%d, want 1/0", pub.Reconciled(), pub.ReconcileFailed())
	}
}
