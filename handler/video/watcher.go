package video

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/handler/internal/fswatcher"
)

// Watch starts an FSWatcher on every path in cfg and returns a single channel
// of ChangeEvent values. Each event includes re-processed entities for the
// changed file. Events from all watchers are fanned into the one output channel.
//
// Returns (nil, nil) when cfg.IsWatchEnabled() is false — callers must check.
func (h *Handler) Watch(ctx context.Context, cfg handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	if !cfg.IsWatchEnabled() {
		return nil, nil
	}

	roots := resolvePaths(cfg)
	if len(roots) == 0 {
		return nil, fmt.Errorf("video handler: Watch requires at least one configured path")
	}

	wcfg := fswatcher.WatchConfig{
		Enabled:        true,
		DebounceDelay:  "200ms",
		FileExtensions: []string{".mp4", ".webm", ".mov", ".avi", ".mkv"},
		ExcludeDirs:    []string{".git", "node_modules", "vendor"},
	}

	// Start one FSWatcher per root and collect them together with their root.
	type watcherEntry struct {
		w    *fswatcher.FSWatcher
		root string
	}
	entries := make([]watcherEntry, 0, len(roots))
	for _, root := range roots {
		w, err := fswatcher.New(wcfg, root)
		if err != nil {
			// Stop any already-started watchers before returning.
			for _, e := range entries {
				_ = e.w.Stop()
			}
			return nil, fmt.Errorf("video handler: create watcher for %q: %w", root, err)
		}
		if err := w.Start(ctx); err != nil {
			for _, e := range entries {
				_ = e.w.Stop()
			}
			_ = w.Stop()
			return nil, fmt.Errorf("video handler: start watcher for %q: %w", root, err)
		}
		entries = append(entries, watcherEntry{w: w, root: root})
	}

	// Fan-in: one goroutine per watcher pipes into the shared output channel.
	// The output channel is closed once all per-watcher goroutines have exited.
	out := make(chan handler.ChangeEvent, 64*len(entries))

	var wg sync.WaitGroup
	for _, entry := range entries {
		wg.Add(1)
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
					enriched := h.enrichEvent(ctx, ev, root, cfg)
					select {
					case out <- enriched:
					case <-ctx.Done():
						return
					}
				}
			}
		}(entry.w, entry.root)
	}

	// Close the output channel once every per-watcher goroutine has finished.
	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

// enrichEvent re-processes the changed file and populates ev.Entities.
// When h.org is set it also populates ev.EntityStates for the normalizer-free
// processor path. For delete events the file is gone, so both slices remain empty.
func (h *Handler) enrichEvent(ctx context.Context, ev handler.ChangeEvent, root string, cfg handler.SourceConfig) handler.ChangeEvent {
	if ev.Operation == handler.OperationDelete {
		ev.Timestamp = time.Now()
		return ev
	}

	// Check the file is still readable before processing.
	if _, err := os.Stat(ev.Path); os.IsNotExist(err) {
		ev.Operation = handler.OperationDelete
		ev.Timestamp = time.Now()
		return ev
	}

	videoEntity, keyframeEntities, err := h.ingestFile(ctx, ev.Path, root, cfg)
	if err == nil {
		ev.Entities = append([]handler.RawEntity{videoEntity}, keyframeEntities...)
		if h.org != "" {
			now := time.Now().UTC()
			ve := videoEntityFromRaw(h.org, videoEntity, now)
			states := []*handler.EntityState{ve.EntityState()}
			for _, kf := range keyframeEntities {
				ke := keyframeEntityFromRaw(h.org, ve.ID, kf, now)
				states = append(states, ke.EntityState())
			}
			ev.EntityStates = states
		}
	}
	ev.Timestamp = time.Now()
	return ev
}
