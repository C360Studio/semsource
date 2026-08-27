package objectstoresource

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
	dochandler "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/handler/objectstore"
	"github.com/c360studio/semsource/internal/degraded"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/seedloss"
	"github.com/c360studio/semsource/internal/seedsup"
	"github.com/c360studio/semsource/internal/sourcestatus"
	"github.com/c360studio/semsource/storage/filestore"
	"github.com/c360studio/semsource/storage/s3store"
)

// objectStoreSourceSchema defines the configuration schema for the component.
var objectStoreSourceSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// lifecycleTriggerTimeout bounds one request/reply round trip to the staleness
// lifecycle pass. Generous: it is fired off a background goroutine, never a
// caller's synchronous path, and a full pass may enumerate many entities.
const lifecycleTriggerTimeout = 30 * time.Second

// sourceCfg is a minimal handler.SourceConfig adapter for the object-store
// handler. The bucket and prefix ride in the URL, which is what the existing
// SourceConfig interface already carries for sources that are not filesystem
// paths.
type sourceCfg struct {
	url          string
	watchEnabled bool
}

func (s *sourceCfg) GetType() string             { return objectstore.SourceType }
func (s *sourceCfg) GetPath() string             { return "" }
func (s *sourceCfg) GetPaths() []string          { return nil }
func (s *sourceCfg) GetURL() string              { return s.url }
func (s *sourceCfg) GetBranch() string           { return "" }
func (s *sourceCfg) IsWatchEnabled() bool        { return s.watchEnabled }
func (s *sourceCfg) GetKeyframeMode() string     { return "" }
func (s *sourceCfg) GetKeyframeInterval() string { return "" }
func (s *sourceCfg) GetSceneThreshold() float64  { return 0 }

// Component implements the objectstore-source processor.
//
// Enumeration, change detection, and skip accounting live in
// handler/objectstore; this component owns the lifecycle, publication, status,
// and retraction around them.
type Component struct {
	name      string
	config    Config
	publisher *entitypub.Publisher
	// seedLoss attributes publisher loss to one seed pass; the publisher's
	// own counters are monotonic and so can never clear.
	seedLoss   seedloss.Tracker
	natsClient *natsclient.Client
	logger     *slog.Logger
	platform   component.PlatformMeta

	handler   *objectstore.Handler
	sourceCfg *sourceCfg
	// system is the entity-ID system segment these documents carry, resolved
	// once: the bucket does not change, and the retraction path must scope by
	// exactly the slug the ingest path published under.
	system string

	// Lifecycle
	running   bool
	startTime time.Time
	mu        sync.RWMutex

	// seed supervises the asynchronous initial ingest (internal/seedsup):
	// Start must return promptly or the framework's component-start barrier
	// keeps the HTTP and metrics listeners from binding (semstreams#867).
	seed seedsup.Supervisor

	// phase is the lifecycle phase the periodic reporter publishes; atomic
	// because the reporter goroutine samples it while Start mutates it.
	phase atomic.Value
	// statusPublishFailing is edge-triggered (ADR-0011): status reporting IS
	// the readiness surface, so a failure must be visible at the default level
	// without logging once per interval while it persists.
	statusPublishFailing degraded.Condition
	// bucketUnreachable: a source that cannot list its prefix ingests nothing,
	// and without this it does so silently for the whole retry window.
	bucketUnreachable degraded.Condition
	// lifecycleFailing: staleness marking has stopped working.
	lifecycleFailing degraded.Condition

	// Metrics
	entitiesPublished atomic.Int64
	ingestErrors      atomic.Int64
	lastActivityMu    sync.RWMutex
	lastActivity      time.Time

	// skipped holds the skip counts of the LAST COMPLETED PASS, not a running
	// total. A skip repeats every pass by nature — an unsupported extension
	// stays unsupported — so a lifetime counter would climb forever while
	// describing the same few objects.
	skippedMu sync.RWMutex
	skipped   map[string]int64

	// distinct tracks distinct entity IDs for honest status counts
	// (publish counters inflate under periodic reindex).
	distinct *entitypub.DistinctTracker

	// Background goroutine cancellation
	cancelFuncs []context.CancelFunc
}

