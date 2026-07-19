// Package entitypub provides a buffered entity publisher for semsource source
// components. It decouples entity ingestion from NATS publishing using a
// circular buffer, providing backpressure handling when the NATS circuit
// breaker trips.
//
// Usage:
//
//	pub := entitypub.New(natsClient, logger)
//	pub.Start(ctx)
//	defer pub.Stop()
//	pub.Send(payload) // non-blocking, buffered
package entitypub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/pkg/buffer"

	"github.com/c360studio/semsource/graph"
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

	// defaultSendTimeout bounds how long Send applies backpressure when the
	// buffer is full before dropping loudly. Bounded so a wedged NATS cannot
	// silently freeze ingest loops forever; loud so a drop is never invisible.
	defaultSendTimeout = 5 * time.Second
)

// NATSPublisher is the subset of natsclient.Client needed for publishing.
type NATSPublisher interface {
	PublishToStream(ctx context.Context, subject string, data []byte) error
}

// Publisher buffers EntityPayload messages and drains them to NATS at a
// controlled rate with circuit breaker backoff.
type Publisher struct {
	client      NATSPublisher
	logger      *slog.Logger
	buf         buffer.Buffer[*graph.EntityPayload]
	sendTimeout time.Duration

	// Metrics
	published atomic.Int64
	dropped   atomic.Int64
	failed    atomic.Int64
	retries   atomic.Int64

	// Lifecycle
	cancel context.CancelFunc
	done   chan struct{}
}

// Option configures a Publisher.
type Option func(*publisherConfig)

type publisherConfig struct {
	capacity        int
	batchSize       int
	drainInterval   time.Duration
	sendTimeout     time.Duration
	metricsRegistry *metric.MetricsRegistry
	metricsPrefix   string
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

// WithMetrics enables buffer metrics export.
func WithMetrics(registry *metric.MetricsRegistry, prefix string) Option {
	return func(c *publisherConfig) {
		c.metricsRegistry = registry
		c.metricsPrefix = prefix
	}
}

// New creates a Publisher with the given NATS client and options.
func New(client NATSPublisher, logger *slog.Logger, opts ...Option) (*Publisher, error) {
	cfg := publisherConfig{
		capacity:      defaultCapacity,
		batchSize:     defaultBatchSize,
		drainInterval: defaultDrainInterval,
		sendTimeout:   defaultSendTimeout,
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
		client:      client,
		logger:      logger,
		buf:         buf,
		sendTimeout: cfg.sendTimeout,
		done:        make(chan struct{}),
	}, nil
}

// Start begins the background drain loop. Call Stop to shut down.
func (p *Publisher) Start(ctx context.Context) {
	drainCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	go p.drainLoop(drainCtx)
}

// Stop signals the drain loop to exit and waits for it to flush.
func (p *Publisher) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	<-p.done
	p.buf.Close()
}

// timeoutWriter is the optional bounded-blocking write surface of the
// underlying buffer (implemented by the circular buffer under the Block
// policy). Kept as a local interface because buffer.Buffer does not expose it.
type timeoutWriter interface {
	WriteWithTimeout(item *graph.EntityPayload, timeout time.Duration) error
}

// Send enqueues an entity for publishing. When the buffer is full it applies
// bounded backpressure (blocking up to the configured send timeout); if the
// buffer stays full past the timeout the entity is dropped LOUDLY: the drop
// counter increments, a WARN names the entity, and the error is returned so
// the caller can attribute the loss in its source status.
func (p *Publisher) Send(payload *graph.EntityPayload) error {
	var err error
	if tw, ok := p.buf.(timeoutWriter); ok {
		err = tw.WriteWithTimeout(payload, p.sendTimeout)
	} else {
		err = p.buf.Write(payload)
	}
	if err != nil {
		p.dropped.Add(1)
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

// Dropped returns the total number of dropped entities (buffer overflow).
func (p *Publisher) Dropped() int64 { return p.dropped.Load() }

// Failed returns the number of entities whose NATS publish failed terminally
// after retries (they left the buffer but never reached the stream).
func (p *Publisher) Failed() int64 { return p.failed.Load() }

// Lost returns the total entities accepted by Send that never reached the
// stream (dropped on overflow + terminal publish failures). Zero means every
// accepted entity was delivered.
func (p *Publisher) Lost() int64 { return p.dropped.Load() + p.failed.Load() }

// Retries returns the number of circuit-breaker backoff retries.
func (p *Publisher) Retries() int64 { return p.retries.Load() }

// Pending returns the current number of entities waiting in the buffer.
func (p *Publisher) Pending() int { return p.buf.Size() }

// Stats returns the underlying buffer statistics.
func (p *Publisher) Stats() *buffer.Statistics { return p.buf.Stats() }

// drainLoop reads entities from the buffer and publishes them to NATS.
// On circuit breaker errors it backs off before retrying.
func (p *Publisher) drainLoop(ctx context.Context) {
	defer close(p.done)

	ticker := time.NewTicker(defaultDrainInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Flush remaining buffered entities before exiting.
			p.flush(context.Background())
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
			if ctx.Err() != nil {
				return
			}
			p.failed.Add(1)
			p.logger.Warn("entity publish failed after retries",
				"id", payload.ID,
				"error", err)
		}
	}
}

// publishOne marshals and publishes a single entity, retrying on circuit
// breaker errors with exponential backoff.
func (p *Publisher) publishOne(ctx context.Context, payload *graph.EntityPayload) error {
	msg := message.NewBaseMessage(graph.EntityType, payload, "semsource")
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal entity message: %w", err)
	}

	backoff := circuitOpenBackoff
	maxBackoff := 10 * time.Second
	maxAttempts := 20

	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = p.client.PublishToStream(ctx, graphIngestSubject, data)
		if err == nil {
			p.published.Add(1)
			return nil
		}

		// Only retry on circuit breaker — other errors are terminal.
		if err.Error() != "circuit breaker is open" {
			return err
		}

		p.retries.Add(1)
		if attempt == 0 {
			p.logger.Debug("circuit breaker open, backing off",
				"entity", payload.ID,
				"backoff", backoff)
		}

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

	return fmt.Errorf("circuit breaker did not recover after %d attempts", maxAttempts)
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
				p.logger.Warn("flush: entity publish failed",
					"id", payload.ID,
					"error", err)
			}
		}
	}
}
