package cfgfilesource

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourceauthority"
	"github.com/c360studio/semsource/internal/workerjoin"
	"log/slog"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/retry"
	"github.com/c360studio/semengine/types"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	cfghandler "github.com/c360studio/semsource/handler/cfgfile"
	"github.com/c360studio/semsource/internal/degraded"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/seedloss"
	"github.com/c360studio/semsource/internal/seedsup"
	"github.com/c360studio/semsource/internal/sourcestatus"
	"github.com/c360studio/semsource/workspace"
)

// lifecycleTriggerTimeout bounds the background NATS round trip triggering a
// staleness lifecycle pass, so a stalled responder cannot hold Stop's join.
const lifecycleTriggerTimeout = 30 * time.Second

// cfgfileSourceSchema defines the configuration schema for the cfgfile-source component.
var cfgfileSourceSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// sourceCfg is a minimal handler.SourceConfig adapter that satisfies the
// handler.SourceConfig interface using paths and watch settings from Config.
type sourceCfg struct {
	paths        []string
	watchEnabled bool
	coalesceMs   int
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
func (s *sourceCfg) GetCoalesceMs() int          { return s.coalesceMs }

// Component implements the cfgfile-source processor.
// It delegates all file discovery, parsing, and watching to the existing
// handler/cfgfile package and publishes EntityPayload messages with
// vocabulary-predicate triples directly to NATS JetStream — no normalizer pass.
type Component struct {
	authority entityid.Authority
	name      string
	config    Config
	publisher *entitypub.Publisher
	// seedLoss attributes publisher loss to one seed pass; the publisher's
	// own counters are monotonic and so can never clear.
	seedLoss   seedloss.Tracker
	natsClient *natsclient.Client
	logger     *slog.Logger
	platform   types.PlatformMeta

	handler   *cfghandler.ConfigHandler
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
	// pathsUnavailable: a source that cannot reach its configured paths seeds
	// nothing, silently, for the whole retry window.
	pathsUnavailable degraded.Condition
	// lifecycleFailing: staleness marking has stopped working.
	lifecycleFailing degraded.Condition

	// Metrics
	entitiesPublished atomic.Int64
	ingestErrors      atomic.Int64
	lastActivityMu    sync.RWMutex
	lastActivity      time.Time

	// distinct tracks distinct entity IDs for honest status counts
	// (publish counters inflate under periodic reindex — audit 2026-07-19).
	distinct *entitypub.DistinctTracker

	// sent gates the #336 reconcile to republishes this process knows changed.
	sent sentContent

	// Background goroutine cancellation
	cancelFuncs []context.CancelFunc
	workers     workerjoin.Group
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
	authority, err := sourceauthority.Resolve(deps.Platform, config.Org)
	if err != nil {
		return nil, err
	}

	// Pass the org into the handler so Watch events also carry EntityStates.
	h := cfghandler.New(&cfghandler.Config{Authority: authority, Project: config.Project})
	sc := &sourceCfg{
		paths:        config.Paths,
		watchEnabled: config.WatchEnabled,
		coalesceMs:   config.CoalesceMs,
	}

	pubOpts := []entitypub.Option{
		// Publish-boundary telemetry, keyed by instance so one stalled source
		// cannot hide behind healthy siblings. Registry is nil in tests.
		entitypub.WithMetrics(deps.MetricsRegistry, config.InstanceName),
	}
	if deps.NATSClient != nil {
		// Every cfgfile publish carries the entity's complete semsource set:
		// IDs are scoped to the declaring manifest (#348) and a manifest's
		// same-ID declarations fold into one entity (#375), and a watch event
		// re-parses the whole manifest (TestFanOutCarriesTheManifestsCompleteStates).
		// Two paths minting under one system could still share a manifest's
		// ID; sourcespawn refuses that config before it reaches here (#349).
		// publishEntity limits the reconcile to known content changes (#336, D4).
		reconciler, err := entitypub.NewSourceReconciler(deps.NATSClient)
		if err != nil {
			return nil, fmt.Errorf("create source reconciler: %w", err)
		}
		pubOpts = append(pubOpts, entitypub.WithReconcile(reconciler))
	}
	pub, err := entitypub.New(deps.NATSClient, deps.GetLogger(), pubOpts...)
	if err != nil {
		return nil, fmt.Errorf("create entity publisher: %w", err)
	}

	c := &Component{
		authority:  authority,
		name:       "cfgfile-source",
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

// Start performs the initial config file ingest across all configured paths,
// then optionally starts the filesystem watcher for real-time change detection.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	c.mu.Unlock()

	runCtx := context.WithoutCancel(ctx)
	c.publisher.Start(runCtx)

	c.seedLoss.Begin(c.publisher.Lost())
	c.publishStatusReport(ctx, "ingesting")

	// Started before the seed, not after it, so progress is visible while the
	// initial ingest runs. Cancel is registered immediately — the seed can
	// return an error, and a reporter registered only on the success path would
	// leak on that return.
	progressCancel := c.startStatusReporter(runCtx)
	c.mu.Lock()
	c.cancelFuncs = append(c.cancelFuncs, progressCancel)
	c.mu.Unlock()

	c.logger.Info("Starting cfgfile-source initial ingest",
		"paths", c.config.Paths,
		"org", c.config.Org,
		"watch_enabled", c.config.WatchEnabled)

	// Everything from here is unbounded — the path retry alone is ~30
	// attempts — so it runs in a supervised goroutine and Start returns.
	// Holding the lifecycle hook across it blocks the framework's
	// component-start barrier, which prevents the HTTP status and metrics
	// listeners from binding at all (semstreams#867).
	c.markRunning()
	c.seed.Start(runCtx, c.logger, c.runSeed)
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
	// Retry initial ingest — paths may not exist yet if a git clone is still
	// in progress (repo expansion pattern). We check IsPathReady rather than
	// just os.Stat because git clone creates the directory before populating it.
	if err := retry.Do(ctx, retry.Persistent(), func() error {
		for _, p := range c.config.Paths {
			if err := workspace.IsPathReady(p); err != nil {
				// Bounded by retry.Persistent (~30 attempts), but a source that
				// never reaches its paths seeds nothing at all — surface it once
				// rather than leaving the whole wait at Debug.
				c.pathsUnavailable.Enter(c.logger,
					"source paths are not available yet — seeding cannot start",
					"path", p, "error", err)
				return err
			}
		}
		return c.ingestOnce(ctx)
	}); err != nil {
		return fmt.Errorf("initial cfgfile ingest failed: %w", err)
	}
	c.pathsUnavailable.Clear(c.logger, "source paths became available")

	c.logger.Info("Cfgfile-source initial ingest complete",
		"paths", c.config.Paths,
		"entities_published", c.entitiesPublished.Load())

	c.publishStatusReport(ctx, "watching")
	c.reportWhenSettled(ctx)

	cancel := c.startWatching(ctx)

	c.mu.Lock()
	if cancel != nil {
		c.cancelFuncs = append(c.cancelFuncs, cancel)
	}
	// The status reporter is already running (started before the seed).
	c.mu.Unlock()

	return nil
}

// ingestOnce runs a single ingest pass using IngestEntityStates — the
// normalizer-free path that builds vocabulary-predicate triples directly.
func (c *Component) ingestOnce(ctx context.Context) error {
	states, err := c.handler.IngestEntityStates(ctx, c.sourceCfg, c.authority)
	if err != nil {
		c.ingestErrors.Add(1)
		return fmt.Errorf("cfgfile handler ingest: %w", err)
	}

	for _, state := range states {
		c.stampOwner(state)
		payload, err := entitypub.PayloadFromState(state)
		if err != nil {
			c.logger.Warn("Invalid cfgfile entity state",
				"id", state.ID,
				"error", err)
			c.ingestErrors.Add(1)
			continue
		}
		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish cfgfile entity",
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

// startWatching starts a goroutine that receives fsnotify change events from
// the cfgfile handler and re-publishes updated entity payloads. Returns the
// cancel func for the watching goroutine, or nil if watch setup fails or
// watching is disabled.
func (c *Component) startWatching(ctx context.Context) context.CancelFunc {
	watchCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))

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

	c.workers.JoinOnStop(func(joinCtx context.Context) error { return workerjoin.Channel(joinCtx, changeCh) })
	c.workers.Go(func() {
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
	})

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

	// A deleted manifest's entities stay in the graph; the staleness pass
	// marks them once it finds the manifest gone, and a create (a re-created
	// manifest, or an editor's save by rename) clears that mark, since its
	// same-content republish does not reconcile (#393). A plain modify needs
	// no pass: the manifest is still there.
	if event.Operation == handler.OperationDelete || event.Operation == handler.OperationCreate {
		for _, root := range c.rootsForPath(event.Path) {
			c.triggerLifecycleRun(ctx, root, graph.LifecycleReasonFileDeleted)
		}
	}
	if event.Operation == handler.OperationDelete {
		return
	}

	for _, state := range event.EntityStates {
		c.stampOwner(state)
		payload, err := entitypub.PayloadFromState(state)
		if err != nil {
			c.logger.Warn("Invalid cfgfile entity state on change",
				"id", state.ID,
				"error", err)
			c.ingestErrors.Add(1)
			continue
		}
		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish cfgfile entity on change",
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

// publishEntity enqueues an EntityPayload for buffered publishing via the
// entity publisher. Non-blocking; the publisher handles NATS delivery. It is
// reconciled only when this process already sent the entity with different
// statements, so the first pass after a start never reconciles (#336, D4).
func (c *Component) publishEntity(_ context.Context, payload *graph.EntityPayload) error {
	sum := fingerprint(payload.TripleData)
	payload.SkipReconcile = !c.sent.differs(payload.ID, sum)
	if err := c.publisher.Send(payload); err != nil {
		return err
	}
	c.sent.record(payload.ID, sum)
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
		SourceType:   "config",
		Phase:        phase,
		EntityCount:  c.distinct.Count(),
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
		SeedLost:       c.seedLoss.LostSince(c.publisher.Lost()),
		ErrorCount:     c.ingestErrors.Load() + c.publisher.Lost(),
		TypeCounts:     c.distinct.TypeCounts(),
		// Publisher distress: retrying against a refusing transport reports
		// no drops and no errors while being functionally stalled (#188).
		Backpressure: c.publisher.InBackpressure(),
		LastError:    c.seed.LastError(),
		Timestamp:    time.Now(),
	}
}

// reportWhenSettled publishes one more status report once the publisher has
// settled everything the seed handed it. The seed-end report is built straight
// after the last Send, so it can carry delivered < offered; without this the
// figures correct only on the periodic tick (#284). ctx is the seed context,
// which Stop cancels before it joins the workers.
//
// Once settled, a watching source also sweeps every root, so a manifest deleted while this
// process was down is marked and one re-created is cleared (#393).
func (c *Component) reportWhenSettled(ctx context.Context) {
	c.workers.Go(func() {
		if c.publisher.WaitSettled(ctx) != nil {
			return
		}
		c.publishStatusReport(ctx, c.currentPhase())
		for _, root := range c.seedSweepRoots() {
			c.runLifecycle(ctx, root, graph.LifecycleReasonPathMissing)
		}
	})
}

// seedSweepRoots are the roots the settled seed sweeps: none for a watch:false
// source, a frozen snapshot that never goes stale (staleness D5).
func (c *Component) seedSweepRoots() []string {
	if !c.config.WatchEnabled {
		return nil
	}
	return c.config.Paths
}

// ownerFor is the lifecycle owner key of one configured root, keyed by its
// absolute form so the stamp and the sweep agree.
func (c *Component) ownerFor(root string) string {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	instance := c.config.InstanceName
	if instance == "" {
		instance = "cfgfile-source"
	}
	return graph.LifecycleOwner(instance, root)
}

// stampOwner writes the owner fact of the root an entity came from, so only
// that root's sweep judges it (#286, #393).
func (c *Component) stampOwner(state *handler.EntityState) {
	if state == nil || state.Root == "" {
		return
	}
	state.Triples = append(state.Triples, graph.LifecycleOwnerTriple(state.ID, c.ownerFor(state.Root), state.UpdatedAt))
}

// rootsForPath returns every distinct configured root that contains path, as
// configured, since the system slug is derived from the configured form.
func (c *Component) rootsForPath(path string) []string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	var roots []string
	seen := make(map[string]bool, len(c.config.Paths))
	for _, p := range c.config.Paths {
		absRoot, err := filepath.Abs(p)
		if err != nil || seen[absRoot] {
			continue
		}
		// Rel, not a prefix test: a "/" root would need the prefix "//".
		if rel, err := filepath.Rel(absRoot, path); err == nil && rel != ".." &&
			!strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			seen[absRoot] = true
			roots = append(roots, p)
		}
	}
	return roots
}

// lifecycleRequest scopes a staleness pass to one root: the system its
// entities carry and the owner they were stamped with.
func (c *Component) lifecycleRequest(root, reason string) graph.LifecycleRunRequest {
	absRoot := root
	if abs, err := filepath.Abs(root); err == nil {
		absRoot = abs
	}
	return graph.LifecycleRunRequest{
		Org:      c.authority.Org,
		Systems:  []string{cfghandler.SystemSlug(c.config.Project, root)},
		RootPath: absRoot,
		Reason:   reason,
		Owner:    c.ownerFor(root),
	}
}

// triggerLifecycleRun runs the pass for root in the background so the watch
// loop never blocks on a full graph pass; Stop joins it.
func (c *Component) triggerLifecycleRun(ctx context.Context, root, reason string) {
	c.workers.Go(func() { c.runLifecycle(ctx, root, reason) })
}

// runLifecycle sends one lifecycle request and reports a failure on the
// transition into it (ADR-0011): when it fails, staleness marking stops.
func (c *Component) runLifecycle(ctx context.Context, root, reason string) {
	runCtx, cancel := context.WithTimeout(ctx, lifecycleTriggerTimeout)
	defer cancel()
	if _, err := graph.PublishLifecycleTrigger(runCtx, c.natsClient, c.lifecycleRequest(root, reason)); err != nil {
		c.lifecycleFailing.Enter(c.logger,
			"staleness lifecycle trigger is failing — retractions may linger",
			"root", root, "reason", reason, "error", err)
		return
	}
	c.lifecycleFailing.Clear(c.logger, "staleness lifecycle trigger recovered")
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
	rCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c.workers.Go(func() {
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
	})
	return cancel
}

// Stop gracefully stops the component; ctx bounds the seed join and cleanup.
func (c *Component) Stop(ctx context.Context) error {
	c.mu.RLock()
	running := c.running
	c.mu.RUnlock()
	if !running {
		return nil
	}
	c.seed.Stop(ctx, c.logger)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("join seed: %w", err)
	}
	c.mu.Lock()
	cancels := c.cancelFuncs
	c.cancelFuncs = nil
	c.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}

	if err := c.workers.Wait(ctx); err != nil {
		return fmt.Errorf("join source workers: %w", err)
	}
	if err := c.publisher.Stop(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
	return nil
}

// Discoverable interface implementation

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
