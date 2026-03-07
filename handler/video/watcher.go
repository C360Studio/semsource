package video

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/handler/internal/fswatcher"
)

// Watch starts an FSWatcher on the path in cfg and returns a channel of
// ChangeEvent values. Each event includes re-processed entities for the
// changed file.
//
// Returns (nil, nil) when cfg.IsWatchEnabled() is false — callers must check.
func (h *VideoHandler) Watch(ctx context.Context, cfg handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	if !cfg.IsWatchEnabled() {
		return nil, nil
	}

	root := cfg.GetPath()
	if root == "" {
		return nil, fmt.Errorf("video handler: Watch requires a non-empty path")
	}

	wcfg := fswatcher.WatchConfig{
		Enabled:        true,
		DebounceDelay:  "200ms",
		FileExtensions: []string{".mp4", ".webm", ".mov", ".avi", ".mkv"},
		ExcludeDirs:    []string{".git", "node_modules", "vendor"},
	}

	w, err := fswatcher.New(wcfg, root)
	if err != nil {
		return nil, fmt.Errorf("video handler: create watcher: %w", err)
	}

	if err := w.Start(ctx); err != nil {
		return nil, fmt.Errorf("video handler: start watcher: %w", err)
	}

	out := make(chan handler.ChangeEvent, 64)

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				_ = w.Stop()
				return

			case ev, ok := <-w.Events():
				if !ok {
					return
				}
				enriched := h.enrichEvent(ctx, ev, root, cfg)
				select {
				case out <- enriched:
				case <-ctx.Done():
					_ = w.Stop()
					return
				}
			}
		}
	}()

	return out, nil
}

// enrichEvent re-processes the changed file and populates ev.Entities.
// For delete events the file is gone, so Entities remains empty.
func (h *VideoHandler) enrichEvent(ctx context.Context, ev handler.ChangeEvent, root string, cfg handler.SourceConfig) handler.ChangeEvent {
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
	}
	ev.Timestamp = time.Now()
	return ev
}
