package entitypub

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/seedproof"
	"github.com/c360studio/semsource/internal/sourceintent"
)

// BindSourceLifecycle installs immutable owner identity before Start.
func (p *Publisher) BindSourceLifecycle(binding sourceintent.Binding, observer sourceintent.PublicationObserver) error {
	p.receiptMu.Lock()
	defer p.receiptMu.Unlock()
	if p.started || p.observer != nil || p.buf.Size() != 0 {
		return fmt.Errorf("source lifecycle binding must precede publisher admission")
	}
	if observer == nil || binding.Handle == "" || binding.Generation == 0 || binding.BootEpoch == "" || binding.Namespace == "" || binding.Factory == "" || binding.ConfigDigest == "" {
		return fmt.Errorf("incomplete source lifecycle binding")
	}
	if err := binding.Authority.Validate(); err != nil {
		return err
	}
	p.binding = binding
	p.observer = observer
	p.tracked = make(map[*graph.EntityPayload]*seedBatch)
	p.batches = make(map[string]*seedBatch)
	return nil
}

type seedBatch struct {
	publisher  *Publisher
	id         string
	initial    bool
	automatic  bool
	mu         sync.Mutex
	entries    map[string]string
	pending    int
	closed     bool
	finishing  bool
	finished   bool
	superseded bool
	err        error
	changed    chan struct{}
	done       chan struct{}
}

func (b *seedBatch) signal() { close(b.changed); b.changed = make(chan struct{}) }

type plainBatch struct{ publisher *Publisher }

func (b plainBatch) Send(payload *graph.EntityPayload) error   { return b.publisher.enqueue(payload) }
func (b plainBatch) Finish(_ context.Context, err error) error { return err }

// BeginSeed creates a distinct enumeration. IDs cannot be reused within an epoch.
func (p *Publisher) BeginSeed(batchID string, initial bool) (sourceintent.SeedBatch, error) {
	p.receiptMu.Lock()
	defer p.receiptMu.Unlock()
	if p.receiptSealed {
		return nil, fmt.Errorf("publisher admission is closed")
	}
	if p.observer == nil {
		return plainBatch{p}, nil
	}
	if batchID == "" {
		return nil, fmt.Errorf("empty seed batch identity")
	}
	if _, exists := p.batches[batchID]; exists {
		return nil, fmt.Errorf("duplicate seed batch %q", batchID)
	}
	if initial && p.initialBegun {
		return nil, fmt.Errorf("initial seed already begun")
	}
	if !initial && !p.initialBegun {
		return nil, fmt.Errorf("live publication precedes initial seed")
	}
	batch := &seedBatch{publisher: p, id: batchID, initial: initial, entries: make(map[string]string), changed: make(chan struct{}), done: make(chan struct{})}
	p.batches[batchID] = batch
	if initial {
		p.initialBegun = true
	}
	return batch, nil
}

func (b *seedBatch) Send(payload *graph.EntityPayload) error {
	frozen, err := sourceintent.ClonePayload(payload)
	if err == nil {
		err = ValidatePayload(frozen)
	}
	if err == nil && !strings.HasPrefix(frozen.ID, b.publisher.binding.Authority.Org+"."+b.publisher.binding.Authority.Platform+".") {
		err = fmt.Errorf("publication authority does not match source binding")
	}
	fingerprint := ""
	if err == nil {
		fingerprint, err = sourceintent.PublicationFingerprint(frozen)
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return fmt.Errorf("seed enumeration is closed")
	}
	if err != nil {
		b.err = errors.Join(b.err, err)
		b.mu.Unlock()
		return err
	}
	if previous, exists := b.entries[frozen.ID]; exists && previous != fingerprint {
		err = fmt.Errorf("seed entity %s changed during enumeration", frozen.ID)
		b.err = errors.Join(b.err, err)
		b.mu.Unlock()
		return err
	}
	b.entries[frozen.ID] = fingerprint
	b.pending++
	if b.automatic {
		b.closed = true
	}
	b.mu.Unlock()
	p := b.publisher
	p.receiptMu.Lock()
	p.tracked[frozen] = b
	p.receiptMu.Unlock()
	if err := p.enqueue(frozen); err != nil {
		p.receiptMu.Lock()
		delete(p.tracked, frozen)
		p.receiptMu.Unlock()
		b.mu.Lock()
		b.pending--
		b.err = errors.Join(b.err, err)
		b.signal()
		b.mu.Unlock()
		return err
	}
	return nil
}

func (b *seedBatch) settled(ctx context.Context, err error) {
	b.mu.Lock()
	b.pending--
	if errors.Is(err, sourceintent.ErrSuperseded) {
		b.superseded = true
	} else {
		b.err = errors.Join(b.err, err)
	}
	b.signal()
	automatic := b.automatic && b.closed && b.pending == 0
	b.mu.Unlock()
	if automatic {
		_ = b.Finish(ctx, nil)
	}
}

