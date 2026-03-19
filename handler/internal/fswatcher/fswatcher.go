// Package fswatcher provides a shared fsnotify-based file watcher for use by
// doc.Handler and ConfigHandler. It is adapted from the DocWatcher implementation
// in semspec's source-ingester package with these changes:
//   - WatchEvent/WatchOperation replaced with handler.ChangeEvent/ChangeOperation
//   - parser.ContentHash replaced with a local crypto/sha256 helper
//   - Renamed from DocWatcher to FSWatcher
//   - semspec schema struct tags removed
package fswatcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/fsnotify/fsnotify"
)

const (
	// eventChannelBuffer is the size of the watch event output channel.
	eventChannelBuffer = 500
)

// WatchConfig configures file watching behaviour for an FSWatcher.
type WatchConfig struct {
	// Enabled controls whether file watching is active.
	Enabled bool

	// DebounceDelay is how long to wait for more changes before processing.
	// Must be a valid Go duration string (e.g. "500ms"). Defaults to "500ms".
	DebounceDelay string

	// FileExtensions lists file extensions to watch (e.g. [".md", ".txt"]).
	// Extensions without a leading dot are accepted and normalized.
	FileExtensions []string

	// ExcludeDirs lists directory base names to skip (e.g. [".git", "vendor"]).
	ExcludeDirs []string
}

// GetDebounceDelay parses DebounceDelay and returns a time.Duration.
// Falls back to 500ms for empty or unparseable values.
func (c *WatchConfig) GetDebounceDelay() time.Duration {
	if c.DebounceDelay == "" {
		return 500 * time.Millisecond
	}
	d, err := time.ParseDuration(c.DebounceDelay)
	if err != nil {
		return 500 * time.Millisecond
	}
	return d
}

// WithCoalesceMs returns a copy of the WatchConfig with DebounceDelay overridden
// by the given millisecond value. If ms <= 0, the original config is returned unchanged.
func (c WatchConfig) WithCoalesceMs(ms int) WatchConfig {
	if ms > 0 {
		c.DebounceDelay = fmt.Sprintf("%dms", ms)
	}
	return c
}

// ContentHash returns the hex-encoded SHA-256 of content.
// Exported so callers can seed the initial hash cache before starting the watcher.
func ContentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// FSWatcher watches a directory tree for file changes and emits
// handler.ChangeEvent values. It is safe for concurrent use.
//
// Lifecycle: New → Start → (receive Events) → Stop.
type FSWatcher struct {
	config  WatchConfig
	root    string
	watcher *fsnotify.Watcher
	logger  *slog.Logger

	extensions map[string]bool // normalised extensions → true
	excludes   map[string]bool // directory base names to skip

	// Debouncing: accumulate file → latest op before flushing.
	pendingMu sync.Mutex
	pending   map[string]fsnotify.Op

	// Hash-based dedup: emit only when content actually changes.
	hashMu sync.RWMutex
	hashes map[string]string // abs path → sha256 hex

	events        chan handler.ChangeEvent
	droppedEvents atomic.Int64
}

// New creates an FSWatcher that watches root according to cfg.
// logger may be nil; slog.Default() is used in that case.
func New(cfg WatchConfig, root string) (*FSWatcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	extensions := make(map[string]bool)
	if len(cfg.FileExtensions) == 0 {
		extensions[".md"] = true
		extensions[".txt"] = true
	} else {
		for _, ext := range cfg.FileExtensions {
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			extensions[strings.ToLower(ext)] = true
		}
	}

	excludes := make(map[string]bool)
	if len(cfg.ExcludeDirs) == 0 {
		excludes[".git"] = true
		excludes["node_modules"] = true
		excludes["vendor"] = true
	} else {
		for _, d := range cfg.ExcludeDirs {
			excludes[d] = true
		}
	}

	return &FSWatcher{
		config:     cfg,
		root:       root,
		watcher:    fsw,
		logger:     slog.Default(),
		extensions: extensions,
		excludes:   excludes,
		pending:    make(map[string]fsnotify.Op),
		hashes:     make(map[string]string),
		events:     make(chan handler.ChangeEvent, eventChannelBuffer),
	}, nil
}

// Events returns the read-only channel of change events.
// The channel is closed when the watcher shuts down.
func (w *FSWatcher) Events() <-chan handler.ChangeEvent {
	return w.events
}

// Start adds recursive directory watches and begins processing fsnotify events.
// It returns immediately after launching the background goroutine.
// ctx cancellation shuts the watcher down gracefully.
func (w *FSWatcher) Start(ctx context.Context) error {
	if err := os.MkdirAll(w.root, 0755); err != nil {
		return err
	}
	if err := w.addWatchesRecursive(w.root); err != nil {
		return err
	}
	go w.processEvents(ctx)
	return nil
}

// Stop closes the underlying fsnotify watcher, which causes processEvents to
// exit and close the Events channel.
func (w *FSWatcher) Stop() error {
	return w.watcher.Close()
}

// SetHash seeds the hash cache for a file, used to prime dedup before starting.
// path should be the absolute file path.
func (w *FSWatcher) SetHash(path, hash string) {
	w.hashMu.Lock()
	defer w.hashMu.Unlock()
	w.hashes[path] = hash
}

