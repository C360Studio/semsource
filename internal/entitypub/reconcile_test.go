package entitypub

import (
	"bytes"
	"context"
	"encoding/json"
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
	readErr    error         // returned by ReadAuthoritative when set
	blockRead  chan struct{} // when set, ReadAuthoritative waits on it or ctx
	recErr     []error       // returned by successive Reconcile calls, then nil
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

func (g *fakeGraph) ReadAuthoritative(ctx context.Context, id string) (*gtypes.ExactEntity, error) {
	g.mu.Lock()
	block := g.blockRead
	g.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
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

func settle(ctx context.Context, t *testing.T, pub *Publisher) {
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
	settle(ctx, t, pub)
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
	settle(ctx, t, pub)
	before := len(g.reconcileCalls())

	g.mu.Lock()
	g.hold = true
	g.mu.Unlock()
	send(t, pub, runPayload(5, false))
	// The write is acknowledged but not applied: no replace may run yet.
	waitUntil(ctx, t, func() bool { return pub.reconcilePending.Load() == 1 && len(g.queue2()) == 1 })
	if n := len(g.reconcileCalls()); n != before {
		t.Fatalf("reconcile replaced the group before its stream write applied (%d calls)", n-before)
	}
	g.release()
	settle(ctx, t, pub)

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
	settle(ctx, t, pub)

	g.mu.Lock()
	g.hold = true
	g.mu.Unlock()
	send(t, pub, runPayload(6, true))  // N-1 still carries the call
	send(t, pub, runPayload(5, false)) // N drops it
	waitUntil(ctx, t, func() bool { return len(g.queue2()) == 2 })
	g.release()
	settle(ctx, t, pub)

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
	settle(ctx, t, pub)
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
			settle(ctx, t, pub)
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
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
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
	waitUntil(ctx, t, func() bool { return pub.Published() == 1 })
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
func waitUntil(ctx context.Context, t *testing.T, cond func() bool) {
	t.Helper()
	for !cond() {
		select {
		case <-ctx.Done():
			t.Fatal("condition never held")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// Stored A, then queued B (adds the call) and A again (drops it), published
// back to back: the fence must not mistake the stored A for the final one.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestReconcileFenceTellsARepeatedWriteFromTheStoredOne(t *testing.T) {
	g := newFakeGraph()
	pub, ctx := startPublisher(t, g, WithReconcile(g))
	// Every publish reads the same wall-clock millisecond.
	frozen := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	pub.now = func() time.Time { return frozen }
	send(t, pub, runPayload(5, false))
	settle(ctx, t, pub)

	g.mu.Lock()
	g.hold = true
	g.mu.Unlock()
	send(t, pub, runPayload(5, true))
	send(t, pub, runPayload(5, false))
	waitUntil(ctx, t, func() bool { return len(g.queue2()) == 2 })
	g.release()
	settle(ctx, t, pub)

	if has(g.snapshot(reconcileID), semast.CodeCalls) {
		t.Error("the repeated write's reconcile ran against the stored copy; the call survived")
	}
	envs := g.envelopes
	for i := 1; i < len(envs); i++ {
		if !envs[i].After(envs[i-1]) {
			t.Errorf("envelope times not strictly increasing: %v then %v", envs[i-1], envs[i])
		}
	}
}

// Stop returns within its deadline while a reconcile is blocked in a request.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestStopIsNotHeldByAnInFlightReconcileRequest(t *testing.T) {
	g := newFakeGraph()
	g.blockRead = make(chan struct{}) // never released
	pub, err := New(g, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), WithReconcile(g))
	if err != nil {
		t.Fatal(err)
	}
	pub.Start(context.Background())
	send(t, pub, runPayload(5, false))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	waitUntil(ctx, t, func() bool { return pub.Published() == 1 })

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	done := make(chan error, 1)
	go func() { done <- pub.Stop(stopCtx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop held past its deadline by a blocked reconcile request")
	}
	if pub.ReconcileFailed() != 0 {
		t.Errorf("an abandoned shutdown reconcile counted as failed (%d)", pub.ReconcileFailed())
	}
}

// A dropped job (full queue) leaves no tracking entry behind.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestQueueOverflowLeavesNoTrackingEntry(t *testing.T) {
	g := newFakeGraph()
	g.blockRead = make(chan struct{})
	pub, err := New(g, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), WithReconcile(g))
	if err != nil {
		t.Fatal(err)
	}
	pub.reconcileQ = make(chan reconcileJob, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pub.Start(ctx)
	t.Cleanup(func() { _ = pub.Stop(context.Background()) })

	for i, id := range []string{"a", "b", "c"} {
		p := runPayload(5, false)
		p.ID = "acme.test.svc.golang.function." + id
		for j := range p.TripleData {
			p.TripleData[j].Subject = p.ID
		}
		send(t, pub, p)
		if i == 0 { // the worker takes the first job and blocks in its read
			waitUntil(ctx, t, func() bool { return pub.Published() == 1 && len(pub.reconcileQ) == 0 })
		}
	}
	waitUntil(ctx, t, func() bool { return pub.Published() == 3 })
	pub.latestMu.Lock()
	tracked := len(pub.latest)
	pub.latestMu.Unlock()
	if tracked != 2 || pub.ReconcileFailed() != 1 {
		t.Errorf("tracked=%d failed=%d, want 2 (running + queued) and 1 dropped", tracked, pub.ReconcileFailed())
	}
	close(g.blockRead)
	settle(ctx, t, pub)
	pub.latestMu.Lock()
	tracked = len(pub.latest)
	pub.latestMu.Unlock()
	if tracked != 0 {
		t.Errorf("tracking entries left after drain: %d", tracked)
	}
}

// A burst of distinct entities inside one millisecond shares that millisecond
// rather than running ahead of the clock, so a restarted publisher's write is
// not older than the stored one under the stream lane's merge rule.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestEnvelopeTimeBurstDoesNotOutrunTheClock(t *testing.T) {
	frozen := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	before := &Publisher{now: func() time.Time { return frozen }}
	var last time.Time
	for i := range 1000 {
		last = before.envelopeTime(fmt.Sprintf("e%d", i))
	}
	if !last.Equal(frozen) {
		t.Fatalf("burst of distinct entities ran ahead of the clock: %v, want %v", last, frozen)
	}
	if again := before.envelopeTime("e0"); !again.After(last) {
		t.Fatalf("a repeat of e0 in one millisecond shares its time %v", again)
	}

	const id = "acme.test.svc.golang.function.e0"
	stored := []message.Triple{{Subject: id, Predicate: semast.CodeDocComment, Object: "old", Source: envelopeSource, Timestamp: before.lastAt}}
	restarted := &Publisher{now: func() time.Time { return frozen.Add(2 * time.Millisecond) }}
	arrival := []message.Triple{{Subject: id, Predicate: semast.CodeDocComment, Object: "new", Source: envelopeSource, Timestamp: restarted.envelopeTime(id)}}
	if res := gtypes.ReplaceBySource(stored, arrival); len(res.Stale) != 0 {
		t.Fatalf("restarted publisher's write rejected as older: %+v", res.Stale)
	}
}

// A changed publish whose reconcile is still waiting, then an identical
// republish that skips its own: both writes apply before the first read. The
// waiting reconcile accepts the later identical write as its own applied and
// still removes the dropped call (#346 review round 4).
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: A republish leaves exactly the statements the source last emitted
func TestSkippedIdenticalFollowUpDoesNotStrandTheReconcile(t *testing.T) {
	g := newFakeGraph()
	pub, ctx := startPublisher(t, g, WithReconcile(g))
	seed := runPayload(5, true)
	seed.SkipReconcile = true
	send(t, pub, seed)
	settle(ctx, t, pub)

	g.mu.Lock()
	g.blockRead = make(chan struct{})
	blocked := g.blockRead
	g.mu.Unlock()
	send(t, pub, runPayload(5, false))
	waitUntil(ctx, t, func() bool { return pub.Published() == 2 })
	again := runPayload(5, false)
	again.SkipReconcile = true
	send(t, pub, again)
	waitUntil(ctx, t, func() bool { return pub.Published() == 3 })
	close(blocked)
	settle(ctx, t, pub)

	if has(g.snapshot(reconcileID), semast.CodeCalls) {
		t.Errorf("the dropped call survived; reconciled=%d failed=%d", pub.Reconciled(), pub.ReconcileFailed())
	}
	if pub.Reconciled() != 1 || pub.ReconcileFailed() != 0 {
		t.Errorf("reconciled=%d failed=%d; want 1 and 0", pub.Reconciled(), pub.ReconcileFailed())
	}
}
