package docsource

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/retry"
	"github.com/c360studio/semstreams/storage/objectstore"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	dochandler "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/internal/entitypub"
	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semsource/workspace"
)

// lifecycleTriggerTimeout bounds the background NATS round trip triggering a
// staleness lifecycle pass. Fire-and-forget from the watch loop's
// perspective — a missing or slow responder degrades staleness marking, it
// never blocks or fails ingestion.
const lifecycleTriggerTimeout = 30 * time.Second

// docSourceSchema defines the configuration schema for the doc-source component.
var docSourceSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// sourceCfg is a minimal handler.SourceConfig adapter for the doc handler.
// It satisfies the handler.SourceConfig interface without coupling this package
// to the full semsource config model.
type sourceCfg struct {
	paths        []string
	watchEnabled bool
	coalesceMs   int
}

func (s *sourceCfg) GetType() string             { return "docs" }
func (s *sourceCfg) GetPath() string             { return "" }
func (s *sourceCfg) GetPaths() []string          { return s.paths }
func (s *sourceCfg) GetURL() string              { return "" }
func (s *sourceCfg) GetBranch() string           { return "" }
func (s *sourceCfg) IsWatchEnabled() bool        { return s.watchEnabled }
func (s *sourceCfg) GetKeyframeMode() string     { return "" }
func (s *sourceCfg) GetKeyframeInterval() string { return "" }
func (s *sourceCfg) GetSceneThreshold() float64  { return 0 }
func (s *sourceCfg) GetCoalesceMs() int          { return s.coalesceMs }

// Component implements the doc-source processor.
// It delegates all filesystem operations to the existing handler/doc package,
// which handles directory walking, content hashing, and fsnotify-based watching.
type Component struct {
	name       string
	config     Config
	publisher  *entitypub.Publisher
	natsClient *natsclient.Client
	logger     *slog.Logger
	platform   component.PlatformMeta

	handler   *dochandler.Handler
	sourceCfg *sourceCfg

	// Lifecycle
	running   bool
	startTime time.Time
	mu        sync.RWMutex

	// Metrics
	entitiesPublished atomic.Int64
	ingestErrors      atomic.Int64
	lastActivityMu    sync.RWMutex
	lastActivity      time.Time

	// distinct tracks distinct entity IDs for honest status counts
	// (publish counters inflate under periodic reindex — audit 2026-07-19).
	distinct *entitypub.DistinctTracker

	// passageCounts is the last published passage count per document path, used
	// to notice a document shrinking so vanished passages get marked promptly.
	// In-memory and therefore lossy across a restart on purpose — the lifecycle
	// pass is graph-derived and converges the same answer without it.
	passageCountMu sync.Mutex
	passageCounts  map[string]int

	// Background goroutine cancellation
	cancelFuncs []context.CancelFunc
}

// NewComponent creates a new doc-source processor component.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
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

	// A minimal handler so c.handler is never nil before Start; Start rebuilds it
	// with the wired body store (which needs a context to attach). The live
	// handler is the one built in Start.
	h := dochandler.NewWithOrg(config.Org)

	sc := &sourceCfg{
		paths:        config.Paths,
		watchEnabled: config.WatchEnabled,
		coalesceMs:   config.CoalesceMs,
	}

	c := &Component{
		name:          "doc-source",
		config:        config,
		publisher:     pub,
		distinct:      entitypub.NewDistinctTracker(),
		natsClient:    deps.NATSClient,
		logger:        deps.GetLogger(),
		platform:      deps.Platform,
		handler:       h,
		sourceCfg:     sc,
		passageCounts: make(map[string]int),
	}

	return c, nil
}

// Initialize prepares the component (no-op; preparation happens in NewComponent).
func (c *Component) Initialize() error {
	return nil
}

