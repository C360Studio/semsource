package sourcelifecycle

import (
	"context"
	"errors"
	"sync"

	"github.com/c360studio/semsource/internal/sourceintent"
)

// LocalProjector binds the product's single mutation owner before component Start.
// Calls stay synchronous so caller cancellation cannot leave a queued product RPC.
type LocalProjector struct {
	mu    sync.RWMutex
	owner sourceintent.Projector
}

// Bind accepts exactly one owner. The owner independently gates Start/Stop readiness.
func (p *LocalProjector) Bind(owner sourceintent.Projector) error {
	if owner == nil {
		return errors.New("nil local projector")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.owner != nil {
		return errors.New("local projector already bound")
	}
	p.owner = owner
	return nil
}
func (p *LocalProjector) current() (sourceintent.Projector, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.owner == nil {
		return nil, blocker(sourceintent.CodeMutation, "local mutation owner not constructed")
	}
	return p.owner, nil
}

// ApplyRemoval retains the caller's stack and gate throughout the owned effect.
func (p *LocalProjector) ApplyRemoval(ctx context.Context, r sourceintent.RemovalProjection) (sourceintent.ProjectionResult, error) {
	owner, err := p.current()
	if err != nil {
		return sourceintent.ProjectionResult{}, err
	}
	return owner.ApplyRemoval(ctx, r)
}

// ApplyReactivation uses the same explicitly bound local owner.
func (p *LocalProjector) ApplyReactivation(ctx context.Context, r sourceintent.ReactivationProjection) (sourceintent.ProjectionResult, error) {
	owner, err := p.current()
	if err != nil {
		return sourceintent.ProjectionResult{}, err
	}
	return owner.ApplyReactivation(ctx, r)
}
