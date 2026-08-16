package astsource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/retry"
	"github.com/c360studio/semstreams/storage"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/internal/degraded"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/seedsup"
	semsourceast "github.com/c360studio/semsource/source/ast"
	"github.com/c360studio/semsource/source/ontology"
	"github.com/c360studio/semsource/workspace"
)

// lifecycleTriggerTimeout bounds the background NATS round trip triggering a
// staleness lifecycle pass. Fire-and-forget from the watch/reindex loop's
// perspective — a missing or slow responder degrades staleness marking, it
// never blocks or fails ingestion.
const lifecycleTriggerTimeout = 30 * time.Second

// astSourceSchema defines the configuration schema for the ast-source component.
var astSourceSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// pathWatcher manages watching and parsing for a single resolved path.
type pathWatcher struct {
	config       WatchPathConfig
	root         string // Resolved absolute path
	scopedSystem string // Pre-computed version-scoped system slug (entityid.ScopedSystemSlug result)
	watcher      *semsourceast.Watcher
	parsers      map[string]semsourceast.FileParser // language → parser instance
	routes       map[string]string                  // file extension → language (see routing.go)
	excludes     map[string]bool                    // set of excluded directory names

	// parseMu serializes ParseFile across this path's parsers. Parser instances
	// carry per-file state (the tree-sitter parser, and the language import-binding
	// map used for cross-file reference resolution) that is unsafe to interleave.
	// The fsnotify watcher and the periodic-reindex goroutine — both on by default —
	// otherwise drive the same parser instance concurrently.
	parseMu sync.Mutex
}

