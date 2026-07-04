package supersession

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semstreams/component"
	gtypes "github.com/c360studio/semstreams/graph"
	graphquery "github.com/c360studio/semstreams/graph/query"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
)

// Component runs the versioned-source correspondence and supersession pass.
type Component struct {
	name   string
	config Config
	client *natsclient.Client
	logger *slog.Logger

	mu          sync.RWMutex
	running     bool
	startTime   time.Time
	publisher   *entitypub.Publisher
	queryClient graphquery.Client
	triggerSub  *natsclient.Subscription
	cancel      context.CancelFunc
	lastRun     time.Time
	lastStats   passStats

	// runMu serializes passes so a periodic tick never overlaps an on-demand run.
	runMu sync.Mutex
}

// NewComponent constructs the supersession component from raw config and deps.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &Component{
		name:   "supersession",
		config: config,
		client: deps.NATSClient,
		logger: deps.GetLogger(),
	}, nil
}

// Initialize prepares the component (no-op).
func (c *Component) Initialize() error { return nil }

// Start wires the entity publisher and query client, subscribes to the run
// trigger, and optionally starts the periodic ticker.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	c.mu.Unlock()

	pub, err := entitypub.New(c.client, c.logger)
	if err != nil {
		return fmt.Errorf("create entity publisher: %w", err)
	}
	pub.Start(ctx)

	q, err := graphquery.NewClient(ctx, c.client, nil)
	if err != nil {
		pub.Stop()
		return fmt.Errorf("create query client: %w", err)
	}

	subject := c.config.triggerSubject()
	sub, err := c.client.SubscribeForRequests(ctx, subject, func(reqCtx context.Context, _ []byte) ([]byte, error) {
		stats, runErr := c.runPass(reqCtx)
		if runErr != nil {
			return nil, runErr
		}
		return json.Marshal(stats)
	})
	if err != nil {
		pub.Stop()
		_ = q.Close()
		return fmt.Errorf("subscribe %s: %w", subject, err)
	}

	runCtx, cancel := context.WithCancel(ctx)

	c.mu.Lock()
	c.publisher = pub
	c.queryClient = q
	c.triggerSub = sub
	c.cancel = cancel
	c.running = true
	c.startTime = time.Now()
	c.mu.Unlock()

	c.logger.Info("supersession listening for run requests", "subject", subject)

	if d := c.config.intervalDuration(); d > 0 {
		go c.periodic(runCtx, d)
		c.logger.Info("supersession periodic pass enabled", "interval", d.String())
	}
	return nil
}

// periodic runs the pass on a fixed interval until the context is cancelled.
func (c *Component) periodic(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := c.runPass(ctx); err != nil {
				c.logger.Warn("supersession periodic pass failed", "error", err)
			}
		}
	}
}

// runPass executes one correspondence + supersession pass: enumerate versioned
// code entities, group and order them, compute lineage edges, and publish only
// the additive delta. Read-and-append only — never retracts or overwrites — and
// idempotent via diffNew. Passes are serialized (runMu).
func (c *Component) runPass(ctx context.Context) (passStats, error) {
	c.runMu.Lock()
	defer c.runMu.Unlock()

	c.mu.RLock()
	q := c.queryClient
	c.mu.RUnlock()
	if q == nil {
		return passStats{}, fmt.Errorf("supersession not started")
	}

	req := gtypes.PrefixQueryRequest{Prefix: c.config.Prefix}
	entities, truncated, err := q.QueryPrefixAll(ctx, req, c.config.maxEntities())
	if err != nil {
		return passStats{}, fmt.Errorf("enumerate entities: %w", err)
	}
	if truncated {
		c.logger.Warn("supersession enumeration hit max_entities cap; some entities not scanned",
			"max_entities", c.config.maxEntities())
	}

	cands := make([]candidate, 0, len(entities))
	existing := make(map[string][]message.Triple, len(entities))
	for i := range entities {
		cand, ok := candidateFromEntity(entities[i])
		if !ok {
			continue
		}
		cands = append(cands, cand)
		existing[cand.id] = entities[i].Triples
	}

	desired, stats := desiredEdges(groupByCorrespondence(cands))
	stats.Entities = len(cands)

	delta := diffNew(desired, existing)
	updated := c.publishDelta(delta)

	c.mu.Lock()
	c.lastRun = time.Now()
	c.lastStats = stats
	c.mu.Unlock()

	c.logger.Info("supersession pass complete",
		"entities", stats.Entities,
		"groups", stats.Groups,
		"corresponding", stats.Corresponding,
		"supersedes", stats.Supersedes,
		"changed", stats.Changed,
		"incomparable", stats.Incomparable,
		"entities_updated", updated)
	return stats, nil
}

// publishDelta appends each entity's fresh lineage triples via the entity
// publisher (the graph.ingest.entity merge path). One EntityPayload per entity
// carries only the delta triples — no ontology re-stamp (the target is already
// classified) and an operational indexing profile (the ingest merge keeps the
// target's original profile). Returns the number of entities updated.
func (c *Component) publishDelta(delta map[string][]message.Triple) int {
	c.mu.RLock()
	pub := c.publisher
	c.mu.RUnlock()
	if pub == nil {
		return 0
	}

	updated := 0
	for id, triples := range delta {
		payload := &graph.EntityPayload{
			ID:                  id,
			TripleData:          triples,
			UpdatedAt:           time.Now(),
			IndexingProfileHint: graph.IndexingProfileControl,
		}
		if err := entitypub.ValidatePayload(payload); err != nil {
			c.logger.Warn("invalid supersession edge payload; skipping", "id", id, "error", err)
			continue
		}
		pub.Send(payload)
		updated++
	}
	return updated
}

// Stop cancels the periodic loop, unsubscribes, and flushes the publisher.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return nil
	}
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if c.triggerSub != nil {
		if err := c.triggerSub.Unsubscribe(); err != nil {
			c.logger.Warn("failed to unsubscribe", "error", err)
		}
		c.triggerSub = nil
	}
	if c.publisher != nil {
		c.publisher.Stop()
		c.publisher = nil
	}
	if c.queryClient != nil {
		if err := c.queryClient.Close(); err != nil {
			c.logger.Warn("failed to close query client", "error", err)
		}
		c.queryClient = nil
	}
	c.running = false
	c.logger.Info("supersession stopped")
	return nil
}

// Meta implements component.Discoverable.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "supersession",
		Type:        "processor",
		Description: "Relates code entities across versions with directional supersession lineage edges",
		Version:     "0.1.0",
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
			port.Config = component.NATSPort{Subject: portDef.Subject}
		}
		ports[i] = port
	}
	return ports
}

// ConfigSchema implements component.Discoverable.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return supersessionSchema
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