// GetHash returns the recorded hash for a file, if any.
func (w *FSWatcher) GetHash(path string) (string, bool) {
	w.hashMu.RLock()
	defer w.hashMu.RUnlock()
	hash, ok := w.hashes[path]
	return hash, ok
}

// DroppedEvents returns the cumulative count of events dropped due to a full
// output channel.
func (w *FSWatcher) DroppedEvents() int64 {
	return w.droppedEvents.Load()
}

// addWatchesRecursive walks root and registers an fsnotify watch on every
// non-excluded directory.
func (w *FSWatcher) addWatchesRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if w.excludes[base] || (strings.HasPrefix(base, ".") && base != ".") {
			return filepath.SkipDir
		}
		if addErr := w.watcher.Add(path); addErr != nil {
			w.logger.Warn("FSWatcher: failed to watch directory",
				"path", path, "error", addErr)
		}
		return nil
	})
}

// processEvents is the main event loop. It runs in a dedicated goroutine until
// ctx is cancelled or the fsnotify watcher is closed.
func (w *FSWatcher) processEvents(ctx context.Context) {
	defer close(w.events)

	ticker := time.NewTicker(w.config.GetDebounceDelay())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleFSEvent(event)

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("FSWatcher: fsnotify error", "error", err)

		case <-ticker.C:
			w.flushPending(ctx)
		}
	}
}

// handleFSEvent inspects a raw fsnotify event. Matching files are accumulated
// in the pending map for debounced processing; new directories are registered.
func (w *FSWatcher) handleFSEvent(event fsnotify.Event) {
	path := event.Name
	ext := strings.ToLower(filepath.Ext(path))

	if !w.extensions[ext] {
		// Still handle directory creation so sub-trees get watched.
		if event.Has(fsnotify.Create) {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				w.handleNewDirectory(path)
			}
		}
		return
	}

	// Skip files inside excluded directories.
	relPath, _ := filepath.Rel(w.root, path)
	for dir := range w.excludes {
		if strings.Contains(relPath, dir+string(filepath.Separator)) {
			return
		}
	}

	w.pendingMu.Lock()
	w.pending[path] = event.Op
	w.pendingMu.Unlock()
}

// handleNewDirectory registers a watch on a freshly created directory,
// provided it is not excluded or hidden.
func (w *FSWatcher) handleNewDirectory(path string) {
	base := filepath.Base(path)
	if w.excludes[base] || strings.HasPrefix(base, ".") {
		return
	}
	if err := w.watcher.Add(path); err != nil {
		w.logger.Warn("FSWatcher: failed to watch new directory",
			"path", path, "error", err)
	}
}

// flushPending drains the pending map and emits ChangeEvent values. It
// computes the content hash to avoid emitting spurious events when the file
// content has not actually changed.
func (w *FSWatcher) flushPending(ctx context.Context) {
	w.pendingMu.Lock()
	if len(w.pending) == 0 {
		w.pendingMu.Unlock()
		return
	}
	toProcess := make(map[string]fsnotify.Op, len(w.pending))
	for k, v := range w.pending {
		toProcess[k] = v
	}
	w.pending = make(map[string]fsnotify.Op)
	w.pendingMu.Unlock()

	for path, op := range toProcess {
		select {
		case <-ctx.Done():
			return
		default:
		}
		w.processOne(path, op)
	}
}

// processOne resolves a single pending file change into a ChangeEvent and
// sends it on the output channel.
func (w *FSWatcher) processOne(path string, op fsnotify.Op) {
	if op.Has(fsnotify.Remove) || op.Has(fsnotify.Rename) {
		w.hashMu.Lock()
		delete(w.hashes, path)
		w.hashMu.Unlock()
		w.sendEvent(handler.ChangeEvent{
			Path:      path,
			Operation: handler.OperationDelete,
		})
		return
	}

	// Stat — file might have been removed between the event and now.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		w.sendEvent(handler.ChangeEvent{
			Path:      path,
			Operation: handler.OperationDelete,
		})
		return
	}

	content, err := os.ReadFile(path)
	if err != nil {
		w.logger.Warn("FSWatcher: failed to read file", "path", path, "error", err)
		return
	}

	newHash := ContentHash(content)
	oldHash, hadHash := w.GetHash(path)
	if hadHash && oldHash == newHash {
		// Content unchanged — suppress the event.
		return
	}
	w.SetHash(path, newHash)

	var op2 handler.ChangeOperation
	if op.Has(fsnotify.Create) || !hadHash {
		op2 = handler.OperationCreate
	} else {
		op2 = handler.OperationModify
	}

	w.sendEvent(handler.ChangeEvent{
		Path:      path,
		Operation: op2,
		// Entities are populated by the handler layer (doc.Handler/ConfigHandler)
		// after reading and parsing the file content.
	})
}

// sendEvent delivers an event to the output channel without blocking.
// If the channel is full the event is counted as dropped and logged.
func (w *FSWatcher) sendEvent(ev handler.ChangeEvent) {
	select {
	case w.events <- ev:
	default:
		dropped := w.droppedEvents.Add(1)
		w.logger.Warn("FSWatcher: event channel full, dropping event",
			"path", ev.Path,
			"total_dropped", dropped)
	}
}
