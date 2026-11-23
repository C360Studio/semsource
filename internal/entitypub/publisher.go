// Package entitypub provides a buffered entity publisher for semsource source
// components. It decouples entity ingestion from NATS publishing using a
// circular buffer, providing backpressure handling when the NATS circuit
// breaker trips.
//
// Usage:
//
//	pub := entitypub.New(natsClient, logger)
//	pub.Start(ctx)
//	defer pub.Stop(ctx)
//	pub.Send(payload) // non-blocking, buffered
package entitypub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/buffer"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/degraded"
)

const (
	// graphIngestSubject is the NATS subject for graph entity ingestion.
	graphIngestSubject = "graph.ingest.entity"

	// defaultCapacity is the default buffer size. Large enough to absorb a
	// burst from a big repo (thousands of Java files) without dropping.
	defaultCapacity = 5000

	// defaultBatchSize is how many entities to drain per read cycle.
	defaultBatchSize = 50

	// defaultDrainInterval is the ticker interval for the drain loop.
	defaultDrainInterval = 5 * time.Millisecond

	// circuitOpenBackoff is how long to wait when the circuit breaker is open.
	circuitOpenBackoff = 500 * time.Millisecond

	// maxCircuitOpenBackoff caps the exponential retry backoff.
	maxCircuitOpenBackoff = 10 * time.Second

	// defaultSendTimeout bounds how long Send applies backpressure when the
	// buffer is full before dropping loudly. Bounded so a wedged NATS cannot
	// silently freeze ingest loops forever; loud so a drop is never invisible.
	defaultSendTimeout = 5 * time.Second

	// deliveryBudget caps one entity's total retry wall-clock. It must stay
	// inside the GRAPH stream's duplicate-detection window (server default
	// 2m), because the Nats-Msg-Id dedup that makes retrying SAFE — the
	// ingest merge APPENDS triples, so an ambiguous timeout retried without
	// dedup double-applies (ADR-055 §5) — only holds within that window.
	deliveryBudget = 90 * time.Second

	// maxPublishAttempts bounds attempts within the budget; with the 500ms→10s
	// backoff the elapsed ceiling binds first (~13 attempts).
	maxPublishAttempts = 20
)

// NATSPublisher is the subset of natsclient.Client needed for publishing.
type NATSPublisher interface {
	PublishToStream(ctx context.Context, subject string, data []byte) error
	// PublishToStreamWithMsgID stamps Nats-Msg-Id so the stream's duplicate
	// window collapses ambiguous retries to one stored message (ADR-055 §5).
	PublishToStreamWithMsgID(ctx context.Context, subject string, data []byte, msgID string) error
}

// Publisher buffers EntityPayload messages and drains them to NATS at a
// controlled rate with circuit breaker backoff.
type Publisher struct {
	client          NATSPublisher
	logger          *slog.Logger
	buf             buffer.Buffer[*graph.EntityPayload]
	sendTimeout     time.Duration
	retryBackoff    time.Duration
	maxRetryBackoff time.Duration
	reconciler      Reconciler

	// Metrics
	accepted  atomic.Int64
	published atomic.Int64
	dropped   atomic.Int64
	failed    atomic.Int64
	retries   atomic.Int64
	// reconciled and reconcileFailed count the group reconcile that follows a
	// stream publish when WithReconcile is set; neither moves the delivery
	// counters above.
	reconciled      atomic.Int64
	reconcileFailed atomic.Int64
	// reconcilePending counts queued reconciles; they are in flight for
	// WaitSettled. latest maps an entity ID to the envelope time of its most
	// recently queued reconcile, so an older queued job knows it is superseded.
	reconcilePending atomic.Int64
	reconcileQ       chan reconcileJob
	reconcileDone    chan struct{}
	latestMu         sync.Mutex
	latest           map[string]uint64
	reconcileSeq     uint64 // guarded by latestMu
	metrics         *pubMetrics

	// backpressure tracks whether the publisher is CURRENTLY in sustained
	// backpressure, so the condition is logged on the transition into and out of
	// it rather than on every retry. Logging per attempt is what forced this
	// signal down to Debug in the first place, where it became invisible.
	backpressure atomic.Bool

	// publishFailing is edge-triggered like backpressure but for TERMINAL
	// failures: one default-level entry when publishes start failing, one
	// recovery line on the next success. The stream-ceiling incident produced
	// 34,871 per-entity WARN lines; a flood is one transition, not N lines —
	// exact counts live on the failed counter and metrics.
	publishFailing degraded.Condition

	// settledMu guards settledCh, the channel WaitSettled blocks on. It is
	// closed on every transition to zero entities in flight (#284).
	settledMu sync.Mutex
	settledCh chan struct{}

	// Lifecycle
	cancel   context.CancelFunc
	done     chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
	started  bool
}

