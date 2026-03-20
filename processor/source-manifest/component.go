package sourcemanifest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
)

const (
	// manifestSubject is the NATS subject for publishing manifest events.
	manifestSubject = "graph.ingest.manifest"

	// querySubject is the NATS subject for request/reply source queries.
	querySubject = "graph.query.sources"

	// summaryQuerySubject is the NATS subject for on-demand summary queries.
	summaryQuerySubject = "graph.query.summary"
)

// Component implements the source-manifest processor.
// On startup it publishes a ManifestPayload to the GRAPH stream and
// subscribes to graph.query.sources for on-demand request/reply queries.
// It also aggregates status reports from source components and publishes
// ingestion status and predicate schema to consumers.
type Component struct {
	name   string
	config Config
	client *natsclient.Client
	logger *slog.Logger

	// Manifest query
	querySub     *natsclient.Subscription
	responseData []byte // pre-marshaled manifest for HTTP and NATS responses

	// Status aggregation
	aggregator     *statusAggregator
	statusSub      *natsclient.Subscription
	statusQuerySub *natsclient.Subscription
	statusData     []byte // pre-marshaled latest status for HTTP and NATS
	statusMu       sync.RWMutex
	seedComplete   bool

	// Predicate schema
	predicatesQuerySub *natsclient.Subscription
	predicatesData     []byte // pre-marshaled predicate schema for HTTP and NATS

	// Summary query
	summaryQuerySub *natsclient.Subscription

	// Background goroutine cancellation
	cancelFuncs []context.CancelFunc

	running   bool
	startTime time.Time
	mu        sync.RWMutex
}

// NewComponent creates a new source-manifest component.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &Component{
		name:   "source-manifest",
		config: config,
		client: deps.NATSClient,
		logger: deps.GetLogger(),
	}, nil
}

// Initialize prepares the component (no-op).
func (c *Component) Initialize() error { return nil }

// Start publishes the manifest and predicate schema, then begins
// listening for source status reports and serving queries.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	c.mu.Unlock()

	if err := c.startManifest(ctx); err != nil {
		return err
	}
	if err := c.startPredicateSchema(ctx); err != nil {
		return err
	}
	if err := c.startStatusAggregation(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	c.running = true
	c.startTime = time.Now()
	c.mu.Unlock()

	return nil
}

// startManifest publishes the manifest and sets up query handlers.
func (c *Component) startManifest(ctx context.Context) error {
	payload := &ManifestPayload{
		Namespace: c.config.Namespace,
		Sources:   c.config.Sources,
		Timestamp: time.Now(),
	}

	if err := c.publishPayload(ctx, ManifestType, payload, manifestSubject); err != nil {
		return fmt.Errorf("publish manifest: %w", err)
	}
	c.logger.Info("source manifest published",
		"namespace", c.config.Namespace,
		"sources", len(c.config.Sources))

	var err error
	c.responseData, err = json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal manifest response: %w", err)
	}

	sub, err := c.client.SubscribeForRequests(ctx, querySubject, func(_ context.Context, _ []byte) ([]byte, error) {
		return c.responseData, nil
	})
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", querySubject, err)
	}
	c.querySub = sub
	c.logger.Info("listening for source queries", "subject", querySubject)
	return nil
}

// startPredicateSchema builds and publishes the predicate schema.
func (c *Component) startPredicateSchema(ctx context.Context) error {
	sourceTypes := c.configuredSourceTypes()
	schema := buildPredicateSchema(sourceTypes)

	if err := c.publishPayload(ctx, PredicatesType, schema, predicatesSubject); err != nil {
		return fmt.Errorf("publish predicate schema: %w", err)
	}
	c.logger.Info("predicate schema published", "source_types", len(sourceTypes))

	var err error
	c.predicatesData, err = json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("marshal predicates response: %w", err)
	}

	sub, err := c.client.SubscribeForRequests(ctx, predicatesQuerySubject, func(_ context.Context, _ []byte) ([]byte, error) {
		return c.predicatesData, nil
	})
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", predicatesQuerySubject, err)
	}
	c.predicatesQuerySub = sub
	c.logger.Info("listening for predicate queries", "subject", predicatesQuerySubject)
	return nil
}

