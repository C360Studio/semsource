package supersession

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/sourceintent"
	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/pkg/projection"
	"github.com/nats-io/nats.go"
)

type replayEffectFence struct {
	active     *sourceintent.EffectAttempt
	begins     []string
	outcomes   []sourceintent.EffectOutcome
	beginErr   error
	resolveErr error
}

func (f *replayEffectFence) Begin(_ context.Context, id string) (sourceintent.EffectAttempt, error) {
	if f.beginErr != nil {
		return sourceintent.EffectAttempt{}, f.beginErr
	}
	if f.active != nil {
		return sourceintent.EffectAttempt{}, projectionBlocker(sourceintent.CodeEffectUnresolved, "earlier effect unresolved")
	}
	attempt := sourceintent.EffectAttempt{ID: fmt.Sprintf("attempt-%d", len(f.begins)+1), Binding: replayBinding(), EntityID: id, StartedAt: time.Now(), Outcome: sourceintent.EffectUnknown}
	f.begins = append(f.begins, id)
	f.active = &attempt
	return attempt, nil
}
func (f *replayEffectFence) Resolve(_ context.Context, attempt sourceintent.EffectAttempt, outcome sourceintent.EffectOutcome) error {
	if f.active == nil || *f.active != attempt {
		return errors.New("wrong effect attempt")
	}
	f.outcomes = append(f.outcomes, outcome)
	if f.resolveErr != nil {
		return f.resolveErr
	}
	if outcome == sourceintent.EffectUnknown {
		return projectionBlocker(sourceintent.CodeEffectUnresolved, "effect outcome unknown")
	}
	f.active = nil
	return nil
}

type receiptReplayMutator struct {
	*replayMutator
	fence    *replayEffectFence
	response func(projection.ReconcileMutation) (projection.MutationReceipt, error)
	calls    []string
}

func (m *receiptReplayMutator) Reconcile(_ context.Context, r projection.ReconcileMutation) (projection.MutationReceipt, error) {
	m.calls = append(m.calls, r.EntityID)
	if m.fence == nil || m.fence.active == nil || m.fence.active.EntityID != r.EntityID {
		return projection.MutationReceipt{}, errors.New("mutation dispatched before durable effect fence")
	}
	if r.Metadata.RequestID != m.fence.active.ID || !r.Metadata.Timestamp.Equal(m.fence.active.StartedAt) {
		return projection.MutationReceipt{}, errors.New("mutation metadata lost durable attempt identity")
	}
	return m.response(r)
}

func TestRemovalEffectFenceRequiredBeforeMutation(t *testing.T) {
	for _, mode := range []string{"missing", "write_failed"} {
		t.Run(mode, func(t *testing.T) {
			e := replayEntity("acme.semsource.docs.web.doc.a", "")
			m := &replayMutator{entities: map[string]gtypes.EntityState{e.ID: e}}
			c := replayComponent(&replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{e}}}}, m)
			request := replayRemoval()
			request.Effects = nil
			if mode == "write_failed" {
				request.Effects = &replayEffectFence{beginErr: errors.New("fence persistence uncertain")}
			}
			result, err := c.ApplyRemoval(t.Context(), request)
			if err == nil || result.Complete || result.Marked != 0 || len(m.writes) != 0 {
				t.Fatalf("unfenced dispatch: result=%+v error=%v writes=%v", result, err, m.writes)
			}
		})
	}
}

