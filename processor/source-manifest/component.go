package sourcemanifest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
)

const (
	// manifestSubject is the NATS subject for publishing manifest events.
	manifestSubject = "graph.ingest.manifest"

	// querySubject is the NATS subject for request/reply source queries.
	querySubject = "graph.query.sources"
)

// Component implements the source-manifest processor.
// On startup it publishes a ManifestPayload to the GRAPH stream and
// subscribes to graph.query.sources for on-demand request/reply queries.
type Component struct {
	name   string
	config Config
	client *natsclient.Client
	logger *slog.Logger

	querySub     *natsclient.Subscription
	responseData []byte // pre-marshaled manifest for HTTP and NATS responses

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

// Start publishes the manifest to the GRAPH stream and sets up the query subscription.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	c.mu.Unlock()

	// Build the manifest payload.
	payload := &ManifestPayload{
		Namespace: c.config.Namespace,
		Sources:   c.config.Sources,
		Timestamp: time.Now(),
	}

	// Publish to the GRAPH stream so WebSocket consumers get it automatically.
	if err := c.publishManifest(ctx, payload); err != nil {
		return fmt.Errorf("publish manifest: %w", err)
	}
	c.logger.Info("source manifest published",
		"namespace", c.config.Namespace,
		"sources", len(c.config.Sources))

	// Pre-marshal response for NATS and HTTP queries.
	var err error
	c.responseData, err = c.marshalResponse(payload)
	if err != nil {
		return fmt.Errorf("marshal query response: %w", err)
	}

	// Subscribe for on-demand NATS queries.
	sub, err := c.client.SubscribeForRequests(ctx, querySubject, func(_ context.Context, _ []byte) ([]byte, error) {
		return c.responseData, nil
	})
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", querySubject, err)
	}
	c.logger.Info("listening for source queries", "subject", querySubject)

	c.mu.Lock()
	c.querySub = sub
	c.running = true
	c.startTime = time.Now()
	c.mu.Unlock()

	return nil
}

// publishManifest marshals and publishes the manifest payload to JetStream.
func (c *Component) publishManifest(ctx context.Context, payload *ManifestPayload) error {
	msg := message.NewBaseMessage(ManifestType, payload, "semsource")
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal manifest message: %w", err)
	}
	return c.client.PublishToStream(ctx, manifestSubject, data)
}

// marshalResponse pre-marshals the manifest for fast query responses.
func (c *Component) marshalResponse(payload *ManifestPayload) ([]byte, error) {
	return json.Marshal(payload)
}

// Stop gracefully stops the component.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	if c.querySub != nil {
		if err := c.querySub.Unsubscribe(); err != nil {
			c.logger.Warn("failed to unsubscribe query handler", "error", err)
		}
		c.querySub = nil
	}

	c.running = false
	c.logger.Info("source-manifest stopped")
	return nil
}

// Meta implements component.Discoverable.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "source-manifest",
		Type:        "processor",
		Description: "Publishes configured source manifest and serves source queries",
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

// RegisterHTTPHandlers registers the /sources endpoint on the ServiceManager's
// shared HTTP mux. The ServiceManager discovers this method automatically via
// interface assertion and calls it with the component instance name as prefix.
func (c *Component) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix = prefix + "/"
	}
	path := prefix + "sources"
	mux.HandleFunc(path, c.handleSources)
	c.logger.Info("registered HTTP handler", "path", path)
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