// Option configures a Publisher.
type Option func(*publisherConfig)

type publisherConfig struct {
	capacity        int
	batchSize       int
	drainInterval   time.Duration
	sendTimeout     time.Duration
	retryBackoff    time.Duration
	maxRetryBackoff time.Duration
	metricsRegistry *metric.MetricsRegistry
	metricsPrefix   string
	reconciler      Reconciler
}

// WithCapacity sets the buffer capacity (default 5000).
func WithCapacity(n int) Option {
	return func(c *publisherConfig) { c.capacity = n }
}

// WithBatchSize sets the max entities drained per cycle (default 50).
func WithBatchSize(n int) Option {
	return func(c *publisherConfig) { c.batchSize = n }
}

// WithDrainInterval sets the drain loop ticker (default 5ms).
func WithDrainInterval(d time.Duration) Option {
	return func(c *publisherConfig) { c.drainInterval = d }
}

// WithSendTimeout bounds Send's backpressure wait when the buffer is full
// (default 5s). After the timeout the entity is dropped loudly.
func WithSendTimeout(d time.Duration) Option {
	return func(c *publisherConfig) { c.sendTimeout = d }
}

// WithRetryBackoff sets the initial and maximum circuit-breaker retry backoff
// (defaults 500ms / 10s). Non-positive values keep the defaults. Tests use
// short backoffs so retry sequences complete in milliseconds instead of
// real-clock seconds.
func WithRetryBackoff(initial, maximum time.Duration) Option {
	return func(c *publisherConfig) {
		if initial > 0 {
			c.retryBackoff = initial
		}
		if maximum > 0 {
			c.maxRetryBackoff = maximum
		}
	}
}

// WithMetrics enables metrics export for this publisher: the buffer's own
// write/read/overflow series plus the publish-boundary counters in metrics.go.
// prefix identifies the source instance and becomes the metric subsystem, so one
// stalled source cannot hide behind its healthy siblings.
func WithMetrics(registry *metric.MetricsRegistry, prefix string) Option {
	return func(c *publisherConfig) {
		c.metricsRegistry = registry
		c.metricsPrefix = prefix
	}
}

// New creates a Publisher with the given NATS client and options.
func New(client NATSPublisher, logger *slog.Logger, opts ...Option) (*Publisher, error) {
	cfg := publisherConfig{
		capacity:        defaultCapacity,
		batchSize:       defaultBatchSize,
		drainInterval:   defaultDrainInterval,
		sendTimeout:     defaultSendTimeout,
		retryBackoff:    circuitOpenBackoff,
		maxRetryBackoff: maxCircuitOpenBackoff,
	}
	for _, o := range opts {
		o(&cfg)
	}

	// Block (bounded by Send's timeout) instead of DropOldest: DropOldest made
	// buffer overflow invisible — Write always succeeded while the buffer
	// silently discarded the oldest entity, so the dropped counter could never
	// increment (audit 2026-07-19, no-silent-entity-loss).
	var bufOpts []buffer.Option[*graph.EntityPayload]
	bufOpts = append(bufOpts, buffer.WithOverflowPolicy[*graph.EntityPayload](buffer.Block))
	if cfg.metricsRegistry != nil && cfg.metricsPrefix != "" {
		bufOpts = append(bufOpts, buffer.WithMetrics[*graph.EntityPayload](cfg.metricsRegistry, cfg.metricsPrefix))
	}

	buf, err := buffer.NewCircularBuffer[*graph.EntityPayload](cfg.capacity, bufOpts...)
	if err != nil {
		return nil, fmt.Errorf("entitypub: create buffer: %w", err)
	}

	return &Publisher{
		client:          client,
		logger:          logger,
		buf:             buf,
		sendTimeout:     cfg.sendTimeout,
		retryBackoff:    cfg.retryBackoff,
		maxRetryBackoff: cfg.maxRetryBackoff,
		reconciler:      cfg.reconciler,
		metrics:         newPubMetrics(cfg.metricsRegistry, cfg.metricsPrefix),
		done:            make(chan struct{}),
		stop:            make(chan struct{}),
		reconcileQ:      make(chan reconcileJob, reconcileQueueSize),
		reconcileDone:   make(chan struct{}),
		latest:          map[string]uint64{},
	}, nil
}

