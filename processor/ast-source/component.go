package astsource

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/retry"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/entitypub"
	semsourceast "github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/workspace"
)

// astSourceSchema defines the configuration schema for the ast-source component.
var astSourceSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// pathWatcher manages watching and parsing for a single resolved path.
type pathWatcher struct {
	config   WatchPathConfig
	root     string // Resolved absolute path
	watcher  *semsourceast.Watcher
	parsers  map[string]semsourceast.FileParser // language → parser instance
	excludes map[string]bool                    // set of excluded directory names
}

// Component implements the ast-source processor.
type Component struct {
	name       string
	config     Config
	publisher  *entitypub.Publisher
	natsClient *natsclient.Client
	logger     *slog.Logger
	platform   component.PlatformMeta

	// Per-path watchers
	watchers []*pathWatcher

	// Lifecycle management
	running   bool
	startTime time.Time
	mu        sync.RWMutex

	// Metrics - aggregated across all watchers
	entitiesIndexed atomic.Int64
	parseFailures   atomic.Int64
	errors          atomic.Int64
	lastActivityMu  sync.RWMutex
	lastActivity    time.Time

	// Entity type counters: domain.type → *atomic.Int64
	typeCounts sync.Map

	// Cancel functions for background goroutines
	cancelFuncs []context.CancelFunc

	// Content hashes for change detection during periodic reindex
	fileHashes   map[string]string // path → content hash
	fileHashesMu sync.RWMutex
}

// NewComponent creates a new ast-source processor component.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Start from defaults, then overlay user config.
	config := DefaultConfig()
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	pub, err := entitypub.New(deps.NATSClient, deps.GetLogger())
	if err != nil {
		return nil, fmt.Errorf("create entity publisher: %w", err)
	}

	c := &Component{
		name:       "ast-source",
		config:     config,
		publisher:  pub,
		natsClient: deps.NATSClient,
		logger:     deps.GetLogger(),
		platform:   deps.Platform,
		fileHashes: make(map[string]string),
	}

	return c, nil
}

// initializeWatchers sets up pathWatcher instances for each configured watch path.
// Safe to call multiple times (e.g., during retry); resets watchers on each call.
func (c *Component) initializeWatchers() error {
	c.watchers = nil

	watchPaths := c.config.GetWatchPaths()

	resolved, err := ResolveWatchPaths(watchPaths)
	if err != nil {
		return err
	}

	for _, rp := range resolved {
		// Check that the path is ready — not just that it exists.
		// A git clone creates the directory before populating it.
		if err := workspace.IsRepoReady(rp.AbsPath); err != nil {
			return fmt.Errorf("resolve path %q: %w", rp.AbsPath, err)
		}

		pw := &pathWatcher{
			config:   rp.Config,
			root:     rp.AbsPath,
			parsers:  make(map[string]semsourceast.FileParser),
			excludes: make(map[string]bool),
		}

		for _, exc := range rp.Config.Excludes {
			pw.excludes[exc] = true
		}

		languages := rp.Config.Languages
		if len(languages) == 0 {
			languages = []string{"go"}
		}

		for _, lang := range languages {
			parser, err := semsourceast.DefaultRegistry.CreateParser(lang, rp.Config.Org, rp.Config.Project, rp.AbsPath)
			if err != nil {
				return fmt.Errorf("create parser for %s: %w", lang, err)
			}
			pw.parsers[lang] = parser
		}

		c.watchers = append(c.watchers, pw)
	}

	if len(c.watchers) == 0 {
		return fmt.Errorf("no valid watch paths configured")
	}

	return nil
}

// Initialize prepares the component (no-op; preparation happens in NewComponent).
func (c *Component) Initialize() error {
	return nil
}

