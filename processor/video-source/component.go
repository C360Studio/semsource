package videosource

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

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	videohandler "github.com/c360studio/semsource/handler/video"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/storage/filestore"
)

// videoSourceSchema defines the configuration schema for the video-source component.
var videoSourceSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// sourceCfg is a minimal handler.SourceConfig adapter for the video handler.
// It satisfies the handler.SourceConfig interface without coupling this package
// to the full semsource config model.
type sourceCfg struct {
	paths            []string
	watchEnabled     bool
	coalesceMs       int
	keyframeMode     string
	keyframeInterval string
	sceneThreshold   float64
}

func (s *sourceCfg) GetType() string             { return "video" }
func (s *sourceCfg) GetPath() string             { return "" }
func (s *sourceCfg) GetPaths() []string          { return s.paths }
func (s *sourceCfg) GetURL() string              { return "" }
func (s *sourceCfg) GetBranch() string           { return "" }
func (s *sourceCfg) IsWatchEnabled() bool        { return s.watchEnabled }
func (s *sourceCfg) GetKeyframeMode() string     { return s.keyframeMode }
func (s *sourceCfg) GetKeyframeInterval() string { return s.keyframeInterval }
func (s *sourceCfg) GetSceneThreshold() float64  { return s.sceneThreshold }
func (s *sourceCfg) GetCoalesceMs() int          { return s.coalesceMs }

// Component implements the video-source processor.
// It delegates all filesystem operations to the existing handler/video package,
// which handles directory walking, ffprobe metadata extraction, keyframe extraction,
// and fsnotify-based watching. When FileStoreRoot is configured, binary content
// is stored in the local filesystem via filestore.
type Component struct {
	name       string
	config     Config
	publisher  *entitypub.Publisher
	natsClient *natsclient.Client
	logger     *slog.Logger
	platform   component.PlatformMeta

	handler   *videohandler.Handler
	sourceCfg *sourceCfg
	fileStore *filestore.Store

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

// NewComponent creates a new video-source processor component.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Build handler options; start with org for entity ID construction.
	opts := []videohandler.Option{videohandler.WithOrg(config.Org)}

	// Wire binary storage when a root directory is configured.
	var fs *filestore.Store
	if config.FileStoreRoot != "" {
		s, err := filestore.New(config.FileStoreRoot, true)
		if err != nil {
			return nil, fmt.Errorf("create file store: %w", err)
		}
		fs = s
		opts = append(opts, videohandler.WithStore(s))
	}

	h := videohandler.New(opts...)

	sc := &sourceCfg{
		paths:            config.Paths,
		watchEnabled:     config.WatchEnabled,
		coalesceMs:       config.CoalesceMs,
		keyframeMode:     config.KeyframeMode,
		keyframeInterval: config.KeyframeInterval,
		sceneThreshold:   config.SceneThreshold,
	}

	pub, err := entitypub.New(deps.NATSClient, deps.GetLogger())
	if err != nil {
		return nil, fmt.Errorf("create entity publisher: %w", err)
	}

	c := &Component{
		name:       "video-source",
		config:     config,
		publisher:  pub,
		natsClient: deps.NATSClient,
		logger:     deps.GetLogger(),
		platform:   deps.Platform,
		handler:    h,
		sourceCfg:  sc,
		fileStore:  fs,
	}

	return c, nil
}

// Initialize prepares the component (no-op; preparation happens in NewComponent).
func (c *Component) Initialize() error {
	return nil
}