// Component implements the ast-source processor.
type Component struct {
	name       string
	config     Config
	publisher  *entitypub.Publisher
	natsClient *natsclient.Client
	logger     *slog.Logger
	platform   component.PlatformMeta

	// bodyStore holds verbatim code bodies offloaded at ingest so the fusion
	// code lens can hydrate them by handle, location-independently (ADR-062 /
	// ADR-0006 §5). nil when unavailable — bodies are then simply not offloaded.
	bodyStore storage.Store

	// Per-path watchers
	watchers []*pathWatcher

	// Lifecycle management
	running   bool
	startTime time.Time
	mu        sync.RWMutex

	// Metrics - aggregated across all watchers
	// statusPublishFailing is edge-triggered (ADR-0011): status reporting IS the
	// readiness surface, so a failure must be visible at the default level
	// without logging once per interval while it persists.
	statusPublishFailing degraded.Condition
	// pathsUnavailable: a source that cannot reach its configured paths seeds
	// nothing, silently, for the whole retry window.
	pathsUnavailable degraded.Condition
	// lifecycleFailing: staleness marking has stopped working.
	lifecycleFailing degraded.Condition

	// bodyPuts records body keys already written this process. Bodies are
	// content-addressed, so a repeat key is byte-identical and its Put is waste.
	bodyPuts sync.Map

	// seed supervises the asynchronous initial seed (internal/seedsup).
	seed seedsup.Supervisor

	// phase is the lifecycle phase the periodic reporter publishes. Held as an
	// atomic because the reporter goroutine samples it while Start mutates it.
	phase atomic.Value

	entitiesIndexed atomic.Int64
	parseFailures   atomic.Int64
	errors          atomic.Int64
	lastActivityMu  sync.RWMutex
	lastActivity    time.Time

	// distinct tracks distinct entity IDs for honest status counts
	// (publish counters inflate under periodic reindex — audit 2026-07-19).
	distinct *entitypub.DistinctTracker

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
	decoder := json.NewDecoder(bytes.NewReader(rawConfig))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	pub, err := entitypub.New(deps.NATSClient, deps.GetLogger(),
		// Publish-boundary telemetry, keyed by instance so one stalled source
		// cannot hide behind healthy siblings. Registry is nil in tests.
		entitypub.WithMetrics(deps.MetricsRegistry, config.InstanceName))
	if err != nil {
		return nil, fmt.Errorf("create entity publisher: %w", err)
	}

	c := &Component{
		name:       "ast-source",
		config:     config,
		publisher:  pub,
		distinct:   entitypub.NewDistinctTracker(),
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

	resolved, err := ResolveWatchPaths(c.config.WatchPaths)
	if err != nil {
		return err
	}

	for _, rp := range resolved {
		// Check that the path is ready — not just that it exists.
		// A git clone creates the directory before populating it.
		if err := workspace.IsRepoReady(rp.AbsPath); err != nil {
			return fmt.Errorf("resolve path %q: %w", rp.AbsPath, err)
		}

		// Compute the scoped system slug once per watch path. ScopedSystemSlug
		// applies a final SystemSlug pass after the join, ensuring the result is
		// ≤80 chars and idempotent under SystemSlug. This means NewScopedCodeEntity's
		// inner SystemSlug call and the golang parser's raw entityid.Build calls
		// (parser.go:503/551) both produce the same system segment — no dangling
		// edges. An empty Version produces the bare SystemSlug of project,
		// preserving existing IDs byte-for-byte for clean projects.
		scopedSystem := entityid.ScopedSystemSlug(rp.Config.Project, rp.Config.Version)

		pw := &pathWatcher{
			config:       rp.Config,
			root:         rp.AbsPath,
			scopedSystem: scopedSystem,
			parsers:      make(map[string]semsourceast.FileParser),
			excludes:     make(map[string]bool),
		}

		// Seed the floor FIRST, then add configured excludes. Additive, never
		// substituted: a config that names one directory must not silently
		// re-enable node_modules. The shipped default config names none at all,
		// which is how ingesting a JS project's dependency tree became the
		// default behaviour.
		for _, exc := range handler.DefaultExcludedDirNames() {
			pw.excludes[exc] = true
		}
		for _, exc := range rp.Config.Excludes {
			pw.excludes[exc] = true
		}

		languages := rp.Config.Languages
		if len(languages) == 0 {
			languages = []string{"go"}
		}

		for _, lang := range languages {
			// Pass scopedSystem as the project so all entity IDs produced by the
			// parser use the consistent, version-scoped system segment.
			parser, err := semsourceast.DefaultRegistry.CreateParser(lang, rp.Config.Org, scopedSystem, rp.AbsPath)
			if err != nil {
				return fmt.Errorf("create parser for %s: %w", lang, err)
			}
			pw.parsers[lang] = parser
		}

		// Resolve extension → language once, here, rather than per file: the
		// answer depends only on the declared language set, and computing it
		// once is what makes it impossible for two files with the same
		// extension to be routed differently within one watch path.
		pw.routes = routeExtensions(languages, semsourceast.DefaultRegistry.GetExtensionsForParser)

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

// statusReportInterval is how often a source republishes its status. It applies
// during the initial seed as well as the watch phase, so "is this moving?" is
// answerable on a fixed cadence regardless of corpus size — a per-entity or
// per-file signal would scale its own cost with the corpus and emit nothing at
// all in the pathological case where no entities are moving.
const statusReportInterval = 30 * time.Second

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
	c.initBodyStore(ctx)

	c.publishStatusReport(ctx, "ingesting")

	c.startProgressReporting(ctx)

	// Everything from here is unbounded — the path retry alone is ~30 attempts —
	// so it runs in a supervised goroutine and Start returns. Holding the
	// lifecycle hook across it blocks the framework's component-start barrier,
	// which in turn prevents the HTTP status and metrics listeners from binding
	// at all (semstreams#867): the surfaces stay dark for the whole seed, which
	// is exactly when an operator needs them.
	c.markRunning()
	c.seed.Start(ctx, c.logger, c.runSeed)
	return nil
}

// seed performs the initial index and everything sequenced after it. It runs in
// its own goroutine; failures surface through the source's error count and a
// WARN rather than as a Start error, because there is no longer a Start to fail.
func (c *Component) runSeed(ctx context.Context) error {
	// Initialize watchers with retry — paths may not exist yet if a git
	// clone is still in progress (repo expansion pattern).
	err := retry.Do(ctx, retry.Persistent(), func() error {
		initErr := c.initializeWatchers()
		if initErr != nil {
			// Bounded by retry.Persistent (~30 attempts), but a source that never
			// reaches its paths seeds nothing at all — surface it once rather than
			// leaving the whole wait at Debug, where it was invisible.
			c.pathsUnavailable.Enter(c.logger,
				"source paths are not available yet — seeding cannot start", "error", initErr)
			return initErr
		}
		c.pathsUnavailable.Clear(c.logger, "source paths became available")
		return nil
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

		// Resolve cross-file type references now that the whole watch path is
		// parsed. Languages whose names map deterministically onto paths (Go,
		// Java, Python) resolve inside the parser; C++ cannot, because nothing
		// ties `class Foo` to a file without a preprocessor and include paths.
		// Doing it here — over the complete result set — keeps it deterministic
		// and independent of walk order.
		for _, parser := range pw.parsers {
			if r, ok := parser.(typeRefResolver); ok {
				r.ResolveTypeRefs(results)
			}
		}

		// Publish repo and folder hierarchy entities before file/symbol entities.
		// Pass the scoped system slug so hierarchy IDs match the code entity IDs.
		c.publishHierarchy(ctx, results, pw.config.Org, pw.scopedSystem)

		for _, result := range results {
			if err := c.publishParseResult(ctx, result, pw); err != nil {
				c.logger.Warn("Failed to publish parse result",
					"path", result.Path,
					"error", err)
				c.incrementErrors()
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
	// The status reporter is already running (started before the seed).
	var cancels []context.CancelFunc

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
	c.cancelFuncs = append(c.cancelFuncs, cancels...)
	c.mu.Unlock()

	return nil
}

// markRunning marks the component live. "Running" means started, NOT seeded —
// those are now genuinely different states, and `phase` is what distinguishes
// them (ingesting vs watching).
func (c *Component) markRunning() {
	c.mu.Lock()
	c.running = true
	c.startTime = time.Now()
	c.mu.Unlock()
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
			c.publishFolderChain(ctx, event.Path, pw.config.Org, pw.scopedSystem)

			if err := c.publishParseResult(ctx, event.Result, pw); err != nil {
				c.logger.Warn("Failed to publish parse result",
					"path", event.Path,
					"error", err)
				c.incrementErrors()
			}
		}
	case semsourceast.OpDelete:
		c.logger.Debug("File deleted", "path", event.Path)
		if c.deleteTriggersLifecycleRun() {
			c.triggerLifecycleRun(ctx, pw, graph.LifecycleReasonFileDeleted)
		}
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
		c.publishHierarchy(ctx, results, pw.config.Org, pw.scopedSystem)

		for _, result := range results {
			totalFiles++

			if result.Hash != "" {
				if oldHash, ok := c.getFileHash(result.Path); ok && oldHash == result.Hash {
					continue
				}
				c.setFileHash(result.Path, result.Hash)
			}

			if err := c.publishParseResult(ctx, result, pw); err != nil {
				c.logger.Warn("Failed to publish parse result during reindex",
					"path", result.Path,
					"error", err)
				c.incrementErrors()
			}
			published++
		}

		// Every sweep announces scope to the staleness lifecycle pass — not
		// just sweeps that observed a change — so a delete that happened
		// while semsource was down (no fsnotify event ever fired) is still
		// detected on the first pass after boot (design D2: graph-derived,
		// restart-proof).
		c.triggerLifecycleRun(ctx, pw, graph.LifecycleReasonPathMissing)
	}

	c.logger.Debug("Periodic reindex complete",
		"files_scanned", totalFiles,
		"files_published", published,
		"files_skipped", totalFiles-published)
}

// deleteTriggersLifecycleRun reports whether an OpDelete event should
// announce scope to the staleness lifecycle pass. False only when watching
// is disabled — a frozen source (D5) never goes stale, so its vanished paths
// must never be marked. The fast-path watcher goroutine only runs when
// WatchEnabled is true anyway, so this is belt-and-suspenders, but it keeps
// the invariant directly unit-testable without a live fsnotify pipeline.
func (c *Component) deleteTriggersLifecycleRun() bool {
	return c.config.WatchEnabled
}

// lifecycleRunRequestFor builds the staleness lifecycle trigger request for
// one watch path. Pure (no I/O) so the scope-wiring is unit-testable without
// a NATS connection.
func lifecycleRunRequestFor(pw *pathWatcher, reason string) graph.LifecycleRunRequest {
	return graph.LifecycleRunRequest{
		Org:      pw.config.Org,
		Systems:  []string{pw.scopedSystem},
		RootPath: pw.root,
		Reason:   reason,
	}
}

// triggerLifecycleRun announces this watch path's scope to the staleness
// lifecycle pass (processor/supersession), fired in the background so the
// watch/reindex loop is never blocked on a full graph pass. Callers gate
// whether to call this at all (the fast path only when WatchEnabled; the
// periodic sweep only ever runs when IndexInterval is set) — this method
// itself does not re-derive scope-eligibility, since "watch:false with an
// explicit index_interval" is a legitimate tracked source (D5), not a frozen
// one, and the two call sites have different preconditions.
func (c *Component) triggerLifecycleRun(ctx context.Context, pw *pathWatcher, reason string) {
	req := lifecycleRunRequestFor(pw, reason)
	go func() {
		runCtx, cancel := context.WithTimeout(ctx, lifecycleTriggerTimeout)
		defer cancel()
		if _, err := graph.PublishLifecycleTrigger(runCtx, c.natsClient, req); err != nil {
			// "degraded" is in the original message: when this fails, staleness
			// marking stops and retracted entities linger. Edge-triggered (ADR-0011).
			c.lifecycleFailing.Enter(c.logger,
				"staleness lifecycle trigger is failing — retractions may linger",
				"path", pw.root, "reason", reason, "error", err)
		} else {
			c.lifecycleFailing.Clear(c.logger, "staleness lifecycle trigger recovered")
		}
	}()
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

	// Routed through the precomputed table rather than by scanning pw.parsers.
	// That scan ranged a map, so once two declared languages claimed one
	// extension — which C and C++ both do, on ".h" — the parser that read a
	// given file was chosen by Go's randomized map order. The same header could
	// then be parsed as a different language from one run to the next, changing
	// the {domain} segment of every entity ID it produced.
	lang, ok := pw.routes[ext]
	if !ok {
		return nil, nil // Unsupported file type — skip silently
	}
	parser, ok := pw.parsers[lang]
	if !ok {
		return nil, nil
	}

	// Serialize: parser instances hold per-file state (tree-sitter + import
	// bindings) that the watcher and reindex goroutines must not interleave.
	// Held only across one file, so reindex releases between files and watch
	// events still make progress.
	pw.parseMu.Lock()
	res, err := parser.ParseFile(ctx, filePath)
	pw.parseMu.Unlock()
	return res, err
}

// publishParseResult converts a ParseResult's entities to EntityPayload messages and publishes them.
func (c *Component) publishParseResult(ctx context.Context, result *semsourceast.ParseResult, pw *pathWatcher) error {
	bodies := c.bodiesForResult(ctx, result, pw.root)
	for _, entity := range result.Entities {
		// Stamp raw source identity + version (ADR-0008 #2) so the entity's triple
		// builder emits code.artifact.project / code.artifact.version. These come
		// from the watch-path config; the parser only sees the folded system slug.
		entity.Project = pw.config.Project
		entity.Version = pw.config.Version
		state := entity.EntityState()
		// Zero value for a container / body-less entity: nil triples (append is a
		// no-op) and a nil StorageRef.
		body := bodies[state.ID]
		state.Triples = append(state.Triples, body.triples...)
		payload, err := payloadFromASTState(state, body.ref)
		if err != nil {
			return fmt.Errorf("invalid AST entity state %s: %w", state.ID, err)
		}
		if err := c.publishEntity(ctx, payload); err != nil {
			return fmt.Errorf("publish entity %s: %w", state.ID, err)
		}
		c.entitiesIndexed.Add(1)
		c.distinct.Observe(state.ID)
		c.updateLastActivity()
	}
	return nil
}

// publishHierarchy builds and publishes repo and folder entities for a batch of parse results.
func (c *Component) publishHierarchy(ctx context.Context, results []*semsourceast.ParseResult, org, project string) {
	entities := semsourceast.BuildHierarchy(results, org, project)
	for _, entity := range entities {
		state := entity.EntityState()
		payload, err := payloadFromASTState(state, nil)
		if err != nil {
			c.logger.Warn("Invalid hierarchy entity state",
				"id", state.ID,
				"error", err)
			continue
		}
		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish hierarchy entity",
				"id", entity.ID, "error", err)
			continue
		}
		c.entitiesIndexed.Add(1)
		c.distinct.Observe(state.ID)
	}
}

// publishFolderChain publishes folder entities for a single file's ancestor directories.
// Used during watch events to ensure containment edges exist for newly created directories.
func (c *Component) publishFolderChain(ctx context.Context, filePath, org, project string) {
	entities := semsourceast.BuildFolderChain(filePath, org, project)
	for _, entity := range entities {
		state := entity.EntityState()
		payload, err := payloadFromASTState(state, nil)
		if err != nil {
			c.logger.Warn("Invalid folder entity state",
				"id", state.ID,
				"error", err)
			continue
		}
		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish folder entity",
				"id", entity.ID, "error", err)
			continue
		}
		c.entitiesIndexed.Add(1)
		c.distinct.Observe(state.ID)
	}
}

