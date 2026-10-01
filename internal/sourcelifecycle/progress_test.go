package sourcelifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semsource/internal/sourceintent"
)

type progressProjection struct {
	result sourceintent.ProjectionResult
	err    error
}

func (p *progressProjection) ApplyRemoval(context.Context, sourceintent.RemovalProjection) (sourceintent.ProjectionResult, error) {
	return p.result, p.err
}
func (p *progressProjection) ApplyReactivation(context.Context, sourceintent.ReactivationProjection) (sourceintent.ProjectionResult, error) {
	return sourceintent.ProjectionResult{}, errors.New("unexpected reactivation")
}
func TestRepeatedRetainedPassPreservesVerifiedEntityCount(t *testing.T) {
	c, _, j, _ := prepareRetired(t)
	c.tail = proofTail{proof: sourceintent.TailProofCurrentRetained}
	p := &progressProjection{result: sourceintent.ProjectionResult{Enumerated: 3, Marked: 3, Complete: true}}
	c.projector = p
	for pass := 0; pass < 3; pass++ {
		err := c.ReconcileOnce(context.Background())
		var b *sourceintent.Blocker
		if !errors.As(err, &b) || b.Code != sourceintent.CodeAppliedTailUnproven {
			t.Fatalf("pass %d err=%v", pass, err)
		}
		r, _, err := j.Get(context.Background(), "docs")
		if err != nil {
			t.Fatal(err)
		}
		if r.Progress.CompletedCount != 3 || r.Phase != sourceintent.Pending || r.Progress.Blocker.Code != sourceintent.CodeAppliedTailUnproven {
			t.Fatalf("pass %d status=%+v", pass, r)
		}
		// The same three retained entities already carry the marker on later passes.
		p.result.Marked = 0
	}
}
func TestIncompleteRetainedPassCountsOnlyConfirmedMutations(t *testing.T) {
	for _, test := range []struct {
		name   string
		result sourceintent.ProjectionResult
		err    error
		want   int
	}{
		{"partial mutation", sourceintent.ProjectionResult{Enumerated: 3, Marked: 1}, blocker(sourceintent.CodeMutation, "one mutation failed"), 1},
		{"cancel before mutation", sourceintent.ProjectionResult{Enumerated: 3}, context.Canceled, 0},
		{"cancel after mutation", sourceintent.ProjectionResult{Enumerated: 3, Marked: 1}, context.Canceled, 1},
		{"incomplete without error", sourceintent.ProjectionResult{Enumerated: 3, Marked: 1}, nil, 1},
		{"contradictory completion", sourceintent.ProjectionResult{Enumerated: 3, Complete: true}, blocker(sourceintent.CodeMutation, "reply failed"), 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _, j, _ := prepareRetired(t)
			c.tail = proofTail{proof: sourceintent.TailProofCurrentRetained}
			c.projector = &progressProjection{result: test.result, err: test.err}
			if c.ReconcileOnce(context.Background()) == nil {
				t.Fatal("incomplete pass claimed success")
			}
			r, _, err := j.Get(context.Background(), "docs")
			if err != nil {
				t.Fatal(err)
			}
			if r.Progress.CompletedCount != test.want || r.Phase != sourceintent.Pending {
				t.Fatalf("status=%+v want count=%d", r, test.want)
			}
		})
	}
}
