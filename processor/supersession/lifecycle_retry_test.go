package supersession

import (
	"context"
	"errors"
	"testing"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/projection"
	"github.com/c360studio/semsource/graph"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// conflictMutator applies reconciles like replayMutator, but first refuses an
// entity's next conflicts[id] writes with revision_mismatch, or every write
// with fail when it is set.
type conflictMutator struct {
	entities  map[string]gtypes.EntityState
	conflicts map[string]int
	fail      error
	calls     int
}

func (m *conflictMutator) Reconcile(_ context.Context, r projection.ReconcileMutation) (projection.MutationReceipt, error) {
	m.calls++
	if m.fail != nil {
		return projection.MutationReceipt{}, m.fail
	}
	if m.conflicts[r.EntityID] > 0 {
		m.conflicts[r.EntityID]--
		return projection.MutationReceipt{}, &projection.MutationError{
			Kind: projection.MutationRevisionConflict, Code: gtypes.ErrorCodeRevisionMismatch,
			Commit: projection.CommitNotCommitted, Err: errors.New("revision mismatch: expected 8, current 11"),
		}
	}
	e := m.entities[r.EntityID]
	kept := []message.Triple{}
	for _, tr := range e.Triples {
		if tr.Predicate != source.EntityLifecycleStale {
			kept = append(kept, tr)
		}
	}
	e.Triples = append(kept, r.Desired...)
	m.entities[e.ID] = e
	return projection.MutationReceipt{Entity: &e, KVRevision: 2}, nil
}

// spec: openspec/specs/entity-staleness/spec.md#Requirement: Stale facts are distinguishable from live facts
func TestLifecyclePassRerunsOnRevisionMismatch(t *testing.T) {
	const id = "acme.semsource.docs.web.doc.a"
	stale := replayEntity(id, graph.LifecycleReasonFileDeleted)
	stale.Triples = append(stale.Triples, message.Triple{Subject: id, Predicate: source.EntityLifecycleOwner, Object: "owner-a"})
	live := replayEntity(id, "")
	live.Triples = append(live.Triples, message.Triple{Subject: id, Predicate: source.EntityLifecycleOwner, Object: "owner-a"})
	otherErr := &projection.MutationError{Kind: projection.MutationCommitUnknown, Code: "storage_unavailable", Err: errors.New("kv down")}

	tests := []struct {
		name      string
		entity    gtypes.EntityState
		absent    []string
		conflicts int
		fail      error
		wantCalls int
		wantErr   bool
		wantStale bool
	}{
		{name: "clear conflicts once then succeeds", entity: stale, absent: []string{}, conflicts: 1, wantCalls: 2},
		{name: "mark conflicts once then succeeds", entity: live, absent: []string{"guide.md"}, conflicts: 1, wantCalls: 2, wantStale: true},
		{name: "persistent conflict stops after three attempts", entity: stale, absent: []string{}, conflicts: 99, wantCalls: 3, wantErr: true, wantStale: true},
		{name: "other error is not retried", entity: stale, absent: []string{}, fail: otherErr, wantCalls: 1, wantErr: true, wantStale: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &replayRequester{pages: map[string]gtypes.PrefixQueryResponse{"": {Entities: []gtypes.EntityState{tt.entity}}}}
			m := &conflictMutator{entities: map[string]gtypes.EntityState{id: tt.entity}, conflicts: map[string]int{id: tt.conflicts}, fail: tt.fail}
			c := replayComponent(p, nil)
			c.mutClient = m
			_, err := c.runLifecyclePass(context.Background(), graph.LifecycleRunRequest{
				Org: "acme", Systems: []string{"docs"}, Reason: graph.LifecycleReasonFileDeleted, Owner: "owner-a", Absent: tt.absent,
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			var me *projection.MutationError
			if tt.wantErr && tt.fail == nil && (!errors.As(err, &me) || me.Code != gtypes.ErrorCodeRevisionMismatch) {
				t.Fatalf("persistent conflict surfaced %v, want revision_mismatch", err)
			}
			if m.calls != tt.wantCalls {
				t.Fatalf("reconcile calls = %d, want %d", m.calls, tt.wantCalls)
			}
			if got := countObject(m.entities[id], source.EntityLifecycleStale) > 0; got != tt.wantStale {
				t.Fatalf("stale = %v, want %v", got, tt.wantStale)
			}
		})
	}
}

func countObject(e gtypes.EntityState, predicate string) int {
	n := 0
	for _, tr := range e.Triples {
		if tr.Predicate == predicate {
			n++
		}
	}
	return n
}