// Finish closes enumeration and waits for every accepted payload, including the
// publisher's local drain batch. A canceled or failed seed cannot seal success.
func (b *seedBatch) Finish(ctx context.Context, enumerationErr error) error {
	b.mu.Lock()
	if b.finishing || b.finished {
		b.mu.Unlock()
		return fmt.Errorf("seed finish already called")
	}
	b.finishing = true
	b.closed = true
	b.err = errors.Join(b.err, enumerationErr)
	for b.pending > 0 && ctx.Err() == nil {
		changed := b.changed
		b.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
		}
		b.mu.Lock()
	}
	b.err = errors.Join(b.err, ctx.Err())
	err := b.err
	superseded := b.superseded
	entries := make([]sourceintent.ManifestEntry, 0, len(b.entries))
	for id, fingerprint := range b.entries {
		entries = append(entries, sourceintent.ManifestEntry{EntityID: id, Fingerprint: fingerprint})
	}
	b.mu.Unlock()
	if !superseded && ctx.Err() == nil {
		digest, digestErr := sourceintent.ManifestDigest(entries)
		if digestErr != nil {
			err = errors.Join(err, digestErr)
		} else {
			manifest := sourceintent.SeedManifest{Binding: b.publisher.binding, BatchID: b.id, Initial: b.initial, Successful: err == nil, Entries: entries, Digest: digest}
			if err != nil {
				manifest.Failure = &sourceintent.Blocker{Code: sourceintent.CodeSeedIncomplete, Message: err.Error(), Retryable: false}
			}
			writeErr := b.publisher.retryReceipt(ctx, func(callCtx context.Context) error { return b.publisher.observer.SeedFinished(callCtx, manifest) })
			if !errors.Is(writeErr, sourceintent.ErrSuperseded) {
				err = errors.Join(err, writeErr)
			}
		}
	}

	if err != nil {
		b.publisher.seedFailures.Add(1)
	}
	b.mu.Lock()
	b.err = err
	b.finished = true
	close(b.done)
	b.signal()
	b.mu.Unlock()
	if b.automatic {
		b.publisher.receiptMu.Lock()
		delete(b.publisher.batches, b.id)
		b.publisher.receiptMu.Unlock()
	}
	return err
}

func (p *Publisher) sendLive(payload *graph.EntityPayload) error {
	p.receiptMu.Lock()
	bound := p.observer != nil
	p.liveSequence++
	id := fmt.Sprintf("live-%d", p.liveSequence)
	p.receiptMu.Unlock()
	if !bound {
		return p.enqueue(payload)
	}
	batch, err := p.BeginSeed(id, false)
	if err != nil {
		return err
	}
	b := batch.(*seedBatch)
	b.mu.Lock()
	b.automatic = true
	b.mu.Unlock()
	err = b.Send(payload)
	if err != nil {
		b.mu.Lock()
		if !b.finished {
			b.finished = true
			b.closed = true
			close(b.done)
			b.signal()
		}
		b.mu.Unlock()
		p.receiptMu.Lock()
		delete(p.batches, b.id)
		p.receiptMu.Unlock()
		p.seedFailures.Add(1)
	}

	return err
}

func (p *Publisher) retryReceipt(ctx context.Context, write func(context.Context) error) error {
	p.receiptWork.Add(1)
	backoff := p.retryBackoff
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := write(callCtx)
		cancel()
		if errors.Is(err, sourceintent.ErrSuperseded) {
			p.receiptWork.Add(-1)
			return err
		}
		if err == nil {
			p.receiptWork.Add(-1)
			p.receiptFailing.Clear(p.logger, "source publication receipt persistence recovered")
			return nil
		}
		p.receiptFailing.Enter(p.logger, "source publication receipts are pending", "error", err)
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		backoff *= 2
		if backoff > p.maxRetryBackoff {
			backoff = p.maxRetryBackoff
		}
	}
}

// ReceiptErrors reports unresolved publication proof separately from transport loss.
func (p *Publisher) ReceiptErrors() int64 { return p.receiptWork.Load() + p.seedFailures.Load() }

func (p *Publisher) waitSeeds(ctx context.Context) error {
	p.receiptMu.Lock()
	batches := make([]*seedBatch, 0, len(p.batches))
	for _, b := range p.batches {
		batches = append(batches, b)
	}
	p.receiptMu.Unlock()
	for _, batch := range batches {
		select {
		case <-batch.done:
		case <-ctx.Done():
			return fmt.Errorf("source seed settlement incomplete: %w", ctx.Err())
		}
	}
	return nil
}

type batchContextKey struct{}

// SendContext carries an explicit seed through producer helper calls. Watch
// traffic outside that scope gets a separate one-publication batch.
func (p *Publisher) SendContext(ctx context.Context, payload *graph.EntityPayload) error {
	if batch, ok := ctx.Value(batchContextKey{}).(sourceintent.SeedBatch); ok {
		return batch.Send(payload)
	}
	return p.Send(payload)
}

// RunInitialSeed brackets enumeration before source watchers are started.
func (p *Publisher) RunInitialSeed(ctx context.Context, enumerate func(context.Context) error) error {
	batch, err := p.BeginSeed("initial", true)
	if err != nil {
		return err
	}
	seedCtx := context.WithValue(ctx, batchContextKey{}, batch)
	p.receiptMu.Lock()
	bound := p.observer != nil
	p.receiptMu.Unlock()
	var proof *seedproof.Proof
	if bound {
		seedCtx, proof = seedproof.Begin(seedCtx)
	}
	err = enumerate(seedCtx)
	if proof != nil {
		err = errors.Join(err, proof.Err())
	}
	return batch.Finish(ctx, err)
}
