// Package engine implements the SemSource core event loop.
//
// The engine coordinates source handlers, normalization, the graph store,
// and event emission per the spec Section 3.6 event loop pseudocode:
//
//   - On start: ingest all sources, emit SEED, start watchers and timers.
//   - On file/source change: re-ingest, emit DELTA or RETRACT.
//   - On quiet period: emit HEARTBEAT.
//   - On re-seed interval: emit full SEED snapshot.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
)

// Normalizer converts raw handler output into normalized graph entities.
// The concrete implementation lives in the normalizer package; this interface
// allows the engine to be tested without the normalizer dependency.
type Normalizer interface {
	Normalize(raw handler.RawEntity) (*graph.GraphEntity, error)
	NormalizeBatch(raws []handler.RawEntity) ([]*graph.GraphEntity, error)
}

// Option configures Engine behaviour.
type Option func(*Engine)

// WithHeartbeatInterval sets the quiet-period heartbeat emission interval.
func WithHeartbeatInterval(d time.Duration) Option {
	return func(e *Engine) { e.heartbeatInterval = d }
}

// WithReseedInterval sets the periodic full-SEED emission interval.
func WithReseedInterval(d time.Duration) Option {
	return func(e *Engine) { e.reseedInterval = d }
}

// WithNormalizer wires a Normalizer into the engine.
// When not set, raw entities are passed through without ID normalization
// (scaffold mode — useful for testing without a normalizer dependency).
func WithNormalizer(n Normalizer) Option {
	return func(e *Engine) { e.normalizer = n }
}

// Engine coordinates the SemSource ingestion pipeline.
type Engine struct {
	cfg        *config.Config
	handlers   []handler.SourceHandler
	normalizer Normalizer // nil = passthrough (scaffold mode)
	store      *graph.Store
	emitter    Emitter
	logger     *slog.Logger

	// heartbeatInterval controls how often HEARTBEAT events are emitted during quiet periods.
	heartbeatInterval time.Duration

	// reseedInterval controls how often a full SEED snapshot is re-emitted.
	reseedInterval time.Duration

	// lastEmit tracks the timestamp of the most recent event emission
	// for quiet-period detection.
	lastEmitMu sync.Mutex
	lastEmit   time.Time

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	stopped bool
	stopMu  sync.Mutex
}

// NewEngine creates a ready-to-start Engine. Call RegisterHandler to add source handlers
// before calling Start. Pass functional options to customize behaviour.
func NewEngine(cfg *config.Config, emitter Emitter, logger *slog.Logger, opts ...Option) *Engine {
	e := &Engine{
		cfg:               cfg,
		store:             graph.NewStore(),
		emitter:           emitter,
		logger:            logger,
		heartbeatInterval: 30 * time.Second,
		reseedInterval:    60 * time.Second,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// RegisterHandler adds a source handler to the engine's dispatch table.
// Must be called before Start.
func (e *Engine) RegisterHandler(h handler.SourceHandler) {
	e.handlers = append(e.handlers, h)
}

// Start runs the full ingestion pipeline: initial seed, watcher goroutines,
// heartbeat timer, and reseed timer. It is non-blocking — background goroutines
// run until Stop is called or ctx is cancelled.
func (e *Engine) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	// Initial ingest and SEED event.
	if err := e.seed(ctx); err != nil {
		cancel()
		return fmt.Errorf("engine: initial seed failed: %w", err)
	}

	// Start per-source watchers for sources with Watch=true.
	for i := range e.cfg.Sources {
		src := sourceConfigAdapter{entry: &e.cfg.Sources[i]}
		if !src.IsWatchEnabled() {
			continue
		}
		h := e.findHandler(src)
		if h == nil {
			e.logger.Warn("no handler found for source, skipping watch", "type", src.GetType())
			continue
		}
		ch, err := h.Watch(ctx, src)
		if err != nil {
			e.logger.Error("watch start failed", "type", src.GetType(), "error", err)
			continue
		}
		if ch == nil {
			continue
		}
		e.wg.Add(1)
		go e.runWatcher(ctx, ch)
	}

	// Heartbeat goroutine.
	e.wg.Add(1)
	go e.runHeartbeat(ctx)

	// Reseed goroutine.
	e.wg.Add(1)
	go e.runReseed(ctx)

	return nil
}

// Stop cancels the engine context and waits for all goroutines to exit.
// Safe to call multiple times.
func (e *Engine) Stop() error {
	e.stopMu.Lock()
	defer e.stopMu.Unlock()
	if e.stopped {
		return nil
	}
	e.stopped = true
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
	return nil
}

// sourceID returns a stable identifier for this engine instance.
func (e *Engine) sourceID() string {
	return "semsource"
}

// seed runs the initial ingest for all configured sources and emits a SEED event.
func (e *Engine) seed(ctx context.Context) error {
	for i := range e.cfg.Sources {
		src := sourceConfigAdapter{entry: &e.cfg.Sources[i]}
		h := e.findHandler(src)
		if h == nil {
			e.logger.Warn("no handler registered for source type, skipping", "type", src.GetType())
			continue
		}
		raws, err := h.Ingest(ctx, src)
		if err != nil {
			return fmt.Errorf("ingest %s: %w", src.GetType(), err)
		}
		entities, err := e.normalizeAll(raws)
		if err != nil {
			return fmt.Errorf("normalize %s: %w", src.GetType(), err)
		}
		for _, entity := range entities {
			e.store.Upsert(entity)
		}
	}

	event := e.buildSeedEvent()
	if err := e.emitter.Emit(ctx, event); err != nil {
		return fmt.Errorf("emit SEED: %w", err)
	}
	e.touchLastEmit()
	return nil
}

// normalizeAll converts raw entities using the normalizer if present,
// or creates passthrough graph entities in scaffold mode.
func (e *Engine) normalizeAll(raws []handler.RawEntity) ([]*graph.GraphEntity, error) {
	if e.normalizer != nil {
		return e.normalizer.NormalizeBatch(raws)
	}
	// Scaffold passthrough: construct minimal entities without full ID normalization.
	entities := make([]*graph.GraphEntity, 0, len(raws))
	for i := range raws {
		raw := &raws[i]
		entity := &graph.GraphEntity{
			ID:      fmt.Sprintf("%s.semsource.%s.%s.%s.%s", e.cfg.Namespace, raw.Domain, raw.System, raw.EntityType, raw.Instance),
			Triples: raw.Triples,
			Provenance: graph.SourceProvenance{
				SourceType: raw.SourceType,
				SourceID:   raw.System,
				Timestamp:  time.Now(),
				Handler:    raw.SourceType,
			},
		}
		entities = append(entities, entity)
	}
	return entities, nil
}

// runWatcher processes ChangeEvents from a handler watch channel.
func (e *Engine) runWatcher(ctx context.Context, ch <-chan handler.ChangeEvent) {
	defer e.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			e.handleChange(ctx, ev)
		}
	}
}