// Start performs the initial document ingest across all configured paths, then
// sets up fsnotify watching if WatchEnabled is true.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	if c.publisher == nil {
		c.mu.Unlock()
		return fmt.Errorf("entity publisher required")
	}
	c.mu.Unlock()

	c.publisher.Start(ctx)

	// Assemble handler storage: the fusion verbatim-body store, so doc-context
	// hydrates passages by handle (ADR-062) and graph-embedding embeds the same
	// offloaded body via the shared StoreRegistry (ADR-063). One CONTENT blob,
	// two consumers — no separate large-content store.
	var opts []dochandler.Option
	if bodyStore, err := objectstore.NewStoreWithConfig(ctx, c.natsClient, objectstore.Config{
		BucketName:   graph.BodyStoreBucket,
		InstanceName: graph.BodyStoreInstance,
	}); err != nil {
		c.logger.Warn("verbatim body store unavailable; doc bodies will not be offloaded", "error", err)
	} else {
		opts = append(opts, dochandler.WithBodyStore(bodyStore, graph.BodyStoreInstance))
	}
	c.handler = dochandler.NewWithOrg(c.config.Org, opts...)

	c.publishStatusReport(ctx, "ingesting")

	c.logger.Info("Starting doc-source initial ingest",
		"paths", c.config.Paths,
		"org", c.config.Org,
		"watch_enabled", c.config.WatchEnabled)

	// Retry initial ingest — paths may not exist yet if a git clone is still
	// in progress (repo expansion pattern). We check IsRepoReady rather than
	// just os.Stat because git clone creates the directory before populating it.
	if err := retry.Do(ctx, retry.Persistent(), func() error {
		for _, p := range c.config.Paths {
			if err := workspace.IsRepoReady(p); err != nil {
				c.logger.Debug("waiting for doc paths to become available",
					"path", p, "error", err)
				return err
			}
		}
		return c.ingestOnce(ctx)
	}); err != nil {
		return fmt.Errorf("initial doc ingest failed: %w", err)
	}

	c.logger.Info("Doc-source initial ingest complete",
		"paths", c.config.Paths,
		"entities_published", c.entitiesPublished.Load())

	c.publishStatusReport(ctx, "watching")

	cancel := c.startWatching(ctx)

	c.mu.Lock()
	if cancel != nil {
		c.cancelFuncs = append(c.cancelFuncs, cancel)
	}
	c.cancelFuncs = append(c.cancelFuncs, c.startStatusReporter(ctx))
	c.running = true
	c.startTime = time.Now()
	c.mu.Unlock()

	return nil
}

// ingestOnce runs a single ingest pass: calls IngestEntityStates on the doc
// handler and publishes each EntityPayload to NATS.
func (c *Component) ingestOnce(ctx context.Context) error {
	states, err := c.handler.IngestEntityStates(ctx, c.sourceCfg, c.config.Org)
	if err != nil {
		c.ingestErrors.Add(1)
		return fmt.Errorf("doc handler ingest: %w", err)
	}

	for _, state := range states {
		payload, err := entitypub.PayloadFromState(state)
		if err != nil {
			c.logger.Warn("Invalid doc entity state",
				"id", state.ID,
				"error", err)
			c.ingestErrors.Add(1)
			continue
		}

		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish doc entity",
				"id", state.ID,
				"error", err)
			c.ingestErrors.Add(1)
			continue
		}

		c.entitiesPublished.Add(1)
		c.distinct.Observe(state.ID)
		c.updateLastActivity()
	}

	return nil
}

// startWatching starts a goroutine that fans in fsnotify events from all watched
// paths and re-publishes changed document entities. Returns the cancel func for
// the watch goroutine, or nil if watching is disabled or setup fails.
func (c *Component) startWatching(ctx context.Context) context.CancelFunc {
	watchCtx, cancel := context.WithCancel(ctx)

	changeCh, err := c.handler.Watch(watchCtx, c.sourceCfg)
	if err != nil {
		c.logger.Warn("Failed to start doc watcher, skipping watch",
			"paths", c.config.Paths,
			"error", err)
		cancel()
		return nil
	}

	if changeCh == nil {
		// Watch returned nil channel — WatchEnabled is false.
		cancel()
		return nil
	}

	c.logger.Info("Doc-source fsnotify watching started", "paths", c.config.Paths)

	go func() {
		for {
			select {
			case <-watchCtx.Done():
				return
			case event, ok := <-changeCh:
				if !ok {
					return
				}
				c.handleChangeEvent(watchCtx, event)
			}
		}
	}()

	return cancel
}

