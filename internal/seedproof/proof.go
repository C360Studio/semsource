// Package seedproof collects otherwise non-fatal enumeration errors only while
// a source is proving a complete reactivation seed. Normal ingestion is unchanged.
package seedproof

import (
	"context"
	"errors"
	"sync"
)

type key struct{}

// Proof is scoped to one enumeration and retains no context.
type Proof struct {
	mu  sync.Mutex
	err error
}

// Begin creates a proof scope that producer helpers may report failures into.
func Begin(ctx context.Context) (context.Context, *Proof) {
	p := &Proof{}
	return context.WithValue(ctx, key{}, p), p
}

// Report makes a partial enumeration visible without changing ordinary ingest.
func Report(ctx context.Context, err error) {
	if err == nil {
		return
	}
	if p, ok := ctx.Value(key{}).(*Proof); ok {
		p.mu.Lock()
		p.err = errors.Join(p.err, err)
		p.mu.Unlock()
	}
}

// Err reports all failed enumeration work after the producer has joined it.
func (p *Proof) Err() error { p.mu.Lock(); defer p.mu.Unlock(); return p.err }

// Active reports whether a caller is proving a complete reactivation seed.
func Active(ctx context.Context) bool { _, ok := ctx.Value(key{}).(*Proof); return ok }