func TestRemovalEffectFenceClassifiesCommitEvidence(t *testing.T) {
	a := replayEntity("acme.semsource.docs.web.doc.a", "")
	b := replayEntity("acme.semsource.docs.web.doc.b", "")
	ack := replayEntity(a.ID, graph.LifecycleReasonSourceRemoved)
	tests := []struct {
		name          string
		receipt       projection.MutationReceipt
		err           error
		outcome       sourceintent.EffectOutcome
		calls, marked int
	}{
		{"verified", projection.MutationReceipt{Commit: projection.CommitVerified, Entity: &ack, KVRevision: 2}, nil, sourceintent.EffectVerified, 2, 2},
		{"verified_missing_entity", projection.MutationReceipt{Commit: projection.CommitVerified, KVRevision: 2}, nil, sourceintent.EffectUnknown, 1, 0},
		{"verified_missing_revision", projection.MutationReceipt{Commit: projection.CommitVerified, Entity: &ack}, nil, sourceintent.EffectUnknown, 1, 0},
		{"verified_without_requested_marker", projection.MutationReceipt{Commit: projection.CommitVerified, Entity: &a, KVRevision: 2}, nil, sourceintent.EffectUnknown, 1, 0},
		{"invalid_before_dispatch", projection.MutationReceipt{Commit: projection.CommitNotCommitted}, &projection.MutationError{Kind: projection.MutationInvalid, Commit: projection.CommitNotCommitted, Err: errors.New("invalid contract")}, sourceintent.EffectNotCommitted, 2, 1},
		{"not_found", projection.MutationReceipt{Commit: projection.CommitNotCommitted}, &projection.MutationError{Kind: projection.MutationNotFound, Code: gtypes.ErrorCodeEntityNotFound, Commit: projection.CommitNotCommitted, Err: errors.New("missing entity")}, sourceintent.EffectNotCommitted, 2, 1},
		{"not_committed_without_error", projection.MutationReceipt{Commit: projection.CommitNotCommitted}, nil, sourceintent.EffectUnknown, 1, 0},
		{"empty_receipt_nil_error", projection.MutationReceipt{}, nil, sourceintent.EffectUnknown, 1, 0},
		{"unknown", projection.MutationReceipt{Commit: projection.CommitUnknown}, &projection.MutationError{Kind: projection.MutationCommitUnknown, Commit: projection.CommitUnknown, Err: errors.New("reply lost")}, sourceintent.EffectUnknown, 1, 0},
		{"internal_nominally_not_committed", projection.MutationReceipt{Commit: projection.CommitNotCommitted}, &projection.MutationError{Kind: projection.MutationInternal, Code: gtypes.ErrorCodeInternal, Commit: projection.CommitNotCommitted, Err: errors.New("KV update timeout")}, sourceintent.EffectUnknown, 1, 0},
		{"revision_rejected", projection.MutationReceipt{Commit: projection.CommitNotCommitted}, &projection.MutationError{Kind: projection.MutationRevisionConflict, Code: gtypes.ErrorCodeRevisionMismatch, Commit: projection.CommitNotCommitted, Err: errors.New("stale revision")}, sourceintent.EffectNotCommitted, 2, 1},
		{"no_responders", projection.MutationReceipt{Commit: projection.CommitNotCommitted}, &projection.MutationError{Kind: projection.MutationUnavailable, Commit: projection.CommitNotCommitted, Err: nats.ErrNoResponders}, sourceintent.EffectNotCommitted, 2, 1},
		{"unclassified_not_committed", projection.MutationReceipt{Commit: projection.CommitNotCommitted}, errors.New("unspecified rejection"), sourceintent.EffectUnknown, 1, 0},
		{"verified_with_error", projection.MutationReceipt{Commit: projection.CommitVerified, Entity: &ack, KVRevision: 2}, errors.New("contradictory reply"), sourceintent.EffectUnknown, 1, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fence := &replayEffectFence{}
			base := &replayMutator{entities: map[string]gtypes.EntityState{a.ID: a, b.ID: b}}
			m := &receiptReplayMutator{replayMutator: base, fence: fence, response: func(r projection.ReconcileMutation) (projection.MutationReceipt, error) {
				if r.EntityID == a.ID {
					return tc.receipt, tc.err
				}
				return base.Reconcile(t.Context(), r)
			}}
			c := replayComponent(&replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{a, b}}}}, base)
			c.mutClient = m
			request := replayRemoval()
			request.Effects = fence
			result, err := c.ApplyRemoval(t.Context(), request)
			if len(m.calls) != tc.calls || result.Marked != tc.marked || len(fence.outcomes) == 0 || fence.outcomes[0] != tc.outcome {
				t.Fatalf("result=%+v error=%v calls=%v outcomes=%v", result, err, m.calls, fence.outcomes)
			}
			if tc.outcome == sourceintent.EffectUnknown && (err == nil || result.Complete || fence.active == nil) {
				t.Fatalf("uncertain effect cleared: %+v %v fence=%+v", result, err, fence.active)
			}
			if tc.outcome != sourceintent.EffectUnknown && fence.active != nil {
				t.Fatal("terminal evidence retained an unresolved fence")
			}
		})
	}
}

func TestRemovalEffectFenceResolutionFailureStopsPass(t *testing.T) {
	a := replayEntity("acme.semsource.docs.web.doc.a", "")
	b := replayEntity("acme.semsource.docs.web.doc.b", "")
	fence := &replayEffectFence{resolveErr: errors.New("terminal journal write failed")}
	base := &replayMutator{entities: map[string]gtypes.EntityState{a.ID: a, b.ID: b}}
	m := &receiptReplayMutator{replayMutator: base, fence: fence, response: func(r projection.ReconcileMutation) (projection.MutationReceipt, error) {
		return base.Reconcile(t.Context(), r)
	}}
	c := replayComponent(&replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{a, b}}}}, base)
	c.mutClient = m
	request := replayRemoval()
	request.Effects = fence
	result, err := c.ApplyRemoval(t.Context(), request)
	if err == nil || result.Complete || result.Marked != 1 || len(m.calls) != 1 || fence.active == nil {
		t.Fatalf("failed fence resolution lost ownership: %+v %v calls=%v fence=%+v", result, err, m.calls, fence.active)
	}
}

