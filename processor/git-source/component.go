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
	"github.com/c360studio/semstreams/pkg/retry"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	githandler "github.com/c360studio/semsource/handler/git"
	"github.com/c360studio/semsource/internal/degraded"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/seedsup"
	"github.com/c360studio/semsource/workspace"
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
		PollInterval:   pollInterval,
		MaxCommits:     config.MaxCommits,
		WorkspaceDir:   config.WorkspaceDir,
		Token:          config.GitToken,
		Org:            config.Org,
		BranchSlug:     config.BranchSlug,
		SkipSubmodules: config.Submodules != nil && !*config.Submodules,
		Logger:         deps.GetLogger(),
	})

	watchEnabled := config.WatchEnabled == nil || *config.WatchEnabled
	sc := &sourceCfg{
		path:         config.RepoPath,
		repoURL:      config.RepoURL,
		branch:       config.Branch,
		watchEnabled: watchEnabled,
	}

	pub, err := entitypub.New(deps.NATSClient, deps.GetLogger(),
		// Publish-boundary telemetry, keyed by instance so one stalled source
		// cannot hide behind healthy siblings. Registry is nil in tests.
		entitypub.WithMetrics(deps.MetricsRegistry, config.InstanceName))
	if err != nil {
		return nil, fmt.Errorf("create entity publisher: %w", err)
	}

	c := &Component{
		name:       "git-source",
		config:     config,
		publisher:  pub,
		distinct:   entitypub.NewDistinctTracker(),
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

	// Started before the seed, not after it, so progress is visible while the
	// initial ingest runs. Cancel is registered immediately — the seed can
	// return an error, and a reporter registered only on the success path would
	// leak on that return.
	progressCancel := c.startStatusReporter(ctx)
	c.mu.Lock()
	c.cancelFuncs = append(c.cancelFuncs, progressCancel)
	c.mu.Unlock()

	repoDesc := c.config.RepoPath
	if repoDesc == "" {
		repoDesc = c.config.RepoURL
	}

	c.logger.Info("Starting git-source initial ingest",
		"repo", repoDesc,
		"org", c.config.Org,
		"branch", c.config.Branch)

	// Everything from here is unbounded — the path retry alone is ~30
	// attempts — so it runs in a supervised goroutine and Start returns.
	// Holding the lifecycle hook across it blocks the framework's
	// component-start barrier, which prevents the HTTP status and metrics
	// listeners from binding at all (semstreams#867).
	c.markRunning()
	c.seed.Start(ctx, c.logger, c.runSeed)
	return nil
}

// markRunning marks the component live. Running means STARTED, not
// seeded — `phase` is what distinguishes those.
func (c *Component) markRunning() {
	c.mu.Lock()
	c.running = true
	c.startTime = time.Now()
	c.mu.Unlock()
}

