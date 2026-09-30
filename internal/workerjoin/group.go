// Package workerjoin joins component-owned background work within a caller's
// deadline. Owners must stop seed/admission paths before waiting.
package workerjoin

import (
	"context"
	"sync"
)

// Group tracks workers, including work they start before they return. The zero
// value is ready to use. A failed Wait retains ownership for a later retry.
type Group struct {
	mu     sync.Mutex
	active int
	done   chan struct{}
	joins  []func(context.Context) error
}

// Go registers the worker before scheduling it. A running worker may register
// nested work because the group cannot settle until both have returned.
func (g *Group) Go(fn func()) {
	g.mu.Lock()
	if g.active == 0 {
		g.done = make(chan struct{})
	}
	g.active++
	g.mu.Unlock()
	go func() { defer g.finish(); fn() }()
}
func (g *Group) finish() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.active--
	if g.active == 0 {
		close(g.done)
	}
}

// Wait joins the currently admitted workers without creating a waiter goroutine.
// Callers must prevent new independent work once waiting begins.
func (g *Group) Wait(ctx context.Context) error {
	g.mu.Lock()
	done := g.done
	joins := append([]func(context.Context) error(nil), g.joins...)
	g.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for _, join := range joins {
		if err := join(ctx); err != nil {
			return err
		}
	}
	return nil
}

// JoinOnStop records an external producer completion barrier. Register it before
// starting its consumer and before calling Wait. Failed waits retain barriers.
func (g *Group) JoinOnStop(join func(context.Context) error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.joins = append(g.joins, join)
}

// Channel waits for a canceled handler's output to close, discarding unconsumed
// changes. A caller deadline reports incomplete ownership instead of hanging.
func Channel[T any](ctx context.Context, ch <-chan T) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-ch:
			if !ok {
				return nil
			}
		}
	}
}