// startStatusAggregation subscribes to internal status reports, starts the
// heartbeat loop, and sets up the seed timeout.
func (c *Component) startStatusAggregation(ctx context.Context) error {
	c.aggregator = newStatusAggregator(c.config.ExpectedSourceCount)

	// Publish initial seeding status.
	initialStatus := c.aggregator.buildStatus(c.config.Namespace)
	c.updateStatusData(initialStatus)
	if err := c.publishPayload(ctx, StatusType, initialStatus, statusSubject); err != nil {
		c.logger.Warn("failed to publish initial status", "error", err)
	}

	// Subscribe to internal status reports from source components.
	sub, err := c.client.Subscribe(ctx, statusReportSubject, func(_ context.Context, msg *nats.Msg) {
		c.handleStatusReport(ctx, msg.Data)
	})
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", statusReportSubject, err)
	}
	c.statusSub = sub

	// Subscribe for on-demand status queries.
	querySub, err := c.client.SubscribeForRequests(ctx, statusQuerySubject, func(_ context.Context, _ []byte) ([]byte, error) {
		c.statusMu.RLock()
		defer c.statusMu.RUnlock()
		return c.statusData, nil
	})
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", statusQuerySubject, err)
	}
	c.statusQuerySub = querySub
	c.logger.Info("listening for status queries", "subject", statusQuerySubject)

	// Subscribe for on-demand summary queries.
	summarySub, err := c.client.SubscribeForRequests(ctx, summaryQuerySubject, func(_ context.Context, _ []byte) ([]byte, error) {
		summary := c.buildSummary()
		return json.Marshal(summary)
	})
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", summaryQuerySubject, err)
	}
	c.summaryQuerySub = summarySub
	c.logger.Info("listening for summary queries", "subject", summaryQuerySubject)

	// Start heartbeat and seed timeout goroutines.
	heartbeatCancel := c.startHeartbeat(ctx)
	c.cancelFuncs = append(c.cancelFuncs, heartbeatCancel)

	timeoutCancel := c.startSeedTimeout(ctx)
	if timeoutCancel != nil {
		c.cancelFuncs = append(c.cancelFuncs, timeoutCancel)
	}

	return nil
}

// handleStatusReport processes an incoming status report from a source component.
func (c *Component) handleStatusReport(ctx context.Context, data []byte) {
	var report SourceStatusReport
	if err := json.Unmarshal(data, &report); err != nil {
		c.logger.Warn("invalid status report", "error", err)
		return
	}

	c.statusMu.Lock()
	wasComplete := c.seedComplete
	c.aggregator.update(&report)
	nowComplete := c.aggregator.allReported()
	if nowComplete {
		c.seedComplete = true
	}
	status := c.aggregator.buildStatus(c.config.Namespace)
	c.statusMu.Unlock()

	c.updateStatusData(status)

	// Publish seed-complete signal the first time all sources report.
	if nowComplete && !wasComplete {
		c.logger.Info("all sources reported — seed complete",
			"total_entities", status.TotalEntities,
			"sources", len(status.Sources))
		if err := c.publishPayload(ctx, StatusType, status, statusSubject); err != nil {
			c.logger.Warn("failed to publish seed-complete status", "error", err)
		}
	}
}

// startHeartbeat publishes status periodically.
func (c *Component) startHeartbeat(ctx context.Context) context.CancelFunc {
	interval := parseDurationOrDefault(c.config.HeartbeatInterval, 30*time.Second)
	hbCtx, cancel := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				c.statusMu.RLock()
				status := c.aggregator.buildStatus(c.config.Namespace)
				c.statusMu.RUnlock()
				c.updateStatusData(status)

				if err := c.publishPayload(hbCtx, StatusType, status, statusSubject); err != nil {
					c.logger.Debug("heartbeat publish failed", "error", err)
				}
			}
		}
	}()

	return cancel
}