func TestLegacyLifecycleCannotRequestSourceRemoved(t *testing.T) {
	e := replayEntity("acme.semsource.docs.web.doc.a", "")
	m := &replayMutator{entities: map[string]gtypes.EntityState{e.ID: e}}
	c := replayComponent(&replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{e}}}}, m)
	_, err := c.runLifecyclePass(t.Context(), graph.LifecycleRunRequest{Org: "acme", Systems: []string{"docs"}, Reason: graph.LifecycleReasonSourceRemoved})
	if err == nil || len(m.writes) != 0 {
		t.Fatalf("legacy source removal bypass: error=%v writes=%v", err, m.writes)
	}
}

type heldProjectionRead struct {
	delegate                   *replayMutator
	entered, canceled, release chan struct{}
}

func (m *heldProjectionRead) ReadAuthoritative(ctx context.Context, _ string) (*gtypes.ExactEntity, error) {
	close(m.entered)
	<-ctx.Done()
	close(m.canceled)
	<-m.release
	return nil, ctx.Err()
}
func (m *heldProjectionRead) Reconcile(ctx context.Context, r projection.ReconcileMutation) (projection.MutationReceipt, error) {
	return m.delegate.Reconcile(ctx, r)
}

func TestDirectProjectionStopJoinsCanceledCall(t *testing.T) {
	e := replayEntity("acme.semsource.docs.web.doc.a", "")
	base := &replayMutator{entities: map[string]gtypes.EntityState{e.ID: e}}
	m := &heldProjectionRead{delegate: base, entered: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	c := replayComponent(&replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{e}}}}, base)
	c.mutClient = m
	c.running = true
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.ApplyRemoval(ctx, replayRemoval()); done <- err }()
	select {
	case <-m.entered:
	case <-time.After(time.Second):
		t.Fatal("direct call never entered")
	}
	cancel()
	select {
	case <-m.canceled:
	case <-time.After(time.Second):
		t.Fatal("direct call missed cancellation")
	}
	expired, expire := context.WithCancel(t.Context())
	expire()
	stopErr := c.Stop(expired)
	c.mu.RLock()
	retained := c.mutClient != nil
	c.mu.RUnlock()
	close(m.release)
	select {
	case err := <-done:
		if err == nil {
			t.Error("canceled direct call succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("direct call did not return")
	}
	if stopErr == nil || !retained {
		t.Fatalf("Stop abandoned active direct call: error=%v retained=%v", stopErr, retained)
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplyRemoval(t.Context(), replayRemoval()); err == nil {
		t.Fatal("stopped projector admitted new work")
	}
	if len(base.writes) != 0 {
		t.Fatalf("canceled call produced effects: %v", base.writes)
	}
}

type alteredEffectFence struct {
	replayEffectFence
	alter func(*sourceintent.EffectAttempt)
}

func (f *alteredEffectFence) Begin(ctx context.Context, id string) (sourceintent.EffectAttempt, error) {
	attempt, err := f.replayEffectFence.Begin(ctx, id)
	if err == nil {
		f.alter(&attempt)
	}
	return attempt, err
}
func TestRemovalEffectFenceAuthorizesExactAttempt(t *testing.T) {
	tests := map[string]func(*sourceintent.EffectAttempt){
		"missing_id":        func(a *sourceintent.EffectAttempt) { a.ID = "" },
		"different_binding": func(a *sourceintent.EffectAttempt) { a.Binding.Generation++ },
		"different_entity":  func(a *sourceintent.EffectAttempt) { a.EntityID = "acme.semsource.sibling.web.doc.a" },
		"missing_time":      func(a *sourceintent.EffectAttempt) { a.StartedAt = time.Time{} },
		"already_terminal":  func(a *sourceintent.EffectAttempt) { a.Outcome = sourceintent.EffectVerified },
	}
	for name, alter := range tests {
		t.Run(name, func(t *testing.T) {
			e := replayEntity("acme.semsource.docs.web.doc.a", "")
			m := &replayMutator{entities: map[string]gtypes.EntityState{e.ID: e}}
			c := replayComponent(&replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{e}}}}, m)
			request := replayRemoval()
			request.Effects = &alteredEffectFence{alter: alter}
			result, err := c.ApplyRemoval(t.Context(), request)
			if err == nil || result.Complete || len(m.writes) != 0 {
				t.Fatalf("wrong fence authorized mutation: %+v %v writes=%v", result, err, m.writes)
			}
		})
	}
}
func TestDirectProjectionCannotEnterAfterStopBegins(t *testing.T) {
	e := replayEntity("acme.semsource.docs.web.doc.a", "")
	m := &replayMutator{entities: map[string]gtypes.EntityState{e.ID: e}}
	c := replayComponent(&replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{e}}}}, m)
	release, err := c.runGate.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	expired, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.Stop(expired); err == nil {
		release()
		t.Fatal("Stop bypassed held run gate")
	}
	release()
	result, err := c.ApplyRemoval(t.Context(), replayRemoval())
	if err == nil || result.Complete || len(m.writes) != 0 {
		t.Fatalf("stopping owner admitted effects: %+v %v writes=%v", result, err, m.writes)
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}
