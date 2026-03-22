package doc

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/handler/internal/fswatcher"
)

// Watch starts one FSWatcher per configured path in cfg and returns a single
// channel of ChangeEvent values. Each event includes re-read entities for the
// changed file. All per-root watcher channels are fanned into the returned
// channel so callers see a unified stream.
//
// Returns (nil, nil) when cfg.IsWatchEnabled() is false — callers must check.
func (h *Handler) Watch(ctx context.Context, cfg handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	if !cfg.IsWatchEnabled() {
		return nil, nil
	}

	roots, err := resolvePaths(cfg)
	if err != nil {
		return nil, fmt.Errorf("doc handler: Watch: %w", err)
	}

	wcfg := fswatcher.WatchConfig{
		Enabled:        true,
		DebounceDelay:  "200ms",
		FileExtensions: []string{".adoc", ".md", ".mdx", ".txt"},
		ExcludeDirs:    []string{".git", "node_modules", "vendor"},
	}
	if cp, ok := cfg.(handler.CoalesceProvider); ok {
		wcfg = wcfg.WithCoalesceMs(cp.GetCoalesceMs())
	}

	// Start one watcher per root, collecting them so we can stop them all on
	// shutdown and fan their events into a single output channel.
	watchers := make([]*fswatcher.FSWatcher, 0, len(roots))
	for _, root := range roots {
		w, err := fswatcher.New(wcfg, root)
		if err != nil {
			// Stop any watchers already started before returning.
			for _, started := range watchers {
				_ = started.Stop()
			}
			return nil, fmt.Errorf("doc handler: create watcher for %q: %w", root, err)
		}
		if err := w.Start(ctx); err != nil {
			_ = w.Stop()
			for _, started := range watchers {
				_ = started.Stop()
			}
			return nil, fmt.Errorf("doc handler: start watcher for %q: %w", root, err)
		}
		watchers = append(watchers, w)
	}

	out := make(chan handler.ChangeEvent, 64*len(watchers))

	// Launch one fan-in goroutine per watcher. Each goroutine knows its root
	// so enrichEvent can compute the correct relative path. Each goroutine
	// stops its own watcher on exit so out is closed as soon as all streams end,
	// regardless of whether ctx was cancelled.
	var wg sync.WaitGroup
	for i, w := range watchers {
		wg.Add(1)
		root := roots[i]
		org := h.org
		go func(w *fswatcher.FSWatcher, root string) {
			defer wg.Done()
			defer func() { _ = w.Stop() }()
			for {
				select {
				case <-ctx.Done():
					return
				case ev, ok := <-w.Events():
					if !ok {
						return
					}
					enriched := h.enrichEvent(ctx, ev, root, org)
					select {
					case out <- enriched:
					case <-ctx.Done():
						return
					}
				}
			}
		}(w, root)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

// enrichEvent re-reads the changed file and populates ev.Entities and, when
// org is set, ev.EntityStates. For delete events the file is gone, so both
// slices remain empty.
//
// When org is set, only the EntityState path runs (single file read). The
// RawEntity path is only used as a fallback when org is empty.
func (h *Handler) enrichEvent(ctx context.Context, ev handler.ChangeEvent, root, org string) handler.ChangeEvent {
	if ev.Operation == handler.OperationDelete {
		ev.Timestamp = time.Now()
		return ev
	}

	// Check file still exists — may have been removed between fsnotify event
	// and processing.
	if _, err := os.Stat(ev.Path); os.IsNotExist(err) {
		ev.Operation = handler.OperationDelete
		ev.Timestamp = time.Now()
		return ev
	}

	if org != "" {
		// Typed EntityState path — single file read, includes store logic.
		ev = h.enrichEventEntityStates(ctx, ev, root, org)
	} else {
		// Legacy RawEntity fallback — no org means no EntityStates.
		entity, err := ingestFile(ev.Path, root)
		if err == nil {
			ev.Entities = []handler.RawEntity{entity}
		}
	}

	ev.Timestamp = time.Now()
	return ev
}