// handleChange processes a single ChangeEvent from a watcher.
func (e *Engine) handleChange(ctx context.Context, ev handler.ChangeEvent) {
	switch ev.Operation {
	case handler.OperationDelete:
		retractedIDs := e.retractionIDsForPath(ev.Path)
		for _, id := range retractedIDs {
			e.store.Remove(id)
		}
		if len(retractedIDs) > 0 {
			event := e.buildRetractEvent(retractedIDs)
			if err := e.emitter.Emit(ctx, event); err != nil {
				e.logger.Error("emit RETRACT failed", "error", err)
			}
			e.touchLastEmit()
		}

	case handler.OperationCreate, handler.OperationModify:
		entities, err := e.normalizeAll(ev.Entities)
		if err != nil {
			// NormalizeBatch is fail-fast: a single invalid entity aborts the
			// whole batch. The change event is skipped entirely — no partial
			// upsert or DELTA is emitted. The handler is responsible for
			// emitting only valid RawEntity values.
			e.logger.Error("normalize change event failed, skipping batch",
				"path", ev.Path,
				"entity_count", len(ev.Entities),
				"error", err,
			)
			return
		}
		var changed []*graph.GraphEntity
		for _, entity := range entities {
			if e.store.Upsert(entity) {
				changed = append(changed, entity)
			}
		}
		if len(changed) > 0 {
			event := e.buildDeltaEvent(changed)
			if err := e.emitter.Emit(ctx, event); err != nil {
				e.logger.Error("emit DELTA failed", "error", err)
			}
			e.touchLastEmit()
		}
	}
}

// runHeartbeat emits HEARTBEAT events during quiet periods.
func (e *Engine) runHeartbeat(ctx context.Context) {
	defer e.wg.Done()
	ticker := time.NewTicker(e.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.lastEmitMu.Lock()
			quiet := time.Since(e.lastEmit) >= e.heartbeatInterval
			e.lastEmitMu.Unlock()
			if quiet {
				event := e.buildHeartbeatEvent()
				if err := e.emitter.Emit(ctx, event); err != nil {
					e.logger.Error("emit HEARTBEAT failed", "error", err)
				}
			}
		}
	}
}

// runReseed periodically emits a full SEED snapshot.
func (e *Engine) runReseed(ctx context.Context) {
	defer e.wg.Done()
	ticker := time.NewTicker(e.reseedInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			event := e.buildSeedEvent()
			if err := e.emitter.Emit(ctx, event); err != nil {
				e.logger.Error("emit periodic SEED failed", "error", err)
			}
			e.touchLastEmit()
		}
	}
}

// findHandler returns the first registered handler that supports the given source,
// or nil if none match.
func (e *Engine) findHandler(src handler.SourceConfig) handler.SourceHandler {
	for _, h := range e.handlers {
		if h.Supports(src) {
			return h
		}
	}
	return nil
}

// touchLastEmit records the current time as the last event emission time.
func (e *Engine) touchLastEmit() {
	e.lastEmitMu.Lock()
	e.lastEmit = time.Now()
	e.lastEmitMu.Unlock()
}

// retractionIDsForPath returns the IDs of all stored entities whose provenance
// SourceID matches the deleted path.
func (e *Engine) retractionIDsForPath(path string) []string {
	all := e.store.Snapshot()
	var ids []string
	for _, entity := range all {
		if entity.Provenance.SourceID == path {
			ids = append(ids, entity.ID)
		}
	}
	return ids
}

// sourceConfigAdapter adapts config.SourceEntry to the handler.SourceConfig interface.
type sourceConfigAdapter struct {
	entry *config.SourceEntry
}

func (a sourceConfigAdapter) GetType() string { return a.entry.Type }

func (a sourceConfigAdapter) GetPath() string {
	if a.entry.Path != "" {
		return a.entry.Path
	}
	if len(a.entry.Paths) > 0 {
		return a.entry.Paths[0]
	}
	return ""
}

func (a sourceConfigAdapter) GetURL() string {
	if a.entry.URL != "" {
		return a.entry.URL
	}
	if len(a.entry.URLs) > 0 {
		return a.entry.URLs[0]
	}
	return ""
}

func (a sourceConfigAdapter) IsWatchEnabled() bool { return a.entry.Watch }
