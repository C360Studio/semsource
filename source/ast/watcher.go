package ast

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	// eventChannelBuffer is the size of the watch event channel.
	// Increased from 100 to handle bursts during large refactoring operations.
	eventChannelBuffer = 1000
)

// WatcherConfig configures the file watcher
type WatcherConfig struct {
	// RepoRoot is the root directory to watch
	RepoRoot string

	// Org is the organization for entity IDs
	Org string

	// Project is the project name for entity IDs
	Project string

	// DebounceDelay is how long to wait for more changes before processing
	DebounceDelay time.Duration

	// Logger for logging events
	Logger *slog.Logger

	// FileExtensions to watch (e.g., ".go", ".ts", ".tsx")
	// If empty, defaults to [".go"]
	FileExtensions []string

	// ExcludeDirs are directory names to skip (e.g., "vendor", "node_modules")
	// If empty, defaults to ["vendor"]
	ExcludeDirs []string
}

// WatchEvent represents a file change event
type WatchEvent struct {
	// Path is the file path relative to repo root
	Path string

	// Operation is the type of change
	Operation WatchOperation

	// Result is the parse result (nil for delete operations)
	Result *ParseResult

	// Error if parsing failed
	Error error
}

// WatchOperation indicates the type of file operation
type WatchOperation string

// OpCreate, OpModify, and OpDelete enumerate the types of file watch operation.
const (
	OpCreate WatchOperation = "create"
	OpModify WatchOperation = "modify"
	OpDelete WatchOperation = "delete"
)

// FileParser is the interface for language-specific parsers
type FileParser interface {
	ParseFile(ctx context.Context, filePath string) (*ParseResult, error)
}

// Watcher watches for source file changes and emits parse results
type Watcher struct {
	config     WatcherConfig
	parser     FileParser
	watcher    *fsnotify.Watcher
	logger     *slog.Logger
	extensions map[string]bool // set of watched extensions
	excludes   map[string]bool // set of excluded directory names

	// Debouncing: collect changes before processing
	pendingMu sync.Mutex
	pending   map[string]fsnotify.Op // path → most recent operation

	// State tracking for change detection
	hashMu sync.RWMutex
	hashes map[string]string // path → content hash

	// Output channel
	events chan WatchEvent

	// Metrics
	droppedEvents atomic.Int64
}

// NewWatcher creates a new file watcher
func NewWatcher(config WatcherConfig) (*Watcher, error) {
	return NewWatcherWithParser(config, nil)
}

// NewWatcherWithParser creates a new file watcher with a custom parser
func NewWatcherWithParser(config WatcherConfig, parser FileParser) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	debounce := config.DebounceDelay
	if debounce == 0 {
		debounce = 100 * time.Millisecond
	}

	// Build extension set
	extensions := make(map[string]bool)
	if len(config.FileExtensions) == 0 {
		extensions[".go"] = true
	} else {
		for _, ext := range config.FileExtensions {
			extensions[ext] = true
		}
	}

	// Build exclude set
	excludes := make(map[string]bool)
	if len(config.ExcludeDirs) == 0 {
		excludes["vendor"] = true
	} else {
		for _, dir := range config.ExcludeDirs {
			excludes[dir] = true
		}
	}

	// Default to Go parser if none provided (uses registry)
	if parser == nil {
		var err error
		parser, err = DefaultRegistry.CreateParser("go", config.Org, config.Project, config.RepoRoot)
		if err != nil {
			return nil, err
		}
	}

	return &Watcher{
		config:     config,
		parser:     parser,
		watcher:    fsw,
		logger:     logger,
		extensions: extensions,
		excludes:   excludes,
		pending:    make(map[string]fsnotify.Op),
		hashes:     make(map[string]string),
		events:     make(chan WatchEvent, eventChannelBuffer),
	}, nil
}

// Events returns the channel of watch events
func (w *Watcher) Events() <-chan WatchEvent {
	return w.events
}

// Start begins watching the repository for changes
func (w *Watcher) Start(ctx context.Context) error {
	// Add watches recursively
	if err := w.addWatchesRecursive(w.config.RepoRoot); err != nil {
		return err
	}

	// Start the event processing goroutine
	go w.processEvents(ctx)

	w.logger.Info("File watcher started",
		"root", w.config.RepoRoot,
		"debounce", w.config.DebounceDelay)

	return nil
}

// Stop stops the watcher
func (w *Watcher) Stop() error {
	close(w.events)
	return w.watcher.Close()
}

// SetHash records the hash for a file (used during initial indexing)
func (w *Watcher) SetHash(path, hash string) {
	w.hashMu.Lock()
	defer w.hashMu.Unlock()
	w.hashes[path] = hash
}

// GetHash returns the recorded hash for a file
func (w *Watcher) GetHash(path string) (string, bool) {
	w.hashMu.RLock()
	defer w.hashMu.RUnlock()
	hash, ok := w.hashes[path]
	return hash, ok
}