// Start performs the initial video ingest across all configured paths, then
// sets up fsnotify watching if WatchEnabled is true.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	c.mu.Unlock()

	c.publisher.Start(ctx)

	c.publishStatusReport(ctx, "ingesting")

	c.logger.Info("Starting video-source initial ingest",
		"paths", c.config.Paths,
		"org", c.config.Org,
		"watch_enabled", c.config.WatchEnabled,
		"keyframe_mode", c.config.KeyframeMode,
		"keyframe_interval", c.config.KeyframeInterval)

	if err := c.ingestOnce(ctx); err != nil {
		return fmt.Errorf("initial video ingest failed: %w", err)
	}

	c.logger.Info("Video-source initial ingest complete",
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

// ingestOnce runs a single ingest pass: calls IngestEntityStates on the video
// handler and publishes each EntityState directly as a graph.EntityPayload,
// bypassing the normalizer entirely.
func (c *Component) ingestOnce(ctx context.Context) error {
	states, err := c.handler.IngestEntityStates(ctx, c.sourceCfg, c.config.Org)
	if err != nil {
		c.ingestErrors.Add(1)
		return fmt.Errorf("video handler ingest: %w", err)
	}

	for _, state := range states {
		payload := entityStateToPayload(state)
		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish video entity",
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

// startWatching starts a goroutine that fans in fsnotify events from all watched
// paths and re-publishes changed video entities. Returns the cancel func for
// the watch goroutine, or nil if watching is disabled or setup fails.
func (c *Component) startWatching(ctx context.Context) context.CancelFunc {
	watchCtx, cancel := context.WithCancel(ctx)

	changeCh, err := c.handler.Watch(watchCtx, c.sourceCfg)
	if err != nil {
		c.logger.Warn("Failed to start video watcher, skipping watch",
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

	c.logger.Info("Video-source fsnotify watching started", "paths", c.config.Paths)

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

// handleChangeEvent processes a change event from the video handler's watch channel.
// It uses event.EntityStates (populated by enrichEvent when org is set) so that
// no normalizer pass is needed.
func (c *Component) handleChangeEvent(ctx context.Context, event handler.ChangeEvent) {
	c.logger.Debug("Video-source change event received",
		"path", event.Path,
		"operation", event.Operation,
		"entity_states", len(event.EntityStates))

	for _, state := range event.EntityStates {
		payload := entityStateToPayload(state)
		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish video entity on change",
				"id", state.ID,
				"error", err)
			c.ingestErrors.Add(1)
			continue
		}
		c.entitiesPublished.Add(1)
		c.updateLastActivity()
	}
}

// entityStateToPayload converts a handler.EntityState to a graph.EntityPayload
// for publication to the NATS graph ingestion stream.
func entityStateToPayload(state *handler.EntityState) *graph.EntityPayload {
	return &graph.EntityPayload{
		ID:         state.ID,
		TripleData: state.Triples,
		UpdatedAt:  state.UpdatedAt,
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

// publishStatusReport sends a status report to the manifest component via NATS core.
func (c *Component) publishStatusReport(ctx context.Context, phase string) {
	report := struct {
		InstanceName string    `json:"instance_name"`
		SourceType   string    `json:"source_type"`
		Phase        string    `json:"phase"`
		EntityCount  int64     `json:"entity_count"`
		ErrorCount   int64     `json:"error_count"`
		Timestamp    time.Time `json:"timestamp"`
	}{
		InstanceName: c.config.InstanceName,
		SourceType:   "video",
		Phase:        phase,
		EntityCount:  c.entitiesPublished.Load(),
		ErrorCount:   c.ingestErrors.Load(),
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

	c.publisher.Stop()
	c.logger.Info("entity publisher stats",
		"published", c.publisher.Published(),
		"dropped", c.publisher.Dropped(),
		"retries", c.publisher.Retries())

	for _, cancel := range c.cancelFuncs {
		cancel()
	}
	c.cancelFuncs = nil
	c.running = false

	if c.fileStore != nil {
		if err := c.fileStore.Close(); err != nil {
			c.logger.Warn("Failed to close file store", "error", err)
		}
	}

	c.logger.Info("Video-source stopped",
		"entities_published", c.entitiesPublished.Load(),
		"ingest_errors", c.ingestErrors.Load())

	return nil
}

// Discoverable interface implementation

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "video-source",
		Type:        "processor",
		Description: "Video source for semsource metadata and keyframe entity extraction",
		Version:     "0.1.0",
	}
}

// InputPorts returns an empty slice — video-source generates data from the filesystem.
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
	return videoSourceSchema
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