// payloadFromASTState builds a Graphable payload from an AST entity state. ref is
// the entity's verbatim-body StorageReference (nil for entities with no offloaded
// body — containers, or when no body store is wired); when set it points at the
// same CONTENT blob as the code.body.* handle triples so graph-embedding embeds
// the body via the shared StoreRegistry (ADR-063).
func payloadFromASTState(state *semsourceast.EntityState, ref *message.StorageReference) (*graph.EntityPayload, error) {
	payload := &graph.EntityPayload{
		ID:                  state.ID,
		TripleData:          ontology.StampClass(state.ID, state.Triples, state.UpdatedAt),
		UpdatedAt:           state.UpdatedAt,
		Storage:             ref,
		IndexingProfileHint: state.IndexingProfile,
	}
	if err := entitypub.ValidatePayload(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// publishEntity enqueues an EntityPayload for buffered publishing via the entity publisher.
func (c *Component) publishEntity(_ context.Context, payload *graph.EntityPayload) error {
	return c.publisher.Send(payload)
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

// getFileHash returns the recorded content hash for a file path.
func (c *Component) getFileHash(path string) (string, bool) {
	c.fileHashesMu.RLock()
	defer c.fileHashesMu.RUnlock()
	hash, ok := c.fileHashes[path]
	return hash, ok
}

// startProgressReporting starts the periodic status reporter BEFORE the initial
// index, not after it, so the seed is observable while it runs. The cancel is
// registered immediately: the initial index can return an error, and a reporter
// registered only on the success path would leak on that return.
func (c *Component) startProgressReporting(ctx context.Context) {
	cancel := c.startStatusReporter(ctx)
	c.mu.Lock()
	c.cancelFuncs = append(c.cancelFuncs, cancel)
	c.mu.Unlock()
}

// setPhase records the phase the periodic reporter should publish. The reporter
// samples this rather than taking a fixed phase, so ONE reporter covers both the
// seed and the watch window (ingest-observability D2).
func (c *Component) setPhase(phase string) { c.phase.Store(phase) }

// currentPhase returns the phase to report, defaulting to the seeding phase:
// the reporter starts before the initial index completes.
func (c *Component) currentPhase() string {
	if p, ok := c.phase.Load().(string); ok && p != "" {
		return p
	}
	return "ingesting"
}

// publishStatusReport sends a status report to the manifest component via NATS core.
func (c *Component) publishStatusReport(ctx context.Context, phase string) {
	// Publishing a phase IS the transition, so the reporter's sampled phase can
	// never diverge from the last one published.
	c.setPhase(phase)
	report := struct {
		InstanceName string           `json:"instance_name"`
		SourceType   string           `json:"source_type"`
		Phase        string           `json:"phase"`
		EntityCount  int64            `json:"entity_count"`
		PublishTotal int64            `json:"publish_total,omitempty"`
		ErrorCount   int64            `json:"error_count"`
		TypeCounts   map[string]int64 `json:"type_counts,omitempty"`
		Backpressure bool             `json:"backpressure,omitempty"`
		LastError    *seedsup.Error   `json:"last_error,omitempty"`
		Timestamp    time.Time        `json:"timestamp"`
	}{
		InstanceName: c.config.InstanceName,
		SourceType:   "ast",
		Phase:        phase,
		// Distinct entities, invariant under periodic republication; raw
		// publish throughput is the separately named publish_total
		// (honest-readiness-and-errors D5).
		EntityCount:  c.distinct.Count(),
		PublishTotal: c.entitiesIndexed.Load(),
		// Delivery truth: parse failures and publisher losses (overflow drops +
		// terminal publish failures) surface here — a healthy-looking status must
		// imply entities actually reached the substrate (no-silent-entity-loss).
		ErrorCount: c.errors.Load() + c.parseFailures.Load() + c.publisher.Lost(),
		TypeCounts: c.distinct.TypeCounts(),
		// A publisher retrying against a refusing transport reports no drops, no
		// failures, and no errors while being functionally stalled. Surfacing the
		// condition is what separates "slow" from "stalled" without reading logs.
		Backpressure: c.publisher.InBackpressure(),
		LastError:    c.seed.LastError(),
		Timestamp:    time.Now(),
	}
	data, err := json.Marshal(report)
	if err != nil {
		c.logger.Warn("failed to marshal status report", "error", err)
		return
	}
	if err := c.natsClient.Publish(ctx, "semsource.internal.status", data); err != nil {
		// Status reporting IS the readiness surface: if it fails silently the
		// component looks stalled to every consumer while being healthy, or vice
		// versa. Reported on the transition into failure so a persistent outage
		// costs one line, not one per interval.
		c.statusPublishFailing.Enter(c.logger,
			"status reporting is failing — readiness will go stale", "error", err)
		return
	}
	c.statusPublishFailing.Clear(c.logger, "status reporting recovered")
}

// startStatusReporter starts a goroutine that periodically re-publishes the
// component's status so the source-manifest component always has fresh data.
// Returns a cancel func that stops the goroutine.
func (c *Component) startStatusReporter(ctx context.Context) context.CancelFunc {
	rCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(statusReportInterval)
		defer ticker.Stop()
		for {
			select {
			case <-rCtx.Done():
				return
			case <-ticker.C:
				// Sample the CURRENT phase. Before this, the reporter was started
				// only after the initial index and hardcoded "watching", so a seed
				// published status exactly twice — at its edges — with the counts
				// frozen in between. That is what made a stalled seed
				// indistinguishable from a slow one.
				c.publishStatusReport(rCtx, c.currentPhase())
			}
		}
	}()
	return cancel
}

// Stop gracefully stops the component; ctx bounds the seed join and cleanup.
func (c *Component) Stop(ctx context.Context) error {
	c.mu.Lock()
	running := c.running
	c.mu.Unlock()

	if !running {
		return nil
	}

	// Cancel the seed and WAIT for it BEFORE taking the lock, for two reasons:
	// this mutex is not reentrant and stopSeed takes it, and — the subtler one —
	// the seed's own tail locks c.mu to register its cancel funcs, so waiting on
	// the seed while holding the lock would deadlock against it.
	//
	// Draining first is also what makes shutdown safe: publisher.Stop() closes
	// the buffer, so a seed still running would publish into a closed publisher.
	c.seed.Stop(ctx, c.logger)

	c.mu.Lock()
	defer c.mu.Unlock()

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
	return component.Port{
		Name:        portDef.Name,
		Direction:   direction,
		Required:    portDef.Required,
		Description: portDef.Description,
		Config:      portDef.Config,
	}
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
		ErrorCount: int(c.errors.Load() + c.parseFailures.Load() + c.publisher.Lost()),
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