// Start begins AST indexing: initial pass, file watchers, and periodic reindex.
// Watch paths may not exist yet (e.g., when a git clone is in progress), so
// watcher initialization is retried with exponential backoff.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	c.mu.Unlock()

	c.publisher.Start(ctx)

	c.publishStatusReport(ctx, "ingesting")

	// Initialize watchers with retry — paths may not exist yet if a git
	// clone is still in progress (repo expansion pattern).
	err := retry.Do(ctx, retry.Persistent(), func() error {
		initErr := c.initializeWatchers()
		if initErr != nil {
			c.logger.Debug("waiting for watch paths to become available",
				"error", initErr)
		}
		return initErr
	})
	if err != nil {
		return fmt.Errorf("initialize watchers: %w", err)
	}

	for _, pw := range c.watchers {
		c.logger.Info("Initializing path watcher",
			"path", pw.root,
			"org", pw.config.Org,
			"project", pw.config.Project,
			"languages", pw.config.Languages)
	}

	c.logger.Info("Starting initial code index", "paths", len(c.watchers))

	totalFiles := 0
	for _, pw := range c.watchers {
		results, err := c.parseDirectory(ctx, pw)
		if err != nil {
			return fmt.Errorf("initial index failed for %s: %w", pw.root, err)
		}

		// Publish repo and folder hierarchy entities before file/symbol entities.
		c.publishHierarchy(ctx, results, pw.config.Org, pw.config.Project)

		for _, result := range results {
			if err := c.publishParseResult(ctx, result); err != nil {
				c.logger.Warn("Failed to publish parse result",
					"path", result.Path,
					"error", err)
			}
			if result.Hash != "" {
				c.setFileHash(result.Path, result.Hash)
			}
		}
		totalFiles += len(results)
	}

	c.logger.Info("Initial index complete",
		"paths", len(c.watchers),
		"files", totalFiles,
		"entities", c.entitiesIndexed.Load(),
		"parse_failures", c.parseFailures.Load())

	c.publishStatusReport(ctx, "watching")

	// Collect cancel funcs locally, then assign under lock to avoid races with Stop.
	var cancels []context.CancelFunc

	cancels = append(cancels, c.startStatusReporter(ctx))

	if c.config.WatchEnabled {
		for _, pw := range c.watchers {
			cancel, err := c.startWatcher(ctx, pw)
			if err != nil {
				c.logger.Warn("Failed to start file watcher",
					"path", pw.root,
					"error", err)
				continue
			}
			cancels = append(cancels, cancel)
		}
	}

	if c.config.IndexInterval != "" {
		cancel := c.startPeriodicIndex(ctx)
		cancels = append(cancels, cancel)
	}

	c.mu.Lock()
	c.cancelFuncs = cancels
	c.running = true
	c.startTime = time.Now()
	c.mu.Unlock()

	return nil
}

// startWatcher starts the file system watcher for a specific path.
// Returns the cancel func for the watcher goroutine.
func (c *Component) startWatcher(ctx context.Context, pw *pathWatcher) (context.CancelFunc, error) {
	excludes := make([]string, 0, len(pw.excludes))
	for exc := range pw.excludes {
		excludes = append(excludes, exc)
	}

	debounceDelay := 100 * time.Millisecond
	if c.config.CoalesceMs > 0 {
		debounceDelay = time.Duration(c.config.CoalesceMs) * time.Millisecond
	}

	watcherConfig := semsourceast.WatcherConfig{
		RepoRoot:       pw.root,
		Org:            pw.config.Org,
		Project:        pw.config.Project,
		DebounceDelay:  debounceDelay,
		Logger:         c.logger,
		FileExtensions: pw.config.GetFileExtensions(),
		ExcludeDirs:    excludes,
	}

	watcher, err := semsourceast.NewWatcherWithParser(watcherConfig, &multiParser{c: c, pw: pw})
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}
	pw.watcher = watcher

	if err := watcher.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start watcher: %w", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)

	go func() {
		for {
			select {
			case <-watchCtx.Done():
				return
			case event, ok := <-watcher.Events():
				if !ok {
					return
				}
				c.handleWatchEvent(watchCtx, pw, event)
			}
		}
	}()

	c.logger.Info("File watcher started",
		"path", pw.root,
		"extensions", pw.config.GetFileExtensions())
	return cancel, nil
}

// multiParser implements semsourceast.FileParser for multi-language support within a path.
type multiParser struct {
	c  *Component
	pw *pathWatcher
}

func (p *multiParser) ParseFile(ctx context.Context, filePath string) (*semsourceast.ParseResult, error) {
	return p.c.parseFileWithWatcher(ctx, p.pw, filePath)
}

