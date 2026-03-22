package docsource

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
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
	"github.com/c360studio/semsource/workspace"
)

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

	// Entity type counters: domain.type → *atomic.Int64
	typeCounts sync.Map

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

	var handlerOpts []dochandler.Option
	if config.ContentThreshold > 0 {
		handlerOpts = append(handlerOpts, dochandler.WithContentThreshold(config.ContentThreshold))
	}

	h := dochandler.NewWithOrg(config.Org, handlerOpts...)

	sc := &sourceCfg{
		paths:        config.Paths,
		watchEnabled: config.WatchEnabled,
		coalesceMs:   config.CoalesceMs,
	}

	c := &Component{
		name:       "doc-source",
		config:     config,
		publisher:  pub,
		natsClient: deps.NATSClient,
		logger:     deps.GetLogger(),
		platform:   deps.Platform,
		handler:    h,
		sourceCfg:  sc,
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

	// Wire ObjectStore for large document content if configured.
	if c.config.ContentThreshold > 0 && c.config.ContentBucket != "" {
		store, err := objectstore.NewStoreWithConfig(ctx, c.natsClient, objectstore.Config{
			BucketName: c.config.ContentBucket,
		})
		if err != nil {
			c.logger.Debug("content store not available, all doc content will be inline",
				"bucket", c.config.ContentBucket, "error", err)
		} else {
			c.handler = dochandler.NewWithOrg(c.config.Org,
				dochandler.WithStore(store, c.config.ContentBucket),
				dochandler.WithContentThreshold(c.config.ContentThreshold),
			)
			c.logger.Info("content store wired for large document storage",
				"bucket", c.config.ContentBucket,
				"threshold", c.config.ContentThreshold)
		}
	}

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
// handler (bypassing the normalizer) and publishes each EntityPayload to NATS.
func (c *Component) ingestOnce(ctx context.Context) error {
	states, err := c.handler.IngestEntityStates(ctx, c.sourceCfg, c.config.Org)
	if err != nil {
		c.ingestErrors.Add(1)
		return fmt.Errorf("doc handler ingest: %w", err)
	}

	for _, state := range states {
		payload := &graph.EntityPayload{
			ID:         state.ID,
			TripleData: state.Triples,
			UpdatedAt:  state.UpdatedAt,
			Storage:    state.StorageRef,
		}

		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish doc entity",
				"id", state.ID,
				"error", err)
			c.ingestErrors.Add(1)
			continue
		}

		c.entitiesPublished.Add(1)
		c.trackEntityType(state.ID)
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

// handleChangeEvent processes a change event from the doc handler's watch channel.
// When the handler populates EntityStates (normalizer-free path), those are used
// directly. The Entities fallback is retained for backward compatibility.
func (c *Component) handleChangeEvent(ctx context.Context, event handler.ChangeEvent) {
	c.logger.Debug("Doc-source change event received",
		"path", event.Path,
		"operation", event.Operation,
		"entity_states", len(event.EntityStates),
		"entities", len(event.Entities))

	// Prefer pre-built EntityStates — no normalizer pass required.
	if len(event.EntityStates) > 0 {
		for _, state := range event.EntityStates {
			payload := &graph.EntityPayload{
				ID:         state.ID,
				TripleData: state.Triples,
				UpdatedAt:  state.UpdatedAt,
				Storage:    state.StorageRef,
			}
			if err := c.publishEntity(ctx, payload); err != nil {
				c.logger.Warn("Failed to publish doc entity on change",
					"id", state.ID,
					"error", err)
				c.ingestErrors.Add(1)
				continue
			}
			c.entitiesPublished.Add(1)
			c.trackEntityType(state.ID)
			c.updateLastActivity()
		}
		return
	}

	// Fallback: event carried only RawEntities (e.g. handler constructed without org).
	// This path should not be reached in normal operation but is retained to
	// avoid silent drops during a mixed-version deployment.
	c.logger.Warn("Doc-source change event has no EntityStates; handler may be missing org",
		"path", event.Path,
		"entities", len(event.Entities))
}

// publishEntity enqueues an EntityPayload for buffered publishing via entitypub.
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
		SourceType:   "docs",
		Phase:        phase,
		EntityCount:  c.entitiesPublished.Load(),
		ErrorCount:   c.ingestErrors.Load(),
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
		ErrorCount: int(c.ingestErrors.Load()),
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