// Start begins the background drain loop. Call Stop to shut down.
// Continuing drain work runs on a context detached from ctx's cancellation
// (ADR-113: Start's ctx bounds Start; Stop owns shutdown and settlement).
func (p *Publisher) Start(ctx context.Context) {
	drainCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	p.cancel = cancel
	p.started = true
	go p.reconcileWorker(drainCtx)
	go p.drainLoop(drainCtx)
}

// Stop seals producer admission and settles accepted work. The caller's
// deadline bounds settlement; expiration cancels delivery and returns an error.
// Start and Stop are owned by the component lifecycle and must not overlap.
func (p *Publisher) Stop(ctx context.Context) error {
	_ = p.buf.Close()
	if !p.started {
		return nil
	}
	p.stopOnce.Do(func() { close(p.stop) })
	select {
	case <-p.done:
		// The drain loop closed the reconcile queue on exit; the worker
		// discards what is left once Stop has begun.
		<-p.reconcileDone
		if lost := p.Lost(); lost > 0 {
			return fmt.Errorf("entitypub stopped with %d undelivered entities", lost)
		}
		return nil
	case <-ctx.Done():
		p.cancel()
		return fmt.Errorf("entitypub settlement incomplete: %w", ctx.Err())
	}
}

// timeoutWriter is the optional bounded-blocking write surface of the
// underlying buffer (implemented by the circular buffer under the Block
// policy). Kept as a local interface because buffer.Buffer does not expose it.
type timeoutWriter interface {
	WriteWithTimeout(item *graph.EntityPayload, timeout time.Duration) error
}

// Send snapshots and enqueues an entity for publishing. The producer may reuse
// its payload after Send returns. When the buffer is full it applies
// bounded backpressure (blocking up to the configured send timeout); if the
// buffer stays full past the timeout the entity is dropped LOUDLY: the drop
// counter increments, a WARN names the entity, and the error is returned so
// the caller can attribute the loss in its source status.
func (p *Publisher) Send(payload *graph.EntityPayload) error {
	owned, err := snapshotPayload(payload)
	if err != nil {
		p.dropped.Add(1)
		p.metrics.incDropped()
		p.logger.Warn("entitypub: dropping unrepresentable entity", "error", err)
		return fmt.Errorf("entitypub: cannot own queued payload: %w", err)
	}
	// Admission is counted before the write: the drain loop can publish the
	// entity before Write returns, and an outcome counted ahead of its
	// admission would read as nothing in flight.
	p.accepted.Add(1)
	if tw, ok := p.buf.(timeoutWriter); ok {
		err = tw.WriteWithTimeout(owned, p.sendTimeout)
	} else {
		err = p.buf.Write(owned)
	}
	if err != nil {
		p.accepted.Add(-1)
		p.noteOutcome()
		p.dropped.Add(1)
		p.metrics.incDropped()
		p.logger.Warn("entitypub: dropping entity after bounded backpressure",
			"id", payload.ID,
			"timeout", p.sendTimeout,
			"error", err)
		return fmt.Errorf("entitypub: buffer full beyond %s, dropped %s: %w",
			p.sendTimeout, payload.ID, err)
	}
	return nil
}

// Published returns the total number of successfully published entities.
func (p *Publisher) Published() int64 { return p.published.Load() }

// Dropped returns entities rejected before enqueue (invalid encoding or overflow).
func (p *Publisher) Dropped() int64 { return p.dropped.Load() }

// Failed returns the number of entities whose NATS publish failed terminally
// after retries (they left the buffer but never reached the stream).
func (p *Publisher) Failed() int64 { return p.failed.Load() }