// handleChangeEvent processes a typed change event from the doc handler.
func (c *Component) handleChangeEvent(ctx context.Context, event handler.ChangeEvent) {
	c.logger.Debug("Doc-source change event received",
		"path", event.Path,
		"operation", event.Operation,
		"entity_states", len(event.EntityStates))

	if event.Operation == handler.OperationDelete {
		if c.deleteTriggersLifecycleRun() {
			if root, ok := c.rootForPath(event.Path); ok {
				c.triggerLifecycleRun(ctx, root, graph.LifecycleReasonFileDeleted)
			} else {
				c.logger.Debug("delete event path not under any configured root; skipping lifecycle trigger",
					"path", event.Path)
			}
		}
		return
	}

	// Prefer pre-built EntityStates.
	if len(event.EntityStates) > 0 {
		for _, state := range event.EntityStates {
			payload, err := entitypub.PayloadFromState(state)
			if err != nil {
				stateID := ""
				if state != nil {
					stateID = state.ID
				}
				c.logger.Warn("Invalid doc entity state on change",
					"id", stateID,
					"error", err)
				c.ingestErrors.Add(1)
				continue
			}
			if err := c.publishEntity(ctx, payload); err != nil {
				c.logger.Warn("Failed to publish doc entity on change",
					"id", state.ID,
					"error", err)
				c.ingestErrors.Add(1)
				continue
			}
			c.entitiesPublished.Add(1)
			c.distinct.Observe(state.ID)
			c.updateLastActivity()
		}
		c.notePassageCount(ctx, event.Path, event.EntityStates)
		return
	}

	c.ingestErrors.Add(1)
	c.logger.Warn("Doc-source change event missing required EntityStates",
		"path", event.Path,
		"operation", event.Operation)
}

// publishEntity enqueues an EntityPayload for buffered publishing via entitypub.
func (c *Component) publishEntity(_ context.Context, payload *graph.EntityPayload) error {
	return c.publisher.Send(payload)
}

// notePassageCount records how many passages a document just published and
// announces scope to the lifecycle pass when that count DROPPED.
//
// A shrinking document is invisible to the pass's filesystem check — every
// passage of a ten-passage document that is now seven passages long still
// carries the path of a file that is still on disk — so the three orphans would
// otherwise keep serving deleted prose as current. The pass itself decides what
// is stale (index versus the parent's DocChunkCount); this only makes it prompt.
//
// The count is tracked in memory rather than read back from the graph, so the
// common edit costs no I/O at all. That makes it lossy across a restart by
// design: the pass is graph-derived and converges the same answer on its next
// run, which is exactly the division of labour the fast path assumes.
func (c *Component) notePassageCount(ctx context.Context, path string, states []*handler.EntityState) {
	passages := 0
	for _, st := range states {
		if st != nil && stateIsPassage(st) {
			passages++
		}
	}

	c.passageCountMu.Lock()
	if c.passageCounts == nil {
		// A Component assembled directly rather than through NewComponent still
		// has to survive a change event; a nil map here would panic on write.
		c.passageCounts = make(map[string]int)
	}
	previous, seen := c.passageCounts[path]
	c.passageCounts[path] = passages
	c.passageCountMu.Unlock()

	if !seen {
		// First sighting of this path — no previous count to compare against.
		return
	}
	if !c.deleteTriggersLifecycleRun() || passages >= previous {
		return
	}
	root, ok := c.rootForPath(path)
	if !ok {
		return
	}
	c.logger.Debug("document shrank; announcing scope so vanished passages are marked",
		"path", path, "was", previous, "now", passages)
	c.triggerLifecycleRun(ctx, root, graph.LifecycleReasonPassageRemoved)
}

