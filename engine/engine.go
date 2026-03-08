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
	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semstreams/federation"
)

// Normalizer converts raw handler output into normalized graph entities.
// The concrete implementation lives in the normalizer package; this interface
// allows the engine to be tested without the normalizer dependency.
type Normalizer interface {
	Normalize(raw handler.RawEntity) (*federation.Entity, error)
	NormalizeBatch(raws []handler.RawEntity) ([]*federation.Entity, error)
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
	store      *entityStore
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

	// pathIndex maps a source file path to the set of entity IDs produced from it.
	// Used by retractionIDsForPath to find entities to remove on a file delete event.
	pathIndexMu sync.Mutex
	pathIndex   map[string]map[string]struct{} // filePath → set of entityIDs

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
		store:             newEntityStore(),
		emitter:           emitter,
		logger:            logger,
		heartbeatInterval: 30 * time.Second,
		reseedInterval:    60 * time.Second,
		pathIndex:         make(map[string]map[string]struct{}),
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

	for _, event := range e.buildSeedEvents() {
		if err := e.emitter.Emit(ctx, event); err != nil {
			return fmt.Errorf("emit SEED: %w", err)
		}
	}
	e.touchLastEmit()
	return nil
}

// normalizeAll converts raw entities using the normalizer if present,
// or creates passthrough graph entities in scaffold mode.
func (e *Engine) normalizeAll(raws []handler.RawEntity) ([]*federation.Entity, error) {
	if e.normalizer != nil {
		return e.normalizer.NormalizeBatch(raws)
	}
	// Scaffold passthrough: construct minimal entities without full ID normalization.
	entities := make([]*federation.Entity, 0, len(raws))
	for i := range raws {
		raw := &raws[i]
		entity := &federation.Entity{
			ID: fmt.Sprintf("%s.semsource.%s.%s.%s.%s", e.cfg.Namespace, raw.Domain, raw.System, raw.EntityType, raw.Instance),
			Provenance: federation.Provenance{
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
		var changed []*federation.Entity
		for _, entity := range entities {
			if e.store.Upsert(entity) {
				changed = append(changed, entity)
			}
		}
		// Record the path → entity ID mapping regardless of whether anything
		// changed — a subsequent delete needs to know which IDs to retract.
		if ev.Path != "" && len(entities) > 0 {
			e.indexPath(ev.Path, entities)
		}
		if len(changed) > 0 {
			for _, event := range e.buildDeltaEvents(changed) {
				if err := e.emitter.Emit(ctx, event); err != nil {
					e.logger.Error("emit DELTA failed", "error", err)
				}
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
			for _, event := range e.buildSeedEvents() {
				if err := e.emitter.Emit(ctx, event); err != nil {
					e.logger.Error("emit periodic SEED failed", "error", err)
				}
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

// retractionIDsForPath returns the IDs of all entities that were produced from
// the given file path, using the path index maintained during handleChange.
// Falls back to a full store scan matching Provenance.SourceID for entities
// that entered the store via the initial seed (which does not have path info).
func (e *Engine) retractionIDsForPath(path string) []string {
	e.pathIndexMu.Lock()
	idSet, ok := e.pathIndex[path]
	if ok {
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		delete(e.pathIndex, path)
		e.pathIndexMu.Unlock()
		return ids
	}
	e.pathIndexMu.Unlock()

	// Fallback: scan store for entities whose SourceID matches the path.
	all := e.store.Snapshot()
	var ids []string
	for _, entity := range all {
		if entity.Provenance.SourceID == path {
			ids = append(ids, entity.ID)
		}
	}
	return ids
}

// indexPath records the association between a file path and the entity IDs
// produced from it. Called after a successful create/modify change event.
func (e *Engine) indexPath(path string, entities []*federation.Entity) {
	e.pathIndexMu.Lock()
	defer e.pathIndexMu.Unlock()
	idSet := make(map[string]struct{}, len(entities))
	for _, ent := range entities {
		idSet[ent.ID] = struct{}{}
	}
	e.pathIndex[path] = idSet
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

func (a sourceConfigAdapter) GetBranch() string   { return a.entry.Branch }
func (a sourceConfigAdapter) GetPaths() []string { return a.entry.Paths }

func (a sourceConfigAdapter) IsWatchEnabled() bool { return a.entry.Watch }

func (a sourceConfigAdapter) GetKeyframeMode() string      { return a.entry.KeyframeMode }
func (a sourceConfigAdapter) GetKeyframeInterval() string   { return a.entry.KeyframeInterval }
func (a sourceConfigAdapter) GetSceneThreshold() float64    { return a.entry.SceneThreshold }

// GetLanguage implements the optional ASTConfig interface consumed by the AST handler.
func (a sourceConfigAdapter) GetLanguage() string { return a.entry.Language }

// GetOrg and GetProject are required by ASTConfig. Return empty strings to let
// the AST handler apply its own defaults.
func (a sourceConfigAdapter) GetOrg() string     { return "" }
func (a sourceConfigAdapter) GetProject() string { return "" }