// Lost returns entities rejected at admission or lost to terminal publish failures.
func (p *Publisher) Lost() int64 { return p.dropped.Load() + p.failed.Load() }

// Retries returns the number of circuit-breaker backoff retries.
func (p *Publisher) Retries() int64 { return p.retries.Load() }

// inFlight is the number of accepted entities with no terminal outcome yet:
// buffered, or read by the drain loop and not yet published or failed.
// Outcomes are read before admissions: a Send racing this read can only make
// the result too high (wait longer), never a false zero.
func (p *Publisher) inFlight() int64 {
	settled := p.published.Load() + p.failed.Load()
	return p.accepted.Load() - settled + p.reconcilePending.Load()
}

// noteOutcome wakes WaitSettled callers when the last in-flight entity
// settles. The caller has already updated its counter, so whichever update
// makes inFlight zero observes it here.
func (p *Publisher) noteOutcome() {
	if p.inFlight() != 0 {
		return
	}
	p.settledMu.Lock()
	if p.settledCh != nil {
		close(p.settledCh)
		p.settledCh = nil
	}
	p.settledMu.Unlock()
}

// WaitSettled blocks until every entity accepted by Send has been published
// or has failed terminally, or until ctx ends. Pending alone cannot answer
// this: the drain loop removes a batch from the buffer before publishing it.
// A source calls it after its seed so the status it reports next carries the
// seed's delivered figures rather than a snapshot taken mid-drain.
func (p *Publisher) WaitSettled(ctx context.Context) error {
	for {
		p.settledMu.Lock()
		if p.inFlight() == 0 {
			p.settledMu.Unlock()
			return nil
		}
		if p.settledCh == nil {
			p.settledCh = make(chan struct{})
		}
		ch := p.settledCh
		p.settledMu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Pending returns the current number of entities waiting in the buffer.
func (p *Publisher) Pending() int { return p.buf.Size() }

// Stats returns the underlying buffer statistics.
func (p *Publisher) Stats() *buffer.Statistics { return p.buf.Stats() }

// InBackpressure reports whether the publisher is currently retrying against a
// transport that is refusing writes. Surfaced in source status so "stalled" is
// answerable without reading logs.
func (p *Publisher) InBackpressure() bool { return p.backpressure.Load() }

// enterBackpressure marks the condition active, logging WARN only on the
// transition so a sustained stall costs one line rather than one per attempt.
func (p *Publisher) enterBackpressure(entityID string, backoff time.Duration) {
	if p.backpressure.Swap(true) {
		return // already reported
	}
	p.metrics.setBackpressure(true)
	p.logger.Warn("entitypub: publish backpressure — transport refusing writes, retrying",
		"first_entity", entityID,
		"backoff", backoff,
		"note", "entities are not lost while retrying; watch publish_retries_total vs entities_published_total")
}

// clearBackpressure reports recovery, once, on the transition back to healthy.
func (p *Publisher) clearBackpressure() {
	if !p.backpressure.Swap(false) {
		return // was not in backpressure
	}
	p.metrics.setBackpressure(false)
	p.logger.Info("entitypub: publish backpressure cleared",
		"retries_total", p.retries.Load(),
		"published_total", p.published.Load())
}

// drainLoop reads entities from the buffer and publishes them to NATS.
// On circuit breaker errors it backs off before retrying.
func (p *Publisher) drainLoop(ctx context.Context) {
	defer close(p.done)
	// Only this loop enqueues reconciles, so it closes their queue.
	defer close(p.reconcileQ)

	ticker := time.NewTicker(defaultDrainInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Flush remaining buffered entities before exiting.
			p.flush(ctx)
			return
		case <-p.stop:
			p.flush(ctx)
			return
		case <-ticker.C:
			p.drainBatch(ctx)
		}
	}
}

// drainBatch reads up to batchSize entities and publishes them.
func (p *Publisher) drainBatch(ctx context.Context) {
	batch := p.buf.ReadBatch(defaultBatchSize)
	if len(batch) == 0 {
		return
	}

	for _, payload := range batch {
		if err := p.publishOne(ctx, payload); err != nil {
			// ReadBatch has already transferred every entity to this publisher.
			// Cancellation is a terminal outcome for each remaining entity too;
			// returning here silently discarded the rest of the accepted batch.
			p.failed.Add(1)
			p.noteOutcome()
			p.metrics.incFailed()
			// Edge-triggered: the first failure names itself at the default
			// level, the flood behind it counts on the failed counter, and
			// per-entity detail stays at Debug (ADR-0011).
			p.publishFailing.Enter(p.logger,
				"entity publishes are failing — see entities_failed_total for the count",
				"first_entity", payload.ID, "error", err)
			p.logger.Debug("entity publish failed", "id", payload.ID, "error", err)
		}
	}
}

// publishOne marshals and publishes a single entity, retrying transient
// transport errors (classifyPublishError) with exponential backoff inside the
// delivery budget. Every attempt carries the same deterministic Nats-Msg-Id
// (entity ID + content hash) so an ambiguous timeout retried is deduped
// server-side instead of double-applying triples downstream.
func (p *Publisher) publishOne(ctx context.Context, payload *graph.EntityPayload) error {
	// The envelope time is fixed here, at the millisecond precision the
	// envelope is encoded with, so the reconcile that follows stamps the same
	// time graph-ingest gives the stream statements.
	at := time.Now().Truncate(time.Millisecond)
	msg := message.NewBaseMessage(graph.EntityType, payload, envelopeSource, message.WithTime(at))
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal entity message (terminal, not retried): %w", err)
	}
	sum := sha256.Sum256(data)
	msgID := payload.ID + ":" + hex.EncodeToString(sum[:6])

	backoff := p.retryBackoff
	maxBackoff := p.maxRetryBackoff
	deadline := time.Now().Add(deliveryBudget)

	var attempts int
	for attempts = 1; attempts <= maxPublishAttempts; attempts++ {
		err = p.client.PublishToStreamWithMsgID(ctx, graphIngestSubject, data, msgID)
		if err == nil {
			// Queue the reconcile before the delivery counters move, so
			// WaitSettled also covers it. It runs off this loop.
			p.enqueueReconcile(payload, at)
			// Recovery state settles before the delivery counter moves: a
			// WaitSettled caller can return the moment published advances, and
			// its report must not pair full delivery with stale backpressure.
			p.clearBackpressure()
			p.publishFailing.Clear(p.logger, "entity publishing recovered")
			p.published.Add(1)
			p.noteOutcome()
			p.metrics.incPublished()
			return nil
		}

		switch classifyPublishError(ctx, err) {
		case publishCanceled:
			return err // shutdown drain; the caller's ctx guard owns it
		case publishTerminal:
			if attempts == 1 {
				return fmt.Errorf("terminal on first attempt (not retried): %w", err)
			}
			return fmt.Errorf("terminal after %d attempts: %w", attempts, err)
		case publishRetryable:
			// fall through to the budget check and backoff below
		}

		if attempts == maxPublishAttempts || time.Now().After(deadline) {
			break
		}

		p.retries.Add(1)
		p.metrics.incRetries()
		// Entering sustained backpressure is a state CHANGE an operator must see
		// at the default level: a publisher retrying every entity reports no
		// drops, no failures, and no errors while being functionally stalled.
		// Reported once on entry, not once per attempt — and now for EVERY
		// retryable class, not only the circuit breaker (the live induction
		// proved capacity refusals previously bypassed this entirely).
		p.enterBackpressure(payload.ID, backoff)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return fmt.Errorf("delivery budget exhausted after %d attempts over %s: %w",
		attempts, deliveryBudget, err)
}

// flush drains all remaining buffered entities. Used during shutdown.
func (p *Publisher) flush(ctx context.Context) {
	for {
		batch := p.buf.ReadBatch(defaultBatchSize)
		if len(batch) == 0 {
			return
		}
		for _, payload := range batch {
			if err := p.publishOne(ctx, payload); err != nil {
				p.failed.Add(1)
				p.noteOutcome()
				p.metrics.incFailed()
				// Shutdown flush keeps one summary-shaped line per entity at
				// Debug; the edge-triggered condition covers the transition.
				p.publishFailing.Enter(p.logger,
					"entity publishes are failing during flush",
					"first_entity", payload.ID, "error", err)
				p.logger.Debug("flush: entity publish failed",
					"id", payload.ID,
					"error", err)
			}
		}
	}
}