// addWatchesRecursive adds watches to all directories
func (w *Watcher) addWatchesRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only watch directories
		if !info.IsDir() {
			return nil
		}

		// Skip excluded and hidden directories
		base := filepath.Base(path)
		if w.excludes[base] || strings.HasPrefix(base, ".") {
			return filepath.SkipDir
		}

		// Add watch
		if err := w.watcher.Add(path); err != nil {
			w.logger.Warn("Failed to watch directory",
				"path", path,
				"error", err)
		} else {
			w.logger.Debug("Watching directory", "path", path)
		}

		return nil
	})
}

// processEvents handles fsnotify events with debouncing
func (w *Watcher) processEvents(ctx context.Context) {
	ticker := time.NewTicker(w.config.DebounceDelay)
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
			w.logger.Error("Watcher error", "error", err)

		case <-ticker.C:
			w.flushPending(ctx)
		}
	}
}

// handleFSEvent processes a single fsnotify event
func (w *Watcher) handleFSEvent(event fsnotify.Event) {
	path := event.Name

	// Check if it's a watched file extension
	ext := filepath.Ext(path)
	if !w.extensions[ext] {
		// But handle directory creation (for new watches)
		if event.Has(fsnotify.Create) {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				w.handleNewDirectory(path)
			}
		}
		return
	}

	// Skip files in excluded directories
	relPath, _ := filepath.Rel(w.config.RepoRoot, path)
	for excludeDir := range w.excludes {
		if strings.Contains(relPath, excludeDir+"/") {
			return
		}
	}

	// Accumulate pending changes
	w.pendingMu.Lock()
	w.pending[path] = event.Op
	w.pendingMu.Unlock()

	w.logger.Debug("File change detected",
		"path", relPath,
		"op", event.Op.String())
}

// handleNewDirectory adds a watch to a newly created directory
func (w *Watcher) handleNewDirectory(path string) {
	base := filepath.Base(path)
	if w.excludes[base] || strings.HasPrefix(base, ".") {
		return
	}

	if err := w.watcher.Add(path); err != nil {
		w.logger.Warn("Failed to watch new directory",
			"path", path,
			"error", err)
	} else {
		w.logger.Debug("Added watch for new directory", "path", path)
	}
}

// flushPending processes accumulated changes
func (w *Watcher) flushPending(ctx context.Context) {
	w.pendingMu.Lock()
	if len(w.pending) == 0 {
		w.pendingMu.Unlock()
		return
	}

	// Copy and clear pending
	toProcess := make(map[string]fsnotify.Op)
	for k, v := range w.pending {
		toProcess[k] = v
	}
	w.pending = make(map[string]fsnotify.Op)
	w.pendingMu.Unlock()

	// Process each change
	for path, op := range toProcess {
		select {
		case <-ctx.Done():
			return
		default:
		}

		relPath, _ := filepath.Rel(w.config.RepoRoot, path)
		event := WatchEvent{Path: relPath}

		if op.Has(fsnotify.Remove) || op.Has(fsnotify.Rename) {
			// File deleted or renamed (treat rename as delete + create)
			event.Operation = OpDelete

			// Remove from hash cache
			w.hashMu.Lock()
			delete(w.hashes, relPath)
			w.hashMu.Unlock()

			w.sendEvent(event)
			continue
		}

		// Check if file still exists
		if _, err := os.Stat(path); os.IsNotExist(err) {
			event.Operation = OpDelete
			w.sendEvent(event)
			continue
		}

		// Parse the file
		result, err := w.parser.ParseFile(ctx, path)
		if err != nil {
			event.Error = err
			w.sendEvent(event)
			continue
		}

		// Check if content actually changed
		oldHash, hadHash := w.GetHash(relPath)
		if hadHash && oldHash == result.Hash {
			// Content unchanged, skip
			continue
		}

		// Update hash cache
		w.SetHash(relPath, result.Hash)

		if op.Has(fsnotify.Create) || !hadHash {
			event.Operation = OpCreate
		} else {
			event.Operation = OpModify
		}
		event.Result = result

		w.sendEvent(event)
	}
}

// sendEvent sends an event to the output channel
func (w *Watcher) sendEvent(event WatchEvent) {
	select {
	case w.events <- event:
		w.logger.Debug("Sent watch event",
			"path", event.Path,
			"op", event.Operation)
	default:
		dropped := w.droppedEvents.Add(1)
		w.logger.Warn("Event channel full, dropping event",
			"path", event.Path,
			"total_dropped", dropped)
	}
}

// DroppedEvents returns the number of events dropped due to channel overflow
func (w *Watcher) DroppedEvents() int64 {
	return w.droppedEvents.Load()
}

// IndexDirectory performs initial indexing of source files.
// Note: For multi-language support, use the component's parseDirectory method instead
// which handles language-specific parsers. This method is kept for backward compatibility.
func (w *Watcher) IndexDirectory(ctx context.Context) ([]*ParseResult, error) {
	// If parser supports directory parsing (like the Go parser), use it
	if dp, ok := w.parser.(interface {
		ParseDirectory(ctx context.Context, dirPath string) ([]*ParseResult, error)
	}); ok {
		results, err := dp.ParseDirectory(ctx, w.config.RepoRoot)
		if err != nil {
			return nil, err
		}

		// Record hashes for change detection
		for _, result := range results {
			w.SetHash(result.Path, result.Hash)
		}

		return results, nil
	}

	// Otherwise return empty results - caller should use component-level parsing
	return nil, nil
}