// stateIsPassage reports whether an entity state is a passage rather than the
// parent document, read from the emitted DocType fact.
func stateIsPassage(state *handler.EntityState) bool {
	for i := range state.Triples {
		if state.Triples[i].Predicate == source.DocType {
			v, _ := state.Triples[i].Object.(string)
			return v == "passage"
		}
	}
	return false
}

// deleteTriggersLifecycleRun reports whether a delete event should announce
// scope to the staleness lifecycle pass. False only when watching is
// disabled — a frozen source (D5) never goes stale, so its vanished paths
// must never be marked. The fsnotify fan-in goroutine only runs when
// WatchEnabled is true anyway, so this is belt-and-suspenders, but it keeps
// the invariant directly unit-testable without a live fsnotify pipeline.
func (c *Component) deleteTriggersLifecycleRun() bool {
	return c.config.WatchEnabled
}

// rootForPath returns the configured Paths entry that is an ancestor of (or
// equal to) absPath, resolving both to absolute form for comparison.
// doc-source may watch multiple root paths; a deleted file's containing root
// determines the entity-ID system and the RootPath the lifecycle pass
// anchors its liveness stat to.
func (c *Component) rootForPath(absPath string) (root string, ok bool) {
	for _, p := range c.config.Paths {
		absRoot, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if absPath == absRoot || strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) {
			return absRoot, true
		}
	}
	return "", false
}

// triggerLifecycleRun announces root's scope to the staleness lifecycle pass
// (processor/supersession), fired in the background so the watch loop is
// never blocked on a full graph pass.
func (c *Component) triggerLifecycleRun(ctx context.Context, root, reason string) {
	req := graph.LifecycleRunRequest{
		Org:      c.config.Org,
		Systems:  []string{entityid.SystemSlug(root)},
		RootPath: root,
		Reason:   reason,
	}
	go func() {
		runCtx, cancel := context.WithTimeout(ctx, lifecycleTriggerTimeout)
		defer cancel()
		if _, err := graph.PublishLifecycleTrigger(runCtx, c.natsClient, req); err != nil {
			c.logger.Debug("lifecycle trigger failed (staleness marking degraded, not fatal)",
				"root", root, "reason", reason, "error", err)
		}
	}()
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

// publishStatusReport sends a status report to the manifest component via NATS core.
func (c *Component) publishStatusReport(ctx context.Context, phase string) {
	report := struct {
		InstanceName string           `json:"instance_name"`
		SourceType   string           `json:"source_type"`
		Phase        string           `json:"phase"`
		EntityCount  int64            `json:"entity_count"`
		PublishTotal int64            `json:"publish_total,omitempty"`
		ErrorCount   int64            `json:"error_count"`
		TypeCounts   map[string]int64 `json:"type_counts,omitempty"`
		Timestamp    time.Time        `json:"timestamp"`
	}{
		InstanceName: c.config.InstanceName,
		SourceType:   "docs",
		Phase:        phase,
		EntityCount:  c.distinct.Count(),
		PublishTotal: c.entitiesPublished.Load(),
		ErrorCount:   c.ingestErrors.Load() + c.publisher.Lost(),
		TypeCounts:   c.distinct.TypeCounts(),
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
	c.running = false

	c.logger.Info("Doc-source stopped",
		"entities_published", c.entitiesPublished.Load(),
		"ingest_errors", c.ingestErrors.Load())

	return nil
}

// Discoverable interface implementation

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "doc-source",
		Type:        "processor",
		Description: "Document source for semsource markdown and plain-text entity extraction",
		Version:     "0.1.0",
	}
}

// InputPorts returns an empty slice — doc-source generates data from the filesystem.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{}
}

// OutputPorts returns the configured output port definitions.
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
	return docSourceSchema
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
		ErrorCount: int(c.ingestErrors.Load() + c.publisher.Lost()),
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