// NewComponent creates a new objectstore-source processor component.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Credentials are read here, from the environment, and nowhere else.
	store, err := s3store.New(config.StoreConfig())
	if err != nil {
		return nil, fmt.Errorf("create object store: %w", err)
	}

	pub, err := entitypub.New(deps.NATSClient, deps.GetLogger(),
		// Publish-boundary telemetry, keyed by instance so one stalled source
		// cannot hide behind healthy siblings. Registry is nil in tests.
		entitypub.WithMetrics(deps.MetricsRegistry, config.InstanceName))
	if err != nil {
		return nil, fmt.Errorf("create entity publisher: %w", err)
	}

	docOpts := []dochandler.Option{dochandler.WithProject(config.Project)}
	if config.BodyStoreRoot != "" {
		bodies, storeErr := filestore.New(config.BodyStoreRoot, true)
		if storeErr != nil {
			return nil, fmt.Errorf("create body store: %w", storeErr)
		}
		docOpts = append(docOpts, dochandler.WithBodyStore(bodies, config.InstanceName))
	}

	h := objectstore.New(store, dochandler.New(docOpts...), config.Org,
		objectstore.WithProject(config.Project),
		objectstore.WithVersion(config.Version),
		objectstore.WithPollInterval(config.PollInterval()))

	c := &Component{
		name:       "objectstore-source",
		config:     config,
		publisher:  pub,
		distinct:   entitypub.NewDistinctTracker(),
		natsClient: deps.NATSClient,
		logger:     deps.GetLogger(),
		platform:   deps.Platform,
		handler:    h,
		system:     h.System(config.Bucket),
		sourceCfg: &sourceCfg{
			url:          objectstore.SourceURL(config.Bucket, config.Prefix),
			watchEnabled: config.WatchEnabled,
		},
	}
	return c, nil
}

// Initialize prepares the component (no-op; preparation happens in
// NewComponent).
func (c *Component) Initialize() error {
	return nil
}

// Start performs the initial ingest of the configured prefix, then keeps
// re-listing it if watching is enabled.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	c.mu.Unlock()

	c.publisher.Start(ctx)

	c.seedLoss.Begin(c.publisher.Lost())
	c.publishStatusReport(ctx, "ingesting")

	// Started before the seed, not after it, so progress is visible while the
	// initial ingest runs. Cancel is registered immediately — the seed can
	// return an error, and a reporter registered only on the success path
	// would leak on that return.
	progressCancel := c.startStatusReporter(ctx)
	c.mu.Lock()
	c.cancelFuncs = append(c.cancelFuncs, progressCancel)
	c.mu.Unlock()

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
// failure surfaces through last_error and a WARN, because there is no longer a
// Start to fail.
func (c *Component) runSeed(ctx context.Context) error {
	c.logger.Info("Starting objectstore-source initial ingest",
		"bucket", c.config.Bucket,
		"prefix", c.config.Prefix,
		"org", c.config.Org)

	if err := c.ingestOnce(ctx); err != nil {
		return fmt.Errorf("initial object-store ingest failed: %w", err)
	}

	c.logger.Info("Objectstore-source initial ingest complete",
		"entities_published", c.entitiesPublished.Load(),
		"tracked_objects", c.handler.Tracked())

	if !c.config.WatchEnabled {
		c.publishStatusReport(ctx, "ready")
		return nil
	}

	c.publishStatusReport(ctx, "watching")

	cancel := c.startPolling(ctx)
	if cancel != nil {
		c.mu.Lock()
		c.cancelFuncs = append(c.cancelFuncs, cancel)
		c.mu.Unlock()
	}
	return nil
}

// ingestOnce runs one enumeration pass and publishes what it produced.
//
// A pass that could not complete its listing returns an error and changes
// nothing: no entity is published and, crucially, nothing is retracted. That
// is the whole retraction-safety contract — a transient listing failure and a
// legitimately emptied prefix look identical from the outside, and only one of
// them is an answer.
func (c *Component) ingestOnce(ctx context.Context) error {
	result, err := c.handler.IngestEntityStates(ctx, c.sourceCfg)
	if err != nil {
		c.ingestErrors.Add(1)
		c.bucketUnreachable.Enter(c.logger,
			"object-store listing is failing — nothing is being ingested and nothing will be retracted",
			"bucket", c.config.Bucket, "prefix", c.config.Prefix, "error", err)
		return err
	}
	c.bucketUnreachable.Clear(c.logger, "object-store listing recovered")

	c.recordSkips(result)

	for _, ingested := range result.Ingested {
		c.publishDocument(ctx, ingested.Key, ingested.States)
	}
	c.retract(ctx, result.Removed)
	return nil
}

// publishDocument publishes one object's entity states.
//
// A document with no valid states publishes nothing and is counted as an
// error: this source emits canonical typed state, so state that will not
// convert is a contract failure, not a reason to fall back to something else.
func (c *Component) publishDocument(ctx context.Context, key string, states []*handler.EntityState) {
	if len(states) == 0 {
		c.ingestErrors.Add(1)
		c.logger.Warn("Objectstore-source produced no entity states for an object",
			"bucket", c.config.Bucket, "key", key)
		return
	}

	for _, state := range states {
		payload, err := entitypub.PayloadFromState(state)
		if err != nil {
			stateID := ""
			if state != nil {
				stateID = state.ID
			}
			c.logger.Warn("Invalid object-store entity state",
				"key", key, "id", stateID, "error", err)
			c.ingestErrors.Add(1)
			continue
		}
		if err := c.publishEntity(ctx, payload); err != nil {
			c.logger.Warn("Failed to publish object-store entity",
				"key", key, "id", state.ID, "error", err)
			c.ingestErrors.Add(1)
			continue
		}
		c.entitiesPublished.Add(1)
		c.distinct.Observe(state.ID)
		c.updateLastActivity()
	}
}

