package sourcelifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/c360studio/semsource/internal/sourceintent"
)

type effectProjection struct {
	fn func(context.Context, sourceintent.RemovalProjection) (sourceintent.ProjectionResult, error)
}

func (p effectProjection) ApplyRemoval(ctx context.Context, r sourceintent.RemovalProjection) (sourceintent.ProjectionResult, error) {
	return p.fn(ctx, r)
}
func (effectProjection) ApplyReactivation(context.Context, sourceintent.ReactivationProjection) (sourceintent.ProjectionResult, error) {
	return sourceintent.ProjectionResult{}, errors.New("unexpected reactivation")
}

type afterCommitJournal struct{ sourceintent.Journal }

func (j afterCommitJournal) Update(ctx context.Context, r sourceintent.Record, rev uint64) (uint64, error) {
	n, err := j.Journal.Update(ctx, r, rev)
	if err != nil {
		return n, err
	}
	return n, errors.New("journal committed but response lost")
}
func effectID(b sourceintent.Binding) string { return b.Authority.Build("manuals", "web", "doc", "a") }
func TestDirectReplayRetainsUnknownEffectBeforeAdmission(t *testing.T) {
	c, s, j, _ := prepareRetired(t)
	ctx := context.Background()
	calls := 0
	c.projector = effectProjection{fn: func(ctx context.Context, r sourceintent.RemovalProjection) (sourceintent.ProjectionResult, error) {
		calls++
		if r.Effects == nil {
			t.Fatal("mutation has no fence")
		}
		attempt, err := r.Effects.Begin(ctx, effectID(r.Binding))
		if err != nil {
			t.Fatal(err)
		}
		saved, _, err := j.Get(ctx, "docs")
		if err != nil || saved.Effect == nil || saved.Effect.ID != attempt.ID {
			t.Fatal("mutation preceded retained fence")
		}
		return sourceintent.ProjectionResult{Enumerated: 1}, context.DeadlineExceeded
	}}
	if c.ReconcileOnce(ctx) == nil {
		t.Fatal("uncertain effect claimed success")
	}
	if c.CheckAdmission(ctx) == nil {
		t.Fatal("unknown effect permits source config writes")
	}
	if c.ReconcileOnce(ctx) == nil || calls != 1 {
		t.Fatal("unknown effect was retried")
	}
	cc := s.components["docs"]
	cc.Enabled = true
	s.components["docs"] = cc
	if c.RecordReadd(ctx, "docs") == nil {
		t.Fatal("generation changed while effect unknown")
	}
	if _, err := c.BindBoot(ctx); err == nil {
		t.Fatal("boot admitted possibly stale producer")
	}
}
func TestEffectTerminalEvidenceSurvivesProgressAndLostReply(t *testing.T) {
	for _, outcome := range []sourceintent.EffectOutcome{sourceintent.EffectVerified, sourceintent.EffectNotCommitted} {
		t.Run(string(outcome), func(t *testing.T) {
			c, _, j, _ := prepareRetired(t)
			ctx := context.Background()
			c.projector = effectProjection{fn: func(ctx context.Context, r sourceintent.RemovalProjection) (sourceintent.ProjectionResult, error) {
				a, err := r.Effects.Begin(ctx, effectID(r.Binding))
				if err != nil {
					t.Fatal(err)
				}
				if err := r.Effects.Resolve(ctx, a, outcome); err != nil {
					t.Fatal(err)
				}
				return sourceintent.ProjectionResult{Enumerated: 1, Marked: 1, Complete: true}, nil
			}}
			if err := c.ReconcileOnce(ctx); err != nil {
				t.Fatal(err)
			}
			r, _, err := j.Get(ctx, "docs")
			if err != nil || r.Effect == nil || r.Effect.Outcome != outcome || r.Phase != sourceintent.Complete {
				t.Fatalf("terminal evidence overwritten: %+v %v", r, err)
			}
			if err := c.CheckAdmission(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
	c, _, j, _ := prepareRetired(t)
	ctx := context.Background()
	r, _, _ := j.Get(ctx, "docs")
	f := c.effectFence(r.Binding)
	a, err := f.Begin(ctx, effectID(r.Binding))
	if err != nil {
		t.Fatal(err)
	}
	c.journal = afterCommitJournal{Journal: j}
	if err := f.Resolve(ctx, a, sourceintent.EffectVerified); err == nil {
		t.Fatal("lost journal response hidden")
	}
	got, _, err := j.Get(ctx, "docs")
	if err != nil || got.Effect == nil || got.Effect.Outcome != sourceintent.EffectVerified {
		t.Fatalf("resolution erased evidence: %+v %v", got, err)
	}
	c.journal = j
	if err := c.CheckAdmission(ctx); err != nil {
		t.Fatalf("authoritative verified evidence ignored: %v", err)
	}
}
func TestEffectBeginAmbiguityAndExactResolution(t *testing.T) {
	c, _, j, _ := prepareRetired(t)
	ctx := context.Background()
	r, _, _ := j.Get(ctx, "docs")
	f := c.effectFence(r.Binding)
	c.journal = afterCommitJournal{Journal: j}
	if _, err := f.Begin(ctx, effectID(r.Binding)); err == nil {
		t.Fatal("ambiguous begin authorized effect")
	}
	c.journal = j
	if c.CheckAdmission(ctx) == nil {
		t.Fatal("ambiguous begin fence disappeared")
	}
	current, _, _ := j.Get(ctx, "docs")
	a := *current.Effect
	wrong := a
	wrong.ID = "other"
	if f.Resolve(ctx, wrong, sourceintent.EffectVerified) == nil {
		t.Fatal("wrong attempt cleared fence")
	}
	if f.Resolve(ctx, a, sourceintent.EffectUnknown) == nil || c.CheckAdmission(ctx) == nil {
		t.Fatal("unknown resolved fence")
	}
	if f.Resolve(ctx, a, "invented") == nil {
		t.Fatal("unrecognized outcome accepted")
	}
	if err := f.Resolve(ctx, a, sourceintent.EffectNotCommitted); err != nil {
		t.Fatal(err)
	}
	if err := f.Resolve(ctx, a, sourceintent.EffectNotCommitted); err != nil {
		t.Fatal("exact terminal retry failed", err)
	}
	if f.Resolve(ctx, a, sourceintent.EffectVerified) == nil {
		t.Fatal("contradictory terminal evidence accepted")
	}
}
func TestEffectBootAdmitsOnlyNonoverlappingExistingSources(t *testing.T) {
	c, s, j, _ := prepareRetired(t)
	ctx := context.Background()
	r, _, _ := j.Get(ctx, "docs")
	if _, err := c.effectFence(r.Binding).Begin(ctx, effectID(r.Binding)); err != nil {
		t.Fatal(err)
	}
	sibling := s.components["docs"]
	sibling.Enabled = true
	sibling.Config = json.RawMessage(`{"paths":["/other"],"project":"other"}`)
	s.components["other"] = sibling
	if _, err := c.BindBoot(ctx); err != nil {
		t.Fatal("stable disjoint boot rejected", err)
	}
	sibling.Config = json.RawMessage(`{"paths":["/overlap"],"project":"manuals"}`)
	s.components["other"] = sibling
	if _, err := c.BindBoot(ctx); err == nil {
		t.Fatal("overlapping producer admitted beside uncertain effect")
	}
}
func TestLocalProjectorRequiresOneSynchronousBinding(t *testing.T) {
	var bridge LocalProjector
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := bridge.ApplyRemoval(ctx, sourceintent.RemovalProjection{}); err == nil {
		t.Fatal("unbound projection admitted")
	}
	if bridge.Bind(nil) == nil {
		t.Fatal("nil projection accepted")
	}
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	p := effectProjection{fn: func(context.Context, sourceintent.RemovalProjection) (sourceintent.ProjectionResult, error) {
		close(started)
		<-release
		return sourceintent.ProjectionResult{Complete: true}, nil
	}}
	if err := bridge.Bind(p); err != nil {
		t.Fatal(err)
	}
	if bridge.Bind(p) == nil {
		t.Fatal("projector rebound")
	}
	go func() { defer close(done); _, _ = bridge.ApplyRemoval(ctx, sourceintent.RemovalProjection{}) }()
	<-started
	cancel()
	select {
	case <-done:
		t.Fatal("bridge abandoned owned direct effect after caller cancel")
	default:
	}
	close(release)
	<-done
}

func TestJournalRejectsCorruptOrDiscardedEffect(t *testing.T) {
	for _, kind := range []string{"id", "outcome", "foreign_entity", "foreign_binding", "generation", "time"} {
		t.Run(kind, func(t *testing.T) {
			c, _, j, _ := prepareRetired(t)
			ctx := context.Background()
			r, _, _ := j.Get(ctx, "docs")
			attempt, err := c.effectFence(r.Binding).Begin(ctx, effectID(r.Binding))
			if err != nil {
				t.Fatal(err)
			}
			r, _, _ = j.Get(ctx, "docs")
			switch kind {
			case "id":
				attempt.ID = ""
			case "outcome":
				attempt.Outcome = "invented"
			case "foreign_entity":
				attempt.EntityID = "foreign.p.manuals.web.doc.a"
			case "foreign_binding":
				attempt.Binding.Namespace = "other"
			case "generation":
				attempt.Binding.Generation++
			case "time":
				attempt.StartedAt = sourceintent.EffectAttempt{}.StartedAt
			}
			r.Effect = &attempt
			raw, _ := json.Marshal(r)
			key, _ := j.recordKey("docs")
			j.kv.(*memoryKV).set(key, raw)
			if _, _, err := j.Get(ctx, "docs"); err == nil {
				t.Fatal("corrupt effect granted admission")
			}
		})
	}
	c, _, j, _ := prepareRetired(t)
	ctx := context.Background()
	r, _, _ := j.Get(ctx, "docs")
	if _, err := c.effectFence(r.Binding).Begin(ctx, effectID(r.Binding)); err != nil {
		t.Fatal(err)
	}
	r, rev, _ := j.Get(ctx, "docs")
	r.Effect = nil
	if _, err := j.Update(ctx, r, rev); !errors.Is(err, sourceintent.ErrConflict) {
		t.Fatalf("unresolved attempt erased: %v", err)
	}
}
func TestEffectCannotAuthorizeStaleGenerationOrChangedDesired(t *testing.T) {
	c, s, j, _ := prepareRetired(t)
	ctx := context.Background()
	r, _, _ := j.Get(ctx, "docs")
	f := c.effectFence(r.Binding)
	if _, err := f.Begin(ctx, "acme.test-a1b2c3.foreign.web.doc.a"); err == nil {
		t.Fatal("foreign scope authorized")
	}
	c.journal = &faultJournal{Journal: j, errorUpdate: errors.New("write failed")}
	if _, err := f.Begin(ctx, effectID(r.Binding)); err == nil {
		t.Fatal("failed begin authorized effect")
	}
	c.journal = j
	cc := s.components["docs"]
	cc.Enabled = true
	s.components["docs"] = cc
	if _, err := f.Begin(ctx, effectID(r.Binding)); err == nil {
		t.Fatal("enabled desired source authorized old effect")
	}
	if err := c.RecordReadd(ctx, "docs"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Begin(ctx, effectID(r.Binding)); !errors.Is(err, sourceintent.ErrConflict) {
		t.Fatalf("stale generation effect: %v", err)
	}
}