// handleWatchEvent processes a file watcher event.
func (c *Component) handleWatchEvent(ctx context.Context, pw *pathWatcher, event semsourceast.WatchEvent) {
	c.updateLastActivity()

	switch event.Operation {
	case semsourceast.OpCreate, semsourceast.OpModify:
		if event.Result != nil {
			// Publish folder chain for new/modified files so containment
			// edges exist even for directories created between full reindexes.
			c.publishFolderChain(ctx, event.Path, pw.config.Org, pw.config.Project)

			if err := c.publishParseResult(ctx, event.Result); err != nil {
				c.logger.Warn("Failed to publish parse result",
					"path", event.Path,
					"error", err)
				c.incrementErrors()
			}
		}
	case semsourceast.OpDelete:
		c.logger.Debug("File deleted", "path", event.Path)
	}

	if event.Error != nil {
		c.logger.Warn("Watch event error",
			"path", event.Path,
			"error", event.Error)
		c.incrementErrors()
	}
}

// startPeriodicIndex starts periodic full reindex.
// Returns the cancel func for the reindex goroutine, or nil if the interval is invalid.
func (c *Component) startPeriodicIndex(ctx context.Context) context.CancelFunc {
	interval, err := time.ParseDuration(c.config.IndexInterval)
	if err != nil {
		c.logger.Warn("Invalid index interval, skipping periodic index", "error", err)
		return nil
	}

	indexCtx, cancel := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-indexCtx.Done():
				return
			case <-ticker.C:
				c.performFullIndex(indexCtx)
			}
		}
	}()

	c.logger.Info("Periodic index started", "interval", interval)
	return cancel
}

// performFullIndex performs a full reindex of all watched paths.
// Files whose content hash hasn't changed since the last index are skipped.
func (c *Component) performFullIndex(ctx context.Context) {
	c.logger.Debug("Starting periodic reindex")

	totalFiles := 0
	published := 0
	for _, pw := range c.watchers {
		results, err := c.parseDirectory(ctx, pw)
		if err != nil {
			c.logger.Error("Periodic reindex failed",
				"path", pw.root,
				"error", err)
			c.incrementErrors()
			continue
		}

		// Re-publish hierarchy entities on reindex (idempotent — deterministic IDs).
		c.publishHierarchy(ctx, results, pw.config.Org, pw.config.Project)

		for _, result := range results {
			totalFiles++

			if result.Hash != "" {
				if oldHash, ok := c.getFileHash(result.Path); ok && oldHash == result.Hash {
					continue
				}
				c.setFileHash(result.Path, result.Hash)
			}

			if err := c.publishParseResult(ctx, result); err != nil {
				c.logger.Warn("Failed to publish parse result during reindex",
					"path", result.Path,
					"error", err)
			}
			published++
		}
	}

	c.logger.Debug("Periodic reindex complete",
		"files_scanned", totalFiles,
		"files_published", published,
		"files_skipped", totalFiles-published)
}