// retractionRequest builds the lifecycle request for a completed pass's
// removals.
//
// The absent set, not a filesystem root: object keys are not files, and the
// pass's stat check has nothing to stat. Supersession groups by the path
// predicate, so naming the key reaches the document AND its passages — which
// is why this rides the existing pass rather than marking entities here.
//
// Pure, so what goes on the wire is directly assertable. Returns false when
// there is nothing to retract, which is not the same as an empty absent set:
// an empty set would assert "nothing is gone" and clear markers, and this
// source has no business making that claim from a pass it did not run.
func (c *Component) retractionRequest(removed []objectstore.Removal) (graph.LifecycleRunRequest, bool) {
	if len(removed) == 0 {
		return graph.LifecycleRunRequest{}, false
	}

	absent := make([]string, 0, len(removed))
	for _, removal := range removed {
		absent = append(absent, removal.Key)
	}

	return graph.LifecycleRunRequest{
		Org:     c.config.Org,
		Systems: []string{c.system},
		Absent:  absent,
		Reason:  graph.LifecycleReasonFileDeleted,
	}, true
}

// retract announces the objects a completed pass no longer observed to the
// staleness lifecycle pass. Called only for a pass that completed — a failed
// pass returns before this.
func (c *Component) retract(ctx context.Context, removed []objectstore.Removal) {
	req, hasWork := c.retractionRequest(removed)
	if !hasWork {
		return
	}

	c.logger.Info("Objectstore-source retracting removed objects",
		"bucket", c.config.Bucket, "keys", len(req.Absent))

	// The keys are forgotten regardless of whether the trigger lands. A
	// retraction that failed is re-derived by the next pass that still does
	// not see the object — the lifecycle pass is convergent, so a missed
	// trigger costs latency, not correctness. Holding the keys instead would
	// re-announce the same removals on every pass forever.
	for _, removal := range removed {
		c.handler.Forget(removal.Key)
	}

	go func() {
		runCtx, cancel := context.WithTimeout(ctx, lifecycleTriggerTimeout)
		defer cancel()
		if _, err := graph.PublishLifecycleTrigger(runCtx, c.natsClient, req); err != nil {
			// Edge-triggered (ADR-0011): when this fails, staleness marking
			// stops and retracted documents linger as live facts.
			c.lifecycleFailing.Enter(c.logger,
				"staleness lifecycle trigger is failing — retractions may linger",
				"bucket", c.config.Bucket, "error", err)
			return
		}
		c.lifecycleFailing.Clear(c.logger, "staleness lifecycle trigger recovered")
	}()
}

