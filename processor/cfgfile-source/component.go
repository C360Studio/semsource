package cfgfilesource

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

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	cfghandler "github.com/c360studio/semsource/handler/cfgfile"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/workspace"
)

// cfgfileSourceSchema defines the configuration schema for the cfgfile-source component.
var cfgfileSourceSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// sourceCfg is a minimal handler.SourceConfig adapter that satisfies the
// handler.SourceConfig interface using paths and watch settings from Config.
type sourceCfg struct {
	paths        []string
	watchEnabled bool
}

func (s *sourceCfg) GetType() string             { return handler.SourceTypeConfig }
func (s *sourceCfg) GetPath() string             { return "" }
func (s *sourceCfg) GetPaths() []string          { return s.paths }
func (s *sourceCfg) GetURL() string              { return "" }
func (s *sourceCfg) GetBranch() string           { return "" }
func (s *sourceCfg) IsWatchEnabled() bool        { return s.watchEnabled }
func (s *sourceCfg) GetKeyframeMode() string     { return "" }
func (s *sourceCfg) GetKeyframeInterval() string { return "" }
func (s *sourceCfg) GetSceneThreshold() float64  { return 0 }

// Component implements the cfgfile-source processor.
// It delegates all file discovery, parsing, and watching to the existing
// handler/cfgfile package and publishes EntityPayload messages with
// vocabulary-predicate triples directly to NATS JetStream — no normalizer pass.
type Component struct {
	name       string
	config     Config
	publisher  *entitypub.Publisher
	natsClient *natsclient.Client
	logger     *slog.Logger
	platform   component.PlatformMeta

	handler   *cfghandler.ConfigHandler
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

	// Background goroutine cancellation
	cancelFuncs []context.CancelFunc
}

// NewComponent creates a new cfgfile-source processor component.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Pass the org into the handler so Watch events also carry EntityStates.
	h := cfghandler.New(&cfghandler.Config{Org: config.Org})
	sc := &sourceCfg{
		paths:        config.Paths,
		watchEnabled: config.WatchEnabled,
	}

	pub, err := entitypub.New(deps.NATSClient, deps.GetLogger())
	if err != nil {
		return nil, fmt.Errorf("create entity publisher: %w", err)
	}

	c := &Component{
		name:       "cfgfile-source",
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

// Start performs the initial config file ingest across all configured paths,
// then optionally starts the filesystem watcher for real-time change detection.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	c.mu.Unlock()

	c.publisher.Start(ctx)

	c.logger.Info("Starting cfgfile-source initial ingest",
		"paths", c.config.Paths,
		"org", c.config.Org,
		"watch_enabled", c.config.WatchEnabled)

	// Retry initial ingest — paths may not exist yet if a git clone is still
	// in progress (repo expansion pattern). We check IsRepoReady rather than
	// just os.Stat because git clone creates the directory before populating it.
	if err := retry.Do(ctx, retry.Persistent(), func() error {
		for _, p := range c.config.Paths {
			if err := workspace.IsRepoReady(p); err != nil {
				c.logger.Debug("waiting for cfgfile paths to become available",
					"path", p, "error", err)
				return err
			}
		}
		return c.ingestOnce(ctx)
	}); err != nil {
		return fmt.Errorf("initial cfgfile ingest failed: %w", err)
	}

	c.logger.Info("Cfgfile-source initial ingest complete",
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

// ingestOnce runs a single ingest pass using IngestEntityStates — the
// normalizer-free path that builds vocabulary-predicate triples directly.
func (c *Component) ingestOnce(ctx context.Context) error {
	states, err := c.handler.IngestEntityStates(ctx, c.sourceCfg, c.config.Org)
	if err != nil {
		c.ingestErrors.Add(1)
		return fmt.Errorf("cfgfile handler ingest: %w", err)
	}

	for _, state := range states {
		payload := &graph.EntityPayload{
			ID:         state.ID,
			TripleData: state.Triples,
			UpdatedAt:  state.UpdatedAt,
		}
		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish cfgfile entity",
				"id", state.ID,
				"error", err)
			c.ingestErrors.Add(1)
			continue
		}
		c.entitiesPublished.Add(1)
		c.updateLastActivity()
	}

	return nil
}

// startWatching starts a goroutine that receives fsnotify change events from
// the cfgfile handler and re-publishes updated entity payloads. Returns the
// cancel func for the watching goroutine, or nil if watch setup fails or
// watching is disabled.
func (c *Component) startWatching(ctx context.Context) context.CancelFunc {
	watchCtx, cancel := context.WithCancel(ctx)

	changeCh, err := c.handler.Watch(watchCtx, c.sourceCfg)
	if err != nil {
		c.logger.Warn("Failed to start cfgfile watcher, skipping watch",
			"paths", c.config.Paths,
			"error", err)
		cancel()
		return nil
	}

	if changeCh == nil {
		// Watch returned nil channel — watching disabled.
		cancel()
		return nil
	}

	c.logger.Info("Cfgfile-source watching started", "paths", c.config.Paths)

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

// handleChangeEvent processes a change event from the cfgfile handler's watch
// channel. When the handler has populated EntityStates (normalizer-free path),
// those are used directly; otherwise the event is silently skipped since the
// component no longer carries a normalizer.
func (c *Component) handleChangeEvent(ctx context.Context, event handler.ChangeEvent) {
	c.logger.Debug("Cfgfile-source change event received",
		"path", event.Path,
		"operation", event.Operation,
		"entity_states", len(event.EntityStates))

	for _, state := range event.EntityStates {
		payload := &graph.EntityPayload{
			ID:         state.ID,
			TripleData: state.Triples,
			UpdatedAt:  state.UpdatedAt,
		}
		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish cfgfile entity on change",
				"id", state.ID,
				"error", err)
			c.ingestErrors.Add(1)
			continue
		}
		c.entitiesPublished.Add(1)
		c.updateLastActivity()
	}
}

// publishEntity enqueues an EntityPayload for buffered publishing via the
// entity publisher. Non-blocking; the publisher handles NATS delivery.
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

// publishStatusReport sends a status report to the manifest component via NATS core.
func (c *Component) publishStatusReport(ctx context.Context, phase string) {
	report := struct {
		SourceType  string    `json:"source_type"`
		Phase       string    `json:"phase"`
		EntityCount int64     `json:"entity_count"`
		ErrorCount  int64     `json:"error_count"`
		Timestamp   time.Time `json:"timestamp"`
	}{
		SourceType:  "config",
		Phase:       phase,
		EntityCount: c.entitiesPublished.Load(),
		ErrorCount:  c.ingestErrors.Load(),
		Timestamp:   time.Now(),
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

	c.logger.Info("Cfgfile-source stopped",
		"entities_published", c.entitiesPublished.Load(),
		"ingest_errors", c.ingestErrors.Load())

	return nil
}

// Discoverable interface implementation

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "cfgfile-source",
		Type:        "processor",
		Description: "Config file source for semsource module, package, image, and dependency entity extraction",
		Version:     "0.1.0",
	}
}

// InputPorts returns an empty slice — cfgfile-source generates data from the filesystem.
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
	return cfgfileSourceSchema
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
