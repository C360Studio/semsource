package image

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/handler/internal/fswatcher"
)

// Watch starts one FSWatcher per root path in cfg and fans all change events
// into a single output channel. Each event is enriched using the root that
// owns the changed file so relative paths and entity IDs remain correct.
//
// Returns (nil, nil) when cfg.IsWatchEnabled() is false — callers must check.
func (h *Handler) Watch(ctx context.Context, cfg handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	if !cfg.IsWatchEnabled() {
		return nil, nil
	}

	roots, err := resolvePaths(cfg)
	if err != nil {
		return nil, fmt.Errorf("image handler: Watch: %w", err)
	}

	wcfg := fswatcher.WatchConfig{
		Enabled:        true,
		DebounceDelay:  "200ms",
		FileExtensions: []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"},
		ExcludeDirs:    []string{".git", "node_modules", "vendor"},
	}
	if cp, ok := cfg.(handler.CoalesceProvider); ok {
		wcfg = wcfg.WithCoalesceMs(cp.GetCoalesceMs())
	}

	out := make(chan handler.ChangeEvent, 64)

	// Create and start all watchers up-front so we can roll back cleanly on
	// any error before launching goroutines.
	type entry struct {
		w    *fswatcher.FSWatcher
		root string
	}
	started := make([]entry, 0, len(roots))

	for _, root := range roots {
		w, err := fswatcher.New(wcfg, root)
		if err != nil {
			for _, e := range started {
				_ = e.w.Stop()
			}
			return nil, fmt.Errorf("image handler: create watcher for %q: %w", root, err)
		}
		if err := w.Start(ctx); err != nil {
			_ = w.Stop()
			for _, e := range started {
				_ = e.w.Stop()
			}
			return nil, fmt.Errorf("image handler: start watcher for %q: %w", root, err)
		}
		started = append(started, entry{w: w, root: root})
	}

	// Fan-in: one goroutine per watcher writes to out; a WaitGroup closer
	// closes out exactly once after all goroutines exit.
	var wg sync.WaitGroup
	for _, e := range started {
		wg.Add(1)
		e := e // capture for goroutine
		go func() {
			defer wg.Done()
			defer func() { _ = e.w.Stop() }()
			for {
				select {
				case <-ctx.Done():
					return
				case ev, ok := <-e.w.Events():
					if !ok {
						return
					}
					enriched := h.enrichEvent(ctx, ev, e.root)
					select {
					case out <- enriched:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

// enrichEvent re-reads the changed file and populates ev.Entities.
// When h.org is set it also populates ev.EntityStates for the normalizer-free
// processor path. For delete events the file is gone, so both slices remain empty.
func (h *Handler) enrichEvent(ctx context.Context, ev handler.ChangeEvent, root string) handler.ChangeEvent {
	if ev.Operation == handler.OperationDelete {
		ev.Timestamp = time.Now()
		return ev
	}

	// Check file is readable before calling ingestFile (which reads it again).
	if _, err := os.Stat(ev.Path); os.IsNotExist(err) {
		ev.Operation = handler.OperationDelete
		ev.Timestamp = time.Now()
		return ev
	}

	entity, err := h.ingestFile(ctx, ev.Path, root)
	if err == nil {
		ev.Entities = []handler.RawEntity{entity}
		if h.org != "" {
			ie := imageEntityFromRaw(h.org, h.storeBucket, entity, time.Now().UTC())
			ev.EntityStates = []*handler.EntityState{ie.EntityState()}
		}
	}
	ev.Timestamp = time.Now()
	return ev
}
