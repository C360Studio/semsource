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

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	urlhandler "github.com/c360studio/semsource/handler/url"
	"github.com/c360studio/semsource/internal/degraded"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/seedsup"
	"github.com/c360studio/semsource/internal/sourcestatus"
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

	// seed supervises the asynchronous initial seed (internal/seedsup): Start
	// must return promptly or the framework's component-start barrier keeps the
	// HTTP and metrics listeners from binding (semstreams#867).
	seed seedsup.Supervisor

	// phase is the lifecycle phase the periodic reporter publishes; atomic
	// because the reporter goroutine samples it while Start mutates it.
	phase atomic.Value
	// statusPublishFailing is edge-triggered (ADR-0011): status reporting IS the
	// readiness surface, so a failure must be visible at the default level
	// without logging once per interval while it persists.
	statusPublishFailing degraded.Condition

	// Metrics
	entitiesPublished atomic.Int64
	ingestErrors      atomic.Int64
	lastActivityMu    sync.RWMutex
	lastActivity      time.Time

	// distinct tracks distinct entity IDs for honest status counts
	// (publish counters inflate under periodic reindex — audit 2026-07-19).
	distinct *entitypub.DistinctTracker

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

	pub, err := entitypub.New(deps.NATSClient, deps.GetLogger(),
		// Publish-boundary telemetry, keyed by instance so one stalled source
		// cannot hide behind healthy siblings. Registry is nil in tests.
		entitypub.WithMetrics(deps.MetricsRegistry, config.InstanceName))
	if err != nil {
		return nil, fmt.Errorf("create entity publisher: %w", err)
	}

	c := &Component{
		name:       "url-source",
		config:     config,
		publisher:  pub,
		distinct:   entitypub.NewDistinctTracker(),
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

	// Started before the seed, not after it, so progress is visible while the
	// initial ingest runs. Cancel is registered immediately — the seed can
	// return an error, and a reporter registered only on the success path would
	// leak on that return.
	progressCancel := c.startStatusReporter(ctx)
	c.mu.Lock()
	c.cancelFuncs = append(c.cancelFuncs, progressCancel)
	c.mu.Unlock()

	// The initial ingest is unbounded, so it runs in a supervised goroutine
	// and Start returns. Holding the lifecycle hook across it blocks the
	// framework's component-start barrier, which prevents the HTTP status and
	// metrics listeners from binding at all (semstreams#867).
	c.markRunning()
	c.seed.Start(ctx, c.logger, c.runSeed)
	return nil
}

// markRunning marks the component live. Running means STARTED, not seeded —
// `phase` is what distinguishes those.
func (c *Component) markRunning() {
	c.mu.Lock()
	c.running = true
	c.startTime = time.Now()
	c.mu.Unlock()
}

// runSeed performs the initial ingest and everything sequenced after it. A
// failure surfaces through last_error and a WARN, because there is no
// longer a Start to fail.
func (c *Component) runSeed(ctx context.Context) error {
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
			payload, err := entitypub.PayloadFromState(state)
			if err != nil {
				c.logger.Warn("Invalid URL entity state",
					"url", rawURL,
					"id", state.ID,
					"error", err)
				c.ingestErrors.Add(1)
				continue
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
			c.distinct.Observe(state.ID)
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

// handleChangeEvent processes a typed ChangeEvent from a URL watcher channel.
func (c *Component) handleChangeEvent(ctx context.Context, event handler.ChangeEvent) {
	c.logger.Debug("URL-source change event received",
		"url", event.Path,
		"operation", event.Operation,
		"entity_states", len(event.EntityStates))

	if event.Operation == handler.OperationDelete {
		return
	}

	// Prefer pre-built EntityStates — no normalizer pass required.
	if len(event.EntityStates) > 0 {
		for _, state := range event.EntityStates {
			payload, err := entitypub.PayloadFromState(state)
			if err != nil {
				stateID := ""
				if state != nil {
					stateID = state.ID
				}
				c.logger.Warn("Invalid URL entity state on change",
					"url", event.Path,
					"id", stateID,
					"error", err)
				c.ingestErrors.Add(1)
				continue
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
			c.distinct.Observe(state.ID)
			c.updateLastActivity()
		}
		return
	}

	c.ingestErrors.Add(1)
	c.logger.Warn("URL-source change event missing required EntityStates",
		"url", event.Path,
		"operation", event.Operation)
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

// setPhase records the phase the periodic reporter should publish.
func (c *Component) setPhase(phase string) { c.phase.Store(phase) }

// currentPhase returns the phase to report, defaulting to the seeding phase
// because the reporter now starts before the initial seed completes.
func (c *Component) currentPhase() string {
	if p, ok := c.phase.Load().(string); ok && p != "" {
		return p
	}
	return "ingesting"
}

// publishStatusReport sends a status report to the manifest component via NATS core.
// buildStatusReport assembles this source's status report. It is pure —
// no I/O and no phase mutation — so the field wiring, in particular the
// accepted/delivered/lost delivery figures, is directly assertable.
func (c *Component) buildStatusReport(phase string) sourcestatus.Report {
	return sourcestatus.Report{
		InstanceName: c.config.InstanceName,
		SourceType:   "url",
		Phase:        phase,
		EntityCount:  c.distinct.Count(),
		PublishTotal: c.entitiesPublished.Load(),
		// Delivery figures: acceptance is not arrival. OfferedTotal is what
		// this source handed to the publisher; DeliveredTotal is what the
		// publisher confirmed onto the stream; LostTotal is the difference
		// that never arrived (overflow drops + terminal failures).
		// Offered includes what the publisher refused on overflow: a drop is a
		// loss of an entity this source had, not a non-event. Counting only
		// accepted hand-offs would put drops in LostTotal but not here, and
		// the figures would not reconcile.
		OfferedTotal:   c.entitiesPublished.Load() + c.publisher.Dropped(),
		DeliveredTotal: c.publisher.Published(),
		LostTotal:      c.publisher.Lost(),
		ErrorCount:     c.ingestErrors.Load() + c.publisher.Lost(),
		TypeCounts:     c.distinct.TypeCounts(),
		// Publisher distress: retrying against a refusing transport reports
		// no drops and no errors while being functionally stalled (#188).
		Backpressure: c.publisher.InBackpressure(),
		LastError:    c.seed.LastError(),
		Timestamp:    time.Now(),
	}
}

func (c *Component) publishStatusReport(ctx context.Context, phase string) {
	// Publishing a phase IS the transition, so the reporter's sampled phase can
	// never diverge from the last one published.
	c.setPhase(phase)
	report := c.buildStatusReport(phase)
	data, err := json.Marshal(report)
	if err != nil {
		c.logger.Warn("failed to marshal status report", "error", err)
		return
	}
	if err := c.natsClient.Publish(ctx, "semsource.internal.status", data); err != nil {
		// Status reporting IS the readiness surface: failing silently makes a
		// healthy component look stalled to every consumer, or vice versa.
		// Reported on the transition into failure, not once per interval.
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
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-rCtx.Done():
				return
			case <-ticker.C:
				// Sample the CURRENT phase: one reporter covers the seed and the
				// watch window, so a long seed is observable while it runs
				// instead of publishing status only at its two edges.
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

	// Drain the seed BEFORE taking the lock: the mutex is not reentrant and the
	// seed itself takes it, so waiting under the lock deadlocks. Draining first
	// also keeps shutdown safe — stopping the publisher closes its buffer.
	c.seed.Stop(ctx, c.logger)

	c.mu.Lock()
	defer c.mu.Unlock()

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
