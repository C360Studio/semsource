package entitypub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// fakeGraph stands in for the stream plus graph-ingest plus the mutation
// lane. Published messages queue and apply in arrival order, as graph-ingest
// applies one entity's writes; hold keeps them queued until release.
type fakeGraph struct {
	mu         sync.Mutex
	hold       bool
	queue      [][]byte
	state      map[string][]message.Triple
	rev        map[string]uint64
	reconciles []projection.ReconcileMutation
	readErr    error   // returned by ReadAuthoritative when set
	recErr     []error // returned by successive Reconcile calls, then nil
	envelopes  []time.Time
}

func newFakeGraph() *fakeGraph {
	return &fakeGraph{state: map[string][]message.Triple{}, rev: map[string]uint64{}}
}

func (g *fakeGraph) PublishToStream(_ context.Context, _ string, data []byte) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.queue = append(g.queue, append([]byte(nil), data...))
	if !g.hold {
		g.applyLocked()
	}
	return nil
}

func (g *fakeGraph) PublishToStreamWithMsgID(ctx context.Context, s string, data []byte, _ string) error {
	return g.PublishToStream(ctx, s, data)
}

func (g *fakeGraph) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.hold = false
	g.applyLocked()
}

// applyLocked applies queued messages the way the stream lane does:
// stamp from the envelope, then replace each carried (predicate, source) set.
func (g *fakeGraph) applyLocked() {
	for _, data := range g.queue {
		var wire struct {
			Payload graph.EntityPayload `json:"payload"`
			Meta    struct {
				CreatedAt int64  `json:"created_at"`
				Source    string `json:"source"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			panic(err)
		}
		at := time.UnixMilli(wire.Meta.CreatedAt).UTC()
		g.envelopes = append(g.envelopes, at)
		carried := map[[2]string]bool{}
		var arriving []message.Triple
		for _, t := range wire.Payload.TripleData {
			if t.Source == "" {
				t.Source = wire.Meta.Source
			}
			if t.Timestamp.IsZero() {
				t.Timestamp = at
			}
			carried[[2]string{t.Predicate, t.Source}] = true
			arriving = append(arriving, t)
		}
		var kept []message.Triple
		for _, t := range g.state[wire.Payload.ID] {
			if !carried[[2]string{t.Predicate, t.Source}] {
				kept = append(kept, t)
			}
		}
		g.state[wire.Payload.ID] = append(kept, arriving...)
		g.rev[wire.Payload.ID]++
	}
	g.queue = nil
}

func (g *fakeGraph) ReadAuthoritative(_ context.Context, id string) (*gtypes.ExactEntity, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.readErr != nil {
		return nil, g.readErr
	}
	triples, ok := g.state[id]
	if !ok {
		return nil, &projection.MutationError{Err: errs.ClassifiedCode(errs.ErrorInvalid, gtypes.ErrorCodeEntityNotFound, errors.New("absent"))}
	}
	return &gtypes.ExactEntity{
		Entity:     &gtypes.EntityState{ID: id, Triples: append([]message.Triple(nil), triples...)},
		KVRevision: g.rev[id],
	}, nil
}

func (g *fakeGraph) Reconcile(_ context.Context, r projection.ReconcileMutation) (projection.MutationReceipt, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reconciles = append(g.reconciles, r)
	if len(g.recErr) > 0 {
		err := g.recErr[0]
		g.recErr = g.recErr[1:]
		if err != nil {
			return projection.MutationReceipt{}, err
		}
	}
	if r.ExpectedRevision != g.rev[r.EntityID] {
		return projection.MutationReceipt{}, &projection.MutationError{Kind: projection.MutationRevisionConflict}
	}
	group := groupPredicates()
	var kept []message.Triple
	for _, t := range g.state[r.EntityID] {
		if !(group[t.Predicate] && t.Source == r.Metadata.Source) {
			kept = append(kept, t)
		}
	}
	g.state[r.EntityID] = append(kept, r.Desired...)
	g.rev[r.EntityID]++
	return projection.MutationReceipt{KVRevision: g.rev[r.EntityID]}, nil
}

func (g *fakeGraph) snapshot(id string) []message.Triple {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]message.Triple(nil), g.state[id]...)
}

func (g *fakeGraph) reconcileCalls() []projection.ReconcileMutation {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]projection.ReconcileMutation(nil), g.reconciles...)
}

func startPublisher(t *testing.T, g *fakeGraph, opts ...Option) (*Publisher, context.Context) {
	t.Helper()
	pub, err := New(g, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), opts...)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	pub.Start(ctx)
	t.Cleanup(func() { _ = pub.Stop(context.Background()) })
	return pub, ctx
}

func send(t *testing.T, pub *Publisher, payload *graph.EntityPayload) {
	t.Helper()
	if err := pub.Send(payload); err != nil {
		t.Fatal(err)
	}
}

func settle(t *testing.T, ctx context.Context, pub *Publisher) {
	t.Helper()
	if err := pub.WaitSettled(ctx); err != nil {
		t.Fatal(err)
	}
}

const reconcileID = "acme.test.svc.golang.function.svc-go-Run"

func runPayload(startLine int, withCall bool) *graph.EntityPayload {
	fixed := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	p := &graph.EntityPayload{
		ID:                  reconcileID,
		IndexingProfileHint: graph.IndexingProfileContent,
		TripleData: []message.Triple{
			{Subject: reconcileID, Predicate: semast.CodeStartLine, Object: startLine},
			{Subject: reconcileID, Predicate: semast.CodeBelongsTo, Object: "x", Source: "semsource", Timestamp: fixed},
			// Outside the group: kept off the reconcile.
			{Subject: reconcileID, Predicate: "entity.ontology.class", Object: "bfo:x"},
			// Another source's statement on a group predicate: never claimed.
			{Subject: reconcileID, Predicate: semast.CodeSupersedes, Object: "y", Source: "supersession", Timestamp: fixed},
		},
	}
	if withCall {
		p.TripleData = append(p.TripleData, message.Triple{Subject: reconcileID, Predicate: semast.CodeCalls, Object: "X"})
	}
	return p
}

func has(triples []message.Triple, predicate string) bool {
	for _, t := range triples {
		if t.Predicate == predicate {
			return true
		}
	}
	return false
}

// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestNoReconcileWithoutTheOption(t *testing.T) {
	g := newFakeGraph()
	pub, ctx := startPublisher(t, g)
	send(t, pub, runPayload(5, false))
	settle(t, ctx, pub)
	if n := len(g.reconcileCalls()); n != 0 || pub.Reconciled() != 0 {
		t.Errorf("reconcile ran without WithReconcile: calls=%d reconciled=%d", n, pub.Reconciled())
	}
}

// The reconcile waits until its own stream write has applied, then replaces
// the group at that revision with statements stamped exactly as the stream
// arrival was.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestReconcileWaitsForItsStreamWriteAndMatchesItsStamping(t *testing.T) {
	g := newFakeGraph()
	pub, ctx := startPublisher(t, g, WithReconcile(g))
	send(t, pub, runPayload(6, true)) // stored without a reconcile in flight
	settle(t, ctx, pub)
	before := len(g.reconcileCalls())

	g.mu.Lock()
	g.hold = true
	g.mu.Unlock()
	send(t, pub, runPayload(5, false))
	// The write is acknowledged but not applied: no replace may run yet.
	waitUntil(t, ctx, func() bool { return pub.reconcilePending.Load() == 1 && len(g.queue2()) == 1 })
	if n := len(g.reconcileCalls()); n != before {
		t.Fatalf("reconcile replaced the group before its stream write applied (%d calls)", n-before)
	}
	g.release()
	settle(t, ctx, pub)

	calls := g.reconcileCalls()[before:]
	if len(calls) != 1 {
		t.Fatalf("reconcile calls after apply = %d, want 1", len(calls))
	}
	r := calls[0]
	if r.Contract != graph.SourceEntityContract().Name || r.Group != graph.GroupSource || r.EntityID != reconcileID {
		t.Errorf("request = %s/%s/%s", r.Contract, r.Group, r.EntityID)
	}
	if r.Metadata.Source != "semsource" || !r.Metadata.Timestamp.IsZero() {
		t.Errorf("metadata = %+v, want source semsource and no timestamp", r.Metadata)
	}
	envelope := g.envelopes[len(g.envelopes)-1]
	got := map[string]message.Triple{}
	for _, d := range r.Desired {
		got[d.Predicate] = d
	}
	if len(got) != 2 {
		t.Fatalf("desired = %v, want exactly start-line and belongs", r.Desired)
	}
	if d := got[semast.CodeStartLine]; d.Source != "semsource" || !d.Timestamp.Equal(envelope) {
		t.Errorf("start-line stamped %+v, want source semsource at envelope time %v", d, envelope)
	}
	if d := got[semast.CodeBelongsTo]; d.Timestamp.Day() != 1 {
		t.Errorf("a statement's own timestamp was overwritten: %+v", d)
	}
	stored := g.snapshot(reconcileID)
	if has(stored, semast.CodeCalls) {
		t.Error("the dropped call survived the reconcile")
	}
	if !has(stored, semast.CodeSupersedes) || !has(stored, "entity.ontology.class") {
		t.Error("another source's statement or a non-group predicate was removed")
	}
}

// Two writes of one entity queued unapplied: the older one cannot land after
// the reconcile and restore a predicate the newer publish dropped.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestOlderQueuedWriteCannotRestoreADroppedPredicate(t *testing.T) {
	g := newFakeGraph()
	pub, ctx := startPublisher(t, g, WithReconcile(g))
	send(t, pub, runPayload(7, true))
	settle(t, ctx, pub)

	g.mu.Lock()
	g.hold = true
	g.mu.Unlock()
	send(t, pub, runPayload(6, true))  // N-1 still carries the call
	send(t, pub, runPayload(5, false)) // N drops it
	waitUntil(t, ctx, func() bool { return len(g.queue2()) == 2 })
	g.release()
	settle(t, ctx, pub)

	if has(g.snapshot(reconcileID), semast.CodeCalls) {
		t.Error("an older queued write restored the dropped call")
	}
	if pub.ReconcileFailed() != 0 {
		t.Errorf("reconcile_failed = %d", pub.ReconcileFailed())
	}
}

func (g *fakeGraph) queue2() [][]byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.queue
}

// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestSkipReconcilePayloadIsNeverReconciled(t *testing.T) {
	g := newFakeGraph()
	pub, ctx := startPublisher(t, g, WithReconcile(g))
	payload := runPayload(5, true)
	payload.SkipReconcile = true
	send(t, pub, payload)
	settle(t, ctx, pub)
	if n := len(g.reconcileCalls()); n != 0 {
		t.Errorf("skip-reconcile payload reconciled %d times", n)
	}
	if pub.Published() != 1 {
		t.Error("skip-reconcile payload not stream-published")
	}
}

// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestReconcileOutcomes(t *testing.T) {
	conflict := &projection.MutationError{Kind: projection.MutationRevisionConflict}
	cases := []struct {
		name       string
		readErr    error
		recErr     []error
		wantCalls  int
		wantFailed int64
		wantDone   int64
	}{
		{"a lost compare-and-set re-reads and commits", nil, []error{conflict}, 2, 0, 1},
		{"a replace error counts, no retry", nil, []error{errors.New("timeout")}, 1, 1, 0},
		{"a read error counts, no replace", errors.New("read down"), nil, 0, 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newFakeGraph()
			g.readErr, g.recErr = tc.readErr, tc.recErr
			pub, ctx := startPublisher(t, g, WithReconcile(g))
			send(t, pub, runPayload(5, false))
			settle(t, ctx, pub)
			if n := len(g.reconcileCalls()); n != tc.wantCalls {
				t.Errorf("replace calls = %d, want %d", n, tc.wantCalls)
			}
			if pub.ReconcileFailed() != tc.wantFailed || pub.Reconciled() != tc.wantDone {
				t.Errorf("failed=%d reconciled=%d, want %d/%d", pub.ReconcileFailed(), pub.Reconciled(), tc.wantFailed, tc.wantDone)
			}
			if pub.Published() != 1 || pub.Failed() != 0 || pub.Lost() != 0 {
				t.Errorf("delivery counters moved: published=%d failed=%d lost=%d", pub.Published(), pub.Failed(), pub.Lost())
			}
		})
	}
}

// Stop while a reconcile waits on an unapplied write returns promptly and
// does not count the abandoned reconcile as a failure.
func TestStopAbandonsAWaitingReconcileUncounted(t *testing.T) {
	g := newFakeGraph()
	g.hold = true
	pub, err := New(g, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), WithReconcile(g))
	if err != nil {
		t.Fatal(err)
	}
	pub.Start(context.Background())
	send(t, pub, runPayload(5, false))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	waitUntil(t, ctx, func() bool { return pub.Published() == 1 })
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()
	if err := pub.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if pub.ReconcileFailed() != 0 {
		t.Errorf("an abandoned shutdown reconcile counted as failed (%d)", pub.ReconcileFailed())
	}
}

// waitUntil polls cond until it holds or ctx ends; cond reads published state.
func waitUntil(t *testing.T, ctx context.Context, cond func() bool) {
	t.Helper()
	for !cond() {
		select {
		case <-ctx.Done():
			t.Fatal("condition never held")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