// parseDirectory parses all source files in a directory using the path's configured parsers.
func (c *Component) parseDirectory(ctx context.Context, pw *pathWatcher) ([]*semsourceast.ParseResult, error) {
	var results []*semsourceast.ParseResult

	err := filepath.Walk(pw.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if info.IsDir() {
			base := filepath.Base(path)
			if pw.excludes[base] || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		result, err := c.parseFileWithWatcher(ctx, pw, path)
		if err != nil {
			c.logger.Warn("Failed to parse file",
				"path", path,
				"error", err)
			c.incrementParseFailures()
			return nil
		}

		if result != nil {
			results = append(results, result)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return results, nil
}

// parseFileWithWatcher parses a single file using the appropriate parser from the pathWatcher.
func (c *Component) parseFileWithWatcher(ctx context.Context, pw *pathWatcher, filePath string) (*semsourceast.ParseResult, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	for lang, parser := range pw.parsers {
		for _, langExt := range semsourceast.DefaultRegistry.GetExtensionsForParser(lang) {
			if langExt == ext {
				return parser.ParseFile(ctx, filePath)
			}
		}
	}

	return nil, nil // Unsupported file type — skip silently
}

// publishParseResult converts a ParseResult's entities to EntityPayload messages and publishes them.
func (c *Component) publishParseResult(ctx context.Context, result *semsourceast.ParseResult) error {
	for _, entity := range result.Entities {
		state := entity.EntityState()
		payload := &graph.EntityPayload{
			ID:         state.ID,
			TripleData: state.Triples,
			UpdatedAt:  state.UpdatedAt,
		}
		if err := c.publishEntity(ctx, payload); err != nil {
			return fmt.Errorf("publish entity %s: %w", state.ID, err)
		}
		c.entitiesIndexed.Add(1)
		c.trackEntityType(state.ID)
		c.updateLastActivity()
	}
	return nil
}

// publishHierarchy builds and publishes repo and folder entities for a batch of parse results.
func (c *Component) publishHierarchy(ctx context.Context, results []*semsourceast.ParseResult, org, project string) {
	entities := semsourceast.BuildHierarchy(results, org, project)
	for _, entity := range entities {
		state := entity.EntityState()
		payload := &graph.EntityPayload{
			ID:         state.ID,
			TripleData: state.Triples,
			UpdatedAt:  state.UpdatedAt,
		}
		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish hierarchy entity",
				"id", entity.ID, "error", err)
			continue
		}
		c.entitiesIndexed.Add(1)
		c.trackEntityType(state.ID)
	}
}

// publishFolderChain publishes folder entities for a single file's ancestor directories.
// Used during watch events to ensure containment edges exist for newly created directories.
func (c *Component) publishFolderChain(ctx context.Context, filePath, org, project string) {
	entities := semsourceast.BuildFolderChain(filePath, org, project)
	for _, entity := range entities {
		state := entity.EntityState()
		payload := &graph.EntityPayload{
			ID:         state.ID,
			TripleData: state.Triples,
			UpdatedAt:  state.UpdatedAt,
		}
		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish folder entity",
				"id", entity.ID, "error", err)
			continue
		}
		c.entitiesIndexed.Add(1)
		c.trackEntityType(state.ID)
	}
}

// publishEntity enqueues an EntityPayload for buffered publishing via the entity publisher.
func (c *Component) publishEntity(_ context.Context, payload *graph.EntityPayload) error {
	c.publisher.Send(payload)
	return nil
}

// updateLastActivity safely updates the last activity timestamp.
func (c *Component) updateLastActivity() {
	c.lastActivityMu.Lock()
	c.lastActivity = time.Now()
	c.lastActivityMu.Unlock()
}

// getLastActivity safely retrieves the last activity timestamp.
func (c *Component) getLastActivity() time.Time {
	c.lastActivityMu.RLock()
	defer c.lastActivityMu.RUnlock()
	return c.lastActivity
}

// incrementErrors safely increments the error counter.
func (c *Component) incrementErrors() {
	c.errors.Add(1)
}

// incrementParseFailures safely increments the parse failure counter.
func (c *Component) incrementParseFailures() {
	c.parseFailures.Add(1)
}

// setFileHash records a content hash for a file path.
func (c *Component) setFileHash(path, hash string) {
	c.fileHashesMu.Lock()
	c.fileHashes[path] = hash
	c.fileHashesMu.Unlock()
}

// trackEntityType increments the per-type counter for the given entity ID.
func (c *Component) trackEntityType(id string) {
	domain, eType := entityid.Parts(id)
	if domain == "" {
		return
	}
	key := domain + "." + eType
	val, _ := c.typeCounts.LoadOrStore(key, &atomic.Int64{})
	val.(*atomic.Int64).Add(1)
}