// startSeedTimeout sets a deadline for all sources to report.
func (c *Component) startSeedTimeout(ctx context.Context) context.CancelFunc {
	timeout := parseDurationOrDefault(c.config.SeedTimeout, 120*time.Second)
	if c.config.ExpectedSourceCount <= 0 {
		return nil
	}

	toCtx, cancel := context.WithCancel(ctx)

	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case <-toCtx.Done():
			return
		case <-timer.C:
			c.statusMu.Lock()
			if c.seedComplete {
				c.statusMu.Unlock()
				return
			}
			status := c.aggregator.markDegraded(c.config.Namespace)
			c.seedComplete = true // don't fire again
			c.statusMu.Unlock()

			c.updateStatusData(status)
			c.logger.Warn("seed timeout — marking status degraded",
				"expected", c.config.ExpectedSourceCount,
				"received", len(status.Sources))

			if err := c.publishPayload(toCtx, StatusType, status, statusSubject); err != nil {
				c.logger.Warn("failed to publish degraded status", "error", err)
			}
		}
	}()

	return cancel
}

// updateStatusData pre-marshals the latest status for query responses.
func (c *Component) updateStatusData(status *StatusPayload) {
	data, err := json.Marshal(status)
	if err != nil {
		c.logger.Warn("failed to marshal status", "error", err)
		return
	}
	c.statusMu.Lock()
	c.statusData = data
	c.statusMu.Unlock()
}

// buildSummary constructs a SummaryPayload from current aggregator state
// and the predicate schema. Called on-demand for query responses.
func (c *Component) buildSummary() *SummaryPayload {
	c.statusMu.RLock()
	status := c.aggregator.buildStatus(c.config.Namespace)
	c.statusMu.RUnlock()

	// Aggregate type counts by domain across all source instances.
	domainMap := make(map[string]*DomainSummary)
	for _, src := range status.Sources {
		for key, count := range src.TypeCounts {
			parts := strings.SplitN(key, ".", 2)
			if len(parts) != 2 {
				continue
			}
			domain, eType := parts[0], parts[1]
			ds, ok := domainMap[domain]
			if !ok {
				ds = &DomainSummary{Domain: domain}
				domainMap[domain] = ds
			}
			ds.EntityCount += count
			// Accumulate type counts.
			found := false
			for i := range ds.Types {
				if ds.Types[i].Type == eType {
					ds.Types[i].Count += count
					found = true
					break
				}
			}
			if !found {
				ds.Types = append(ds.Types, TypeCount{Type: eType, Count: count})
			}
			// Track contributing sources.
			hasSource := false
			for _, s := range ds.Sources {
				if s == src.InstanceName {
					hasSource = true
					break
				}
			}
			if !hasSource {
				ds.Sources = append(ds.Sources, src.InstanceName)
			}
		}
	}

	// Convert map to sorted slice (sort by entity count descending).
	domains := make([]DomainSummary, 0, len(domainMap))
	for _, ds := range domainMap {
		// Sort types by count descending.
		sort.Slice(ds.Types, func(i, j int) bool {
			return ds.Types[i].Count > ds.Types[j].Count
		})
		domains = append(domains, *ds)
	}
	sort.Slice(domains, func(i, j int) bool {
		return domains[i].EntityCount > domains[j].EntityCount
	})

	// Get predicate schema.
	sourceTypes := c.configuredSourceTypes()
	predicates := buildPredicateSchema(sourceTypes)

	return &SummaryPayload{
		Namespace:      c.config.Namespace,
		Phase:          status.Phase,
		EntityIDFormat: "{org}.semsource.{domain}.{system}.{type}.{instance}",
		TotalEntities:  status.TotalEntities,
		Domains:        domains,
		Predicates:     predicates.Sources,
		Timestamp:      time.Now(),
	}
}

