package gitsource

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
	githandler "github.com/c360studio/semsource/handler/git"
	"github.com/c360studio/semsource/internal/entitypub"
)

// gitSourceSchema defines the configuration schema for the git-source component.
var gitSourceSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// sourceCfg is a minimal handler.SourceConfig adapter for the git handler.
// It satisfies the handler.SourceConfig interface without importing the handler
// package directly in this file.
type sourceCfg struct {
	path         string
	repoURL      string
	branch       string
	watchEnabled bool
}

func (s *sourceCfg) GetType() string             { return "git" }
func (s *sourceCfg) GetPath() string             { return s.path }
func (s *sourceCfg) GetPaths() []string          { return nil }
func (s *sourceCfg) GetURL() string              { return s.repoURL }
func (s *sourceCfg) GetBranch() string           { return s.branch }
func (s *sourceCfg) IsWatchEnabled() bool        { return s.watchEnabled }
func (s *sourceCfg) GetKeyframeMode() string     { return "" }
func (s *sourceCfg) GetKeyframeInterval() string { return "" }
func (s *sourceCfg) GetSceneThreshold() float64  { return 0 }

// Component implements the git-source processor.
// It delegates all repository operations to the existing handler/git package,
// which handles local path resolution, remote cloning, commit log walking,
// and change detection via polling.
type Component struct {
	name       string
	config     Config
	publisher  *entitypub.Publisher
	natsClient *natsclient.Client
	logger     *slog.Logger
	platform   component.PlatformMeta

	handler   *githandler.Handler
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

// NewComponent creates a new git-source processor component.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	pollInterval, err := time.ParseDuration(config.PollInterval)
	if err != nil {
		// Validate() already caught malformed durations, but be defensive.
		return nil, fmt.Errorf("parse poll_interval: %w", err)
	}

	h := githandler.New(githandler.Config{
		PollInterval: pollInterval,
		MaxCommits:   config.MaxCommits,
		WorkspaceDir: config.WorkspaceDir,
		Token:        config.GitToken,
		Org:          config.Org,
		BranchSlug:   config.BranchSlug,
	})

	watchEnabled := config.WatchEnabled == nil || *config.WatchEnabled
	sc := &sourceCfg{
		path:         config.RepoPath,
		repoURL:      config.RepoURL,
		branch:       config.Branch,
		watchEnabled: watchEnabled,
	}

	pub, err := entitypub.New(deps.NATSClient, deps.GetLogger())
	if err != nil {
		return nil, fmt.Errorf("create entity publisher: %w", err)
	}

	c := &Component{
		name:       "git-source",
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

// Start performs the initial git ingest, then starts polling for new commits.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	c.mu.Unlock()

	c.publisher.Start(ctx)

	c.publishStatusReport(ctx, "ingesting")

	repoDesc := c.config.RepoPath
	if repoDesc == "" {
		repoDesc = c.config.RepoURL
	}

	c.logger.Info("Starting git-source initial ingest",
		"repo", repoDesc,
		"org", c.config.Org,
		"branch", c.config.Branch)

	if err := c.ingestOnce(ctx); err != nil {
		return fmt.Errorf("initial git ingest failed: %w", err)
	}

	c.logger.Info("Git-source initial ingest complete",
		"repo", repoDesc,
		"entities_published", c.entitiesPublished.Load())

	c.publishStatusReport(ctx, "watching")

	cancel := c.startPolling(ctx)

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

// ingestOnce runs a single ingest pass: calls the git handler to produce
// typed EntityState values with vocabulary-predicate triples, then publishes
// each as an EntityPayload to NATS — no normalizer pass required.
func (c *Component) ingestOnce(ctx context.Context) error {
	states, err := c.handler.IngestEntityStates(ctx, c.sourceCfg, c.config.Org)
	if err != nil {
		c.ingestErrors.Add(1)
		return fmt.Errorf("git handler ingest: %w", err)
	}

	for _, state := range states {
		payload := &graph.EntityPayload{
			ID:         state.ID,
			TripleData: state.Triples,
			UpdatedAt:  state.UpdatedAt,
		}

		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish git entity",
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

// startPolling starts a goroutine that watches for HEAD changes and re-ingests
// on each commit. Returns the cancel func for the polling goroutine, or nil
// if watch setup fails.
func (c *Component) startPolling(ctx context.Context) context.CancelFunc {
	pollCtx, cancel := context.WithCancel(ctx)

	changeCh, err := c.handler.Watch(pollCtx, c.sourceCfg)
	if err != nil {
		c.logger.Warn("Failed to start git polling watcher, skipping watch",
			"repo", c.config.RepoPath,
			"url", c.config.RepoURL,
			"error", err)
		cancel()
		return nil
	}

	if changeCh == nil {
		// Watch returned nil channel — watching disabled or not applicable.
		cancel()
		return nil
	}

	pollInterval, _ := time.ParseDuration(c.config.PollInterval)
	c.logger.Info("Git-source polling started",
		"interval", pollInterval)

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

// handleChangeEvent processes a change event from the git handler's watch channel.
// When the event carries EntityStates (the normalizer-free path), they are
// published directly. This is the expected path for git-source watch events
// when cfg.Org is configured.
func (c *Component) handleChangeEvent(ctx context.Context, event handler.ChangeEvent) {
	c.logger.Debug("Git-source change event received",
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
			c.logger.Warn("Failed to publish git entity on change",
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

// publishEntity enqueues an EntityPayload for buffered delivery to NATS.
// Send is non-blocking; the publisher's circular buffer absorbs backpressure.
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
		SourceType:   "git",
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

	c.logger.Info("Git-source stopped",
		"entities_published", c.entitiesPublished.Load(),
		"ingest_errors", c.ingestErrors.Load())

	return nil
}

// Discoverable interface implementation

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "git-source",
		Type:        "processor",
		Description: "Git repository source for semsource commit, author, and branch entity extraction",
		Version:     "0.1.0",
	}
}

// InputPorts returns an empty slice — git-source generates data from git repositories.
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
	return gitSourceSchema
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