// snapshotTypeCounts returns a point-in-time copy of all per-type counts.
func (c *Component) snapshotTypeCounts() map[string]int64 {
	counts := make(map[string]int64)
	c.typeCounts.Range(func(k, v any) bool {
		counts[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	return counts
}

// getFileHash returns the recorded content hash for a file path.
func (c *Component) getFileHash(path string) (string, bool) {
	c.fileHashesMu.RLock()
	defer c.fileHashesMu.RUnlock()
	hash, ok := c.fileHashes[path]
	return hash, ok
}

// publishStatusReport sends a status report to the manifest component via NATS core.
func (c *Component) publishStatusReport(ctx context.Context, phase string) {
	report := struct {
		InstanceName string           `json:"instance_name"`
		SourceType   string           `json:"source_type"`
		Phase        string           `json:"phase"`
		EntityCount  int64            `json:"entity_count"`
		ErrorCount   int64            `json:"error_count"`
		TypeCounts   map[string]int64 `json:"type_counts,omitempty"`
		Timestamp    time.Time        `json:"timestamp"`
	}{
		InstanceName: c.config.InstanceName,
		SourceType:   "ast",
		Phase:        phase,
		EntityCount:  c.entitiesIndexed.Load(),
		ErrorCount:   c.errors.Load(),
		TypeCounts:   c.snapshotTypeCounts(),
		Timestamp:    time.Now(),
	}
	data, err := json.Marshal(report)
	if err != nil {
		c.logger.Warn("failed to marshal status report", "error", err)
		return
	}
	if err := c.natsClient.Publish(ctx, "semsource.internal.status", data); err != nil {
		c.logger.Debug("failed to publish status report", "error", err)
	}
}

// startStatusReporter starts a goroutine that periodically re-publishes the
// component's status so the source-manifest component always has fresh data.
// Returns a cancel func that stops the goroutine.
func (c *Component) startStatusReporter(ctx context.Context) context.CancelFunc {
	rCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-rCtx.Done():
				return
			case <-ticker.C:
				c.publishStatusReport(rCtx, "watching")
			}
		}
	}()
	return cancel
}

// Stop gracefully stops the component within the given timeout.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	c.logger.Info("entity publisher stats",
		"published", c.publisher.Published(),
		"dropped", c.publisher.Dropped(),
		"retries", c.publisher.Retries())
	c.publisher.Stop()

	for _, cancel := range c.cancelFuncs {
		cancel()
	}
	c.cancelFuncs = nil

	for _, pw := range c.watchers {
		if pw.watcher != nil {
			if err := pw.watcher.Stop(); err != nil {
				c.logger.Warn("Error stopping watcher",
					"path", pw.root,
					"error", err)
			}
		}
	}

	c.running = false
	c.logger.Info("AST source stopped",
		"paths", len(c.watchers),
		"entities_indexed", c.entitiesIndexed.Load(),
		"parse_failures", c.parseFailures.Load(),
		"errors", c.errors.Load())

	return nil
}

// Discoverable interface implementation

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "ast-source",
		Type:        "processor",
		Description: "Multi-language AST source for semsource code entity extraction and graph ingestion",
		Version:     "0.1.0",
	}
}

// InputPorts returns configured input port definitions.
// ast-source has no input ports — it generates data from the file system.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{}
}

// OutputPorts returns configured output port definitions.
func (c *Component) OutputPorts() []component.Port {
	if c.config.Ports == nil {
		return []component.Port{}
	}

	ports := make([]component.Port, len(c.config.Ports.Outputs))
	for i, portDef := range c.config.Ports.Outputs {
		ports[i] = buildPort(portDef, component.DirectionOutput)
	}
	return ports
}

// buildPort creates a component.Port from a PortDefinition.
func buildPort(portDef component.PortDefinition, direction component.Direction) component.Port {
	port := component.Port{
		Name:        portDef.Name,
		Direction:   direction,
		Required:    portDef.Required,
		Description: portDef.Description,
	}
	if portDef.Type == "jetstream" {
		port.Config = component.JetStreamPort{
			StreamName: portDef.StreamName,
			Subjects:   []string{portDef.Subject},
		}
	} else {
		port.Config = component.NATSPort{
			Subject: portDef.Subject,
		}
	}
	return port
}

// ConfigSchema returns the configuration schema.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return astSourceSchema
}

// Health returns the current health status.
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	running := c.running
	startTime := c.startTime
	c.mu.RUnlock()

	status := "stopped"
	if running {
		status = "running"
	}

	return component.HealthStatus{
		Healthy:    running,
		LastCheck:  time.Now(),
		ErrorCount: int(c.errors.Load()),
		Uptime:     time.Since(startTime),
		Status:     status,
	}
}

// DataFlow returns current data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	return component.FlowMetrics{
		MessagesPerSecond: 0,
		BytesPerSecond:    0,
		ErrorRate:         0,
		LastActivity:      c.getLastActivity(),
	}
}
