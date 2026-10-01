package supersession

import (
	"context"
	"sync"
)

// requestGate serializes only this component's requests, with cancelable admission.
// It retains no work or outcomes beyond the running request.
type requestGate struct {
	once  sync.Once
	token chan struct{}
}

func (g *requestGate) acquire(ctx context.Context) (func(), error) {
	g.once.Do(func() { g.token = make(chan struct{}, 1); g.token <- struct{}{} })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-g.token:
	}
	if err := ctx.Err(); err != nil {
		g.token <- struct{}{}
		return nil, err
	}
	var release sync.Once
	return func() { release.Do(func() { g.token <- struct{}{} }) }, nil
}
