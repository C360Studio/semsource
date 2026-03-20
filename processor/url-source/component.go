package urlsource

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

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	urlhandler "github.com/c360studio/semsource/handler/url"
	"github.com/c360studio/semsource/internal/entitypub"
)

// urlSourceSchema defines the configuration schema for the url-source component.
var urlSourceSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// sourceCfg is a minimal handler.SourceConfig adapter for the URL handler.
// It satisfies the handler.SourceConfig interface and the optional
// urlhandler.URLSourceConfig interface so the handler can read the poll interval.
type sourceCfg struct {
	rawURL       string
	pollInterval string
	watchEnabled bool
}

func (s *sourceCfg) GetType() string             { return handler.SourceTypeURL }
func (s *sourceCfg) GetPath() string             { return "" }
func (s *sourceCfg) GetPaths() []string          { return nil }
func (s *sourceCfg) GetURL() string              { return s.rawURL }
func (s *sourceCfg) GetBranch() string           { return "" }
func (s *sourceCfg) IsWatchEnabled() bool        { return s.watchEnabled }
func (s *sourceCfg) GetKeyframeMode() string     { return "" }
func (s *sourceCfg) GetKeyframeInterval() string { return "" }
func (s *sourceCfg) GetSceneThreshold() float64  { return 0 }

// GetPollInterval implements urlhandler.URLSourceConfig so the handler honours
// the configured interval rather than falling back to its own default.
func (s *sourceCfg) GetPollInterval() string { return s.pollInterval }

// Component implements the url-source processor.
// It delegates all fetching and change detection to the existing handler/url
// package, which handles SSRF-safe retrieval, ETag-based conditional fetching,
// and content-hash diffing.
type Component struct {
	name       string
	config     Config
	publisher  *entitypub.Publisher
	natsClient *natsclient.Client
	logger     *slog.Logger
	platform   component.PlatformMeta

	handler *urlhandler.URLHandler

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

// NewComponent creates a new url-source processor component.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	h := urlhandler.NewWithOrg(deps.GetLogger(), config.Org)

	pub, err := entitypub.New(deps.NATSClient, deps.GetLogger())
	if err != nil {
		return nil, fmt.Errorf("create entity publisher: %w", err)
	}

	c := &Component{
		name:       "url-source",
		config:     config,
		publisher:  pub,
		natsClient: deps.NATSClient,
		logger:     deps.GetLogger(),
		platform:   deps.Platform,
		handler:    h,
	}

	return c, nil
}

// Initialize prepares the component (no-op; preparation happens in NewComponent).
func (c *Component) Initialize() error {
	return nil
}

// Start performs the initial ingest of all configured URLs, then starts a
// polling watcher for each URL to detect content changes.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	c.mu.Unlock()

	c.publisher.Start(ctx)

	c.publishStatusReport(ctx, "ingesting")

	c.logger.Info("Starting url-source initial ingest",
		"urls", len(c.config.URLs),
		"org", c.config.Org,
		"poll_interval", c.config.PollInterval)

	if err := c.ingestAll(ctx); err != nil {
		return fmt.Errorf("initial url ingest failed: %w", err)
	}

	c.logger.Info("URL-source initial ingest complete",
		"entities_published", c.entitiesPublished.Load())

	c.publishStatusReport(ctx, "watching")

	for _, rawURL := range c.config.URLs {
		cancel := c.startPolling(ctx, rawURL)
		if cancel != nil {
			c.mu.Lock()
			c.cancelFuncs = append(c.cancelFuncs, cancel)
			c.mu.Unlock()
		}
	}

	c.mu.Lock()
	c.cancelFuncs = append(c.cancelFuncs, c.startStatusReporter(ctx))
	c.running = true
	c.startTime = time.Now()
	c.mu.Unlock()

	return nil
}

// ingestAll performs a single ingest pass over every configured URL using
// IngestEntityStates (bypassing the normalizer).
func (c *Component) ingestAll(ctx context.Context) error {
	for _, rawURL := range c.config.URLs {
		sc := &sourceCfg{
			rawURL:       rawURL,
			pollInterval: c.config.PollInterval,
			watchEnabled: false,
		}

		states, err := c.handler.IngestEntityStates(ctx, sc, c.config.Org)
		if err != nil {
			c.logger.Warn("URL ingest failed",
				"url", rawURL,
				"error", err)
			c.ingestErrors.Add(1)
			continue
		}

		for _, state := range states {
			payload := &graph.EntityPayload{
				ID:         state.ID,
				TripleData: state.Triples,
				UpdatedAt:  state.UpdatedAt,
			}
			if err := c.publishEntity(ctx, payload); err != nil {
				c.logger.Warn("Failed to publish URL entity",
					"url", rawURL,
					"id", state.ID,
					"error", err)
				c.ingestErrors.Add(1)
				continue
			}
			c.entitiesPublished.Add(1)
			c.trackEntityType(state.ID)
			c.updateLastActivity()
		}
	}
	return nil
}

// startPolling starts a watcher goroutine for a single URL and returns its
// cancel func. Returns nil if watch setup fails.
func (c *Component) startPolling(ctx context.Context, rawURL string) context.CancelFunc {
	pollCtx, cancel := context.WithCancel(ctx)

	sc := &sourceCfg{
		rawURL:       rawURL,
		pollInterval: c.config.PollInterval,
		watchEnabled: true,
	}

	changeCh, err := c.handler.Watch(pollCtx, sc)
	if err != nil {
		c.logger.Warn("Failed to start URL watcher, skipping watch",
			"url", rawURL,
			"error", err)
		cancel()
		return nil
	}

	if changeCh == nil {
		cancel()
		return nil
	}

	c.logger.Info("URL-source polling started",
		"url", rawURL,
		"poll_interval", c.config.PollInterval)

	go func() {
		for {
			select {
			case <-pollCtx.Done():
				return
			case event, ok := <-changeCh:
				if !ok {
					return
				}
				c.handleChangeEvent(pollCtx, event)
			}
		}
	}()

	return cancel
}

// handleChangeEvent processes a ChangeEvent from a URL watcher channel.
// When the handler populates EntityStates (normalizer-free path), those are used
// directly. The Entities fallback is retained for backward compatibility.
func (c *Component) handleChangeEvent(ctx context.Context, event handler.ChangeEvent) {
	c.logger.Debug("URL-source change event received",
		"url", event.Path,
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
			}
			if err := c.publishEntity(ctx, payload); err != nil {
				c.logger.Warn("Failed to publish URL entity on change",
					"url", event.Path,
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
	c.logger.Warn("URL-source change event has no EntityStates; handler may be missing org",
		"url", event.Path,
		"entities", len(event.Entities))
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
		SourceType:   "url",
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

	c.logger.Info("URL-source stopped",
		"entities_published", c.entitiesPublished.Load(),
		"ingest_errors", c.ingestErrors.Load())

	return nil
}

// Discoverable interface implementation

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "url-source",
		Type:        "processor",
		Description: "HTTP/S URL source for semsource web page entity extraction and change detection",
		Version:     "0.1.0",
	}
}

// InputPorts returns an empty slice — url-source generates data from remote URLs.
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
	return urlSourceSchema
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
