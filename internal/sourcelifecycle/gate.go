// Package sourcelifecycle owns retained source intents and their replay lifecycle.
package sourcelifecycle

import (
	"context"
	"sync"
)

// Gate serializes desired changes and one replay operation with cancelable admission.
// The zero value is ready for use. Receipt writes deliberately do not acquire it.
type Gate struct {
	once  sync.Once
	token chan struct{}
}

// Acquire returns the caller's exact release function; a canceled caller never enters.
func (g *Gate) Acquire(ctx context.Context) (func(), error) {
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
	var once sync.Once
	return func() { once.Do(func() { g.token <- struct{}{} }) }, nil
}