// publishPayload marshals and publishes a payload to JetStream.
func (c *Component) publishPayload(ctx context.Context, msgType message.Type, payload message.Payload, subject string) error {
	msg := message.NewBaseMessage(msgType, payload, "semsource")
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	return c.client.PublishToStream(ctx, subject, data)
}

// configuredSourceTypes returns the unique source types from config.
func (c *Component) configuredSourceTypes() []string {
	seen := make(map[string]bool)
	var types []string
	for _, src := range c.config.Sources {
		if !seen[src.Type] {
			seen[src.Type] = true
			types = append(types, src.Type)
		}
	}
	return types
}

// Stop gracefully stops the component.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	for _, cancel := range c.cancelFuncs {
		cancel()
	}
	c.cancelFuncs = nil

	for _, sub := range []*natsclient.Subscription{c.querySub, c.statusSub, c.statusQuerySub, c.predicatesQuerySub, c.summaryQuerySub} {
		if sub != nil {
			if err := sub.Unsubscribe(); err != nil {
				c.logger.Warn("failed to unsubscribe", "error", err)
			}
		}
	}
	c.querySub = nil
	c.statusSub = nil
	c.statusQuerySub = nil
	c.predicatesQuerySub = nil
	c.summaryQuerySub = nil

	c.running = false
	c.logger.Info("source-manifest stopped")
	return nil
}

// Meta implements component.Discoverable.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "source-manifest",
		Type:        "processor",
		Description: "Publishes configured source manifest, ingestion status, and predicate schema",
		Version:     "0.2.0",
	}
}

// InputPorts implements component.Discoverable.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{}
}

// OutputPorts implements component.Discoverable.
func (c *Component) OutputPorts() []component.Port {
	if c.config.Ports == nil {
		return []component.Port{}
	}
	ports := make([]component.Port, len(c.config.Ports.Outputs))
	for i, portDef := range c.config.Ports.Outputs {
		port := component.Port{
			Name:        portDef.Name,
			Direction:   component.DirectionOutput,
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
		ports[i] = port
	}
	return ports
}

// ConfigSchema implements component.Discoverable.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return manifestSchema
}

// Health implements component.Discoverable.
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
		Healthy:   running,
		LastCheck: time.Now(),
		Uptime:    time.Since(startTime),
		Status:    status,
	}
}

// DataFlow implements component.Discoverable.
func (c *Component) DataFlow() component.FlowMetrics {
	return component.FlowMetrics{}
}

// RegisterHTTPHandlers registers the /sources, /status, and /predicates
// endpoints on the ServiceManager's shared HTTP mux.
func (c *Component) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix = prefix + "/"
	}

	sourcesPath := prefix + "sources"
	mux.HandleFunc(sourcesPath, c.handleSources)
	c.logger.Info("registered HTTP handler", "path", sourcesPath)

	statusPath := prefix + "status"
	mux.HandleFunc(statusPath, c.handleStatus)
	c.logger.Info("registered HTTP handler", "path", statusPath)

	predicatesPath := prefix + "predicates"
	mux.HandleFunc(predicatesPath, c.handlePredicates)
	c.logger.Info("registered HTTP handler", "path", predicatesPath)

	summaryPath := prefix + "summary"
	mux.HandleFunc(summaryPath, c.handleSummary)
	c.logger.Info("registered HTTP handler", "path", summaryPath)
}

// handleSources serves the pre-marshaled manifest payload.
func (c *Component) handleSources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(c.responseData)
}

// handleStatus serves the current aggregated status.
func (c *Component) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c.statusMu.RLock()
	data := c.statusData
	c.statusMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// handlePredicates serves the predicate schema.
func (c *Component) handlePredicates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(c.predicatesData)
}

// handleSummary serves a combined status and predicate schema response.
func (c *Component) handleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	summary := c.buildSummary()
	data, err := json.Marshal(summary)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// parseDurationOrDefault parses a duration string with a fallback default.
func parseDurationOrDefault(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}