// startPolling starts the watch goroutine and returns its cancel func, or nil
// when watching is off or could not start.
func (c *Component) startPolling(ctx context.Context) context.CancelFunc {
	pollCtx, cancel := context.WithCancel(ctx)

	changeCh, err := c.handler.Watch(pollCtx, c.sourceCfg)
	if err != nil {
		c.logger.Warn("Failed to start object-store watcher, skipping watch",
			"bucket", c.config.Bucket, "error", err)
		cancel()
		return nil
	}
	if changeCh == nil {
		cancel()
		return nil
	}

	c.logger.Info("Objectstore-source polling started",
		"bucket", c.config.Bucket,
		"prefix", c.config.Prefix,
		"poll_interval", c.config.PollInterval())

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

// handleChangeEvent processes one typed change event from the watch channel.
func (c *Component) handleChangeEvent(ctx context.Context, event handler.ChangeEvent) {
	c.logger.Debug("Objectstore-source change event received",
		"key", event.Path,
		"operation", event.Operation,
		"entity_states", len(event.EntityStates))

	if event.Operation == handler.OperationDelete {
		c.retract(ctx, []objectstore.Removal{{Key: event.Path}})
		return
	}

	if len(event.EntityStates) == 0 {
		// A non-delete event with no typed state is a contract violation: it
		// publishes nothing and is counted, rather than falling back to a
		// normalizer pass this source does not use.
		c.ingestErrors.Add(1)
		c.logger.Warn("Objectstore-source change event missing required EntityStates",
			"key", event.Path, "operation", event.Operation)
		return
	}
	c.publishDocument(ctx, event.Path, event.EntityStates)
}

// publishEntity enqueues an EntityPayload for buffered publishing.
func (c *Component) publishEntity(_ context.Context, payload *graph.EntityPayload) error {
	return c.publisher.Send(payload)
}

// recordSkips replaces the skip counts with this pass's.
func (c *Component) recordSkips(result *objectstore.Result) {
	counts := result.SkipCounts()

	c.skippedMu.Lock()
	c.skipped = counts
	c.skippedMu.Unlock()
}

// skipCounts returns a copy of the last completed pass's skip counts.
func (c *Component) skipCounts() map[string]int64 {
	c.skippedMu.RLock()
	defer c.skippedMu.RUnlock()

	if len(c.skipped) == 0 {
		return nil
	}
	out := make(map[string]int64, len(c.skipped))
	for reason, count := range c.skipped {
		out[reason] = count
	}
	return out
}

// updateLastActivity safely updates the last activity timestamp.
func (c *Component) updateLastActivity() {
	c.lastActivityMu.Lock()
	c.lastActivity = time.Now()
	c.lastActivityMu.Unlock()
}

func (c *Component) getLastActivity() time.Time {
	c.lastActivityMu.RLock()
	defer c.lastActivityMu.RUnlock()
	return c.lastActivity
}

func (c *Component) setPhase(phase string) { c.phase.Store(phase) }

// currentPhase returns the phase to report, defaulting to the seeding phase
// because the reporter starts before the initial ingest completes.
func (c *Component) currentPhase() string {
	if p, ok := c.phase.Load().(string); ok && p != "" {
		return p
	}
	return "ingesting"
}

// buildStatusReport assembles this source's status report. It is pure — no I/O
// and no phase mutation — so the field wiring is directly assertable.
func (c *Component) buildStatusReport(phase string) sourcestatus.Report {
	return sourcestatus.Report{
		InstanceName: c.config.InstanceName,
		SourceType:   objectstore.SourceType,
		Phase:        phase,
		EntityCount:  c.distinct.Count(),
		// Delivery figures: acceptance is not arrival. Offered includes what
		// the publisher refused on overflow — a drop is the loss of an entity
		// this source had, so it belongs on both sides of the arithmetic.
		OfferedTotal:   c.entitiesPublished.Load() + c.publisher.Dropped(),
		DeliveredTotal: c.publisher.Published(),
		LostTotal:      c.publisher.Lost(),
		SeedLost:       c.seedLoss.LostSince(c.publisher.Lost()),
		ErrorCount:     c.ingestErrors.Load() + c.publisher.Lost(),
		TypeCounts:     c.distinct.TypeCounts(),
		// The count an operator needs to tell "there is no such document"
		// apart from "that document was never parsed".
		ObjectsSkipped: c.skipCounts(),
		// Publisher distress: retrying against a refusing transport reports no
		// drops and no errors while being functionally stalled (#188).
		Backpressure: c.publisher.InBackpressure(),
		LastError:    c.seed.LastError(),
		Timestamp:    time.Now(),
	}
}

func (c *Component) publishStatusReport(ctx context.Context, phase string) {
	// Publishing a phase IS the transition, so the reporter's sampled phase
	// can never diverge from the last one published.
	c.setPhase(phase)
	report := c.buildStatusReport(phase)
	data, err := json.Marshal(report)
	if err != nil {
		c.logger.Warn("failed to marshal status report", "error", err)
		return
	}
	if err := c.natsClient.Publish(ctx, "semsource.internal.status", data); err != nil {
		c.statusPublishFailing.Enter(c.logger,
			"status reporting is failing — readiness will go stale", "error", err)
		return
	}
	c.statusPublishFailing.Clear(c.logger, "status reporting recovered")
}

// startStatusReporter periodically re-publishes status so source-manifest
// always has fresh data. Returns a cancel func that stops the goroutine.
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
				// Sample the CURRENT phase: one reporter covers the seed and
				// the watch window, so a long seed is observable while it runs
				// instead of only at its two edges.
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
	// buffer.
	c.seed.Stop(ctx, c.logger)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.publisher.Stop()

	for _, cancel := range c.cancelFuncs {
		cancel()
	}
	c.cancelFuncs = nil
	c.running = false

	c.logger.Info("Objectstore-source stopped",
		"entities_published", c.entitiesPublished.Load(),
		"ingest_errors", c.ingestErrors.Load())
	return nil
}

// Discoverable interface implementation

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "objectstore-source",
		Type:        "processor",
		Description: "S3-compatible object store source for semsource document artifact ingestion",
		Version:     "0.1.0",
	}
}

// InputPorts returns an empty slice — this source generates data from a bucket.
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
	return objectStoreSourceSchema
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
