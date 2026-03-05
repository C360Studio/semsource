package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
)

// Compile-time assertion: Component satisfies component.Discoverable.
var _ component.Discoverable = (*Component)(nil)

// federationSchema is the pre-generated config schema for this component.
var federationSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// NATS subjects for input and output.
const (
	// defaultInputSubject is the JetStream subject this processor consumes GraphEvent messages from.
	defaultInputSubject = "semsource.graph.events"
	// defaultOutputSubject is the JetStream subject merged GraphEvent messages are published to.
	defaultOutputSubject = "semsource.graph.merged"
	// consumerName is the durable consumer name used to avoid message competition.
	consumerName = "federation-processor"
)

// Component implements component.Discoverable for the FederationProcessor.
// It subscribes to incoming GraphEvent payloads on a JetStream subject, applies
// federation merge policy via Merger, and republishes the filtered/merged event
// to the output subject.
type Component struct {
	name       string
	cfg        Config
	merger     *Merger
	natsClient *natsclient.Client
	logger     *slog.Logger

	// Lifecycle state
	mu        sync.Mutex
	running   bool
	startTime time.Time
	cancel    context.CancelFunc

	// Metrics
	processed atomic.Int64
	filtered  atomic.Int64
	errors    atomic.Int64
}

// NewComponent creates a FederationProcessor component from raw JSON config.
// Follows the semstreams factory pattern: unmarshal → apply defaults → validate.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	cfg := DefaultConfig()

	// Unmarshal user-provided values on top of defaults.
	if len(rawConfig) > 0 && string(rawConfig) != "{}" {
		if err := json.Unmarshal(rawConfig, &cfg); err != nil {
			return nil, fmt.Errorf("federation: unmarshal config: %w", err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("federation: invalid config: %w", err)
	}

	logger := deps.GetLogger().With("component", "federation-processor")

	return &Component{
		name:       "federation-processor",
		cfg:        cfg,
		merger:     NewMerger(cfg),
		natsClient: deps.NATSClient,
		logger:     logger,
	}, nil
}

// Start subscribes to the input JetStream subject and begins processing
// GraphEvent messages in the background.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("federation: component already running")
	}
	if c.natsClient == nil {
		return fmt.Errorf("federation: NATS client required")
	}

	procCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	err := c.natsClient.ConsumeStreamWithConfig(procCtx, natsclient.StreamConsumerConfig{
		StreamName:    "GRAPH_EVENTS",
		ConsumerName:  consumerName,
		FilterSubject: defaultInputSubject,
		DeliverPolicy: "new",
		AckPolicy:     "explicit",
	}, c.handleMessage)
	if err != nil {
		cancel()
		return fmt.Errorf("federation: subscribe input: %w", err)
	}

	c.running = true
	c.startTime = time.Now()
	c.logger.Info("federation processor started",
		"namespace", c.cfg.LocalNamespace,
		"policy", c.cfg.MergePolicy)
	return nil
}

// Stop cancels the background goroutine and marks the component as stopped.
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

	c.running = false
	c.logger.Info("federation processor stopped",
		"processed", c.processed.Load(),
		"filtered", c.filtered.Load(),
		"errors", c.errors.Load())
	return nil
}

// handleMessage is the JetStream message handler. It deserialises the incoming
// BaseMessage, extracts the GraphEventPayload, applies merge policy, and
// publishes the result to the output subject.
func (c *Component) handleMessage(ctx context.Context, msg jetstream.Msg) {
	var base message.BaseMessage
	if err := json.Unmarshal(msg.Data(), &base); err != nil {
		c.logger.Warn("federation: unmarshal BaseMessage failed", "error", err)
		c.errors.Add(1)
		_ = msg.Nak()
		return
	}

	// BaseMessage.Payload() returns message.Payload — assert to GraphEventPayload.
	payload, ok := base.Payload().(*graph.GraphEventPayload)
	if !ok {
		c.logger.Warn("federation: unexpected payload type",
			"got", fmt.Sprintf("%T", base.Payload()))
		c.errors.Add(1)
		_ = msg.Nak()
		return
	}

	merged, err := c.merger.ApplyEvent(&payload.Event, nil)
	if err != nil {
		c.logger.Warn("federation: ApplyEvent failed", "error", err)
		c.errors.Add(1)
		_ = msg.Nak()
		return
	}

	// Publish merged event.
	outPayload := &graph.GraphEventPayload{Event: *merged}
	outMsg := message.NewBaseMessage(graph.GraphEventType, outPayload, c.name)
	data, err := json.Marshal(outMsg)
	if err != nil {
		c.logger.Warn("federation: marshal output failed", "error", err)
		c.errors.Add(1)
		_ = msg.Nak()
		return
	}

	if err := c.natsClient.PublishToStream(ctx, defaultOutputSubject, data); err != nil {
		c.logger.Warn("federation: publish merged event failed", "error", err)
		c.errors.Add(1)
		_ = msg.Nak()
		return
	}

	c.processed.Add(1)
	_ = msg.Ack()
}

// --- component.Discoverable implementation ---

// Meta returns component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "federation-processor",
		Type:        "processor",
		Description: "Applies SemSource federation merge policy to incoming GraphEvent payloads",
		Version:     "0.1.0",
	}
}

// InputPorts returns the single JetStream input port definition.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "graph_events_in",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "JetStream input port for incoming GraphEvent payloads",
			Config: component.JetStreamPort{
				StreamName: "GRAPH_EVENTS",
				Subjects:   []string{defaultInputSubject},
			},
		},
	}
}

// OutputPorts returns the single JetStream output port definition.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "graph_events_out",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "JetStream output port for merged GraphEvent payloads",
			Config: component.JetStreamPort{
				StreamName: "GRAPH_MERGED",
				Subjects:   []string{defaultOutputSubject},
			},
		},
	}
}

// ConfigSchema returns the pre-generated configuration schema.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return federationSchema
}

// Health returns the current health status of the component.
func (c *Component) Health() component.HealthStatus {
	c.mu.Lock()
	running := c.running
	startTime := c.startTime
	c.mu.Unlock()

	status := "stopped"
	if running {
		status = "running"
	}

	return component.HealthStatus{
		Healthy:    running,
		LastCheck:  time.Now(),
		ErrorCount: int(c.errors.Load()),
		Uptime:     time.Since(startTime),
		Status:     status,
	}
}

// DataFlow returns current message flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	return component.FlowMetrics{
		MessagesPerSecond: 0,
		BytesPerSecond:    0,
		ErrorRate:         0,
		LastActivity:      time.Now(),
	}
}