// runSeed performs the initial ingest and everything sequenced after it.
// It runs in its own goroutine; a failure surfaces through the source's
// last_error and a WARN, because there is no longer a Start to fail.
func (c *Component) runSeed(ctx context.Context) error {
	repoDesc := c.config.RepoPath
	if repoDesc == "" {
		repoDesc = c.config.RepoURL
	}

	// Retry initial ingest — the repo filesystem may not be ready yet if
	// a Docker volume mount is still settling or a clone is in progress.
	// retry.Do swallows interim errors, so we log each failed attempt at WARN
	// so operators can see why seeding is taking time instead of staring at a
	// silent status counter ticking up.
	var attempt atomic.Int32
	if err := retry.Do(ctx, retry.Persistent(), func() error {
		n := attempt.Add(1)
		if c.config.RepoPath != "" {
			if err := workspace.IsRepoReady(c.config.RepoPath); err != nil {
				c.logger.Warn("git-source: repo not ready — retrying",
					"repo", c.config.RepoPath,
					"attempt", n,
					"error", err)
				return err
			}
		}
		if err := c.ingestOnce(ctx); err != nil {
			c.logger.Warn("git-source: initial ingest attempt failed — retrying",
				"repo", repoDesc,
				"attempt", n,
				"error", err)
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("initial git ingest failed after %d attempts: %w", attempt.Load(), err)
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
	// The status reporter is already running (started before the seed).
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
		payload, err := entitypub.PayloadFromState(state)
		if err != nil {
			c.logger.Warn("Invalid git entity state",
				"id", state.ID,
				"error", err)
			c.ingestErrors.Add(1)
			continue
		}

		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish git entity",
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
		payload, err := entitypub.PayloadFromState(state)
		if err != nil {
			c.logger.Warn("Invalid git entity state on change",
				"id", state.ID,
				"error", err)
			c.ingestErrors.Add(1)
			continue
		}

		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish git entity on change",
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

// publishEntity enqueues an EntityPayload for buffered delivery to NATS.
// Send is non-blocking; the publisher's circular buffer absorbs backpressure.
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

// submoduleState is one declared submodule path on the status report. JSON
// mirrors source-manifest's SubmoduleStatus (reports are duck-typed).
type submoduleState struct {
	Path  string `json:"path"`
	SHA   string `json:"sha,omitempty"`
	State string `json:"state"`
}

// submoduleStates classifies the handler's latest probe for the status
// report. States: materialized, unmaterialized (declared and pinned but the
// tree is empty — the silently-missing-code case), excluded_by_config
// (submodules: false), declared_but_absent (stale .gitmodules entry with no
// gitlink), beyond_cap (nesting deeper than the inventory cap).
func (c *Component) submoduleStates() []submoduleState {
	return classifySubmodules(c.handler.SubmoduleInventory(),
		c.config.Submodules != nil && !*c.config.Submodules)
}

// classifySubmodules maps a probe inventory to status states. skip reflects
// the submodules:false opt-out, which turns unmaterialized into
// excluded_by_config — deliberate absence, distinguishable from unexpected.
func classifySubmodules(inv *workspace.SubmoduleInventory, skip bool) []submoduleState {
	if inv == nil {
		return nil
	}
	var out []submoduleState
	for _, s := range inv.Submodules {
		st := submoduleState{Path: s.Path}
		if len(s.SHA) >= 12 {
			st.SHA = s.SHA[:12]
		}
		switch {
		case s.Materialized:
			st.State = "materialized"
		case s.SHA == "":
			st.State = "declared_but_absent"
		case skip:
			st.State = "excluded_by_config"
		default:
			st.State = "unmaterialized"
		}
		out = append(out, st)
	}
	for _, p := range inv.BeyondCap {
		out = append(out, submoduleState{Path: p, State: "beyond_cap"})
	}
	return out
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
		Submodules   []submoduleState `json:"submodules,omitempty"`
		LastError    *seedsup.Error   `json:"last_error,omitempty"`
		Timestamp    time.Time        `json:"timestamp"`
	}{
		InstanceName: c.config.InstanceName,
		SourceType:   "git",
		Phase:        phase,
		EntityCount:  c.distinct.Count(),
		PublishTotal: c.entitiesPublished.Load(),
		ErrorCount:   c.ingestErrors.Load() + c.handler.WatchErrorCount() + c.publisher.Lost(),
		TypeCounts:   c.distinct.TypeCounts(),
		// The no-silent-entity-loss posture applied to inputs: every declared
		// submodule path and its state, so missing trees are visible on every
		// status surface instead of silently absent from the graph.
		Submodules: c.submoduleStates(),
		LastError:  c.seed.LastError(),
		Timestamp:  time.Now(),
	}
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

	// Drain the seed BEFORE taking the lock: the mutex is not reentrant and
	// the seed itself takes it, so waiting under the lock deadlocks. Draining
	// first also keeps shutdown safe — stopping the publisher closes its
	// buffer, and a live seed would publish into a closed one.
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
