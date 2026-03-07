package engine

import (
	"context"
	"log/slog"
	"sync"

	"github.com/c360studio/semstreams/federation"
)

// Emitter publishes federation Events to downstream consumers.
// Implementations must be safe for concurrent use.
type Emitter interface {
	Emit(ctx context.Context, event *federation.Event) error
}

// LogEmitter logs events via slog and captures them for test inspection.
// It is goroutine-safe.
type LogEmitter struct {
	logger *slog.Logger
	mu     sync.Mutex
	events []*federation.Event
}

// NewLogEmitter creates a LogEmitter that writes to the given logger.
func NewLogEmitter(logger *slog.Logger) *LogEmitter {
	return &LogEmitter{logger: logger}
}

// Emit logs the event and appends it to the internal capture slice.
func (e *LogEmitter) Emit(_ context.Context, event *federation.Event) error {
	e.mu.Lock()
	e.events = append(e.events, event)
	e.mu.Unlock()

	e.logger.Info("graph event emitted",
		"type", event.Type,
		"namespace", event.Namespace,
		"source_id", event.SourceID,
		"entity_count", len(event.Entities),
		"retraction_count", len(event.Retractions),
	)
	return nil
}

// CapturedEvents returns a snapshot of all events emitted so far.
// The returned slice is a copy and safe to inspect from tests.
func (e *LogEmitter) CapturedEvents() []*federation.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*federation.Event, len(e.events))
	copy(out, e.events)
	return out
}

// NATSEmitter publishes federation Events to a NATS subject.
// It is a placeholder — NATS wiring is deferred until the engine is fully integrated.
type NATSEmitter struct {
	subject string
}

// NewNATSEmitter constructs a NATSEmitter that will publish to the given subject.
func NewNATSEmitter(subject string) *NATSEmitter {
	return &NATSEmitter{subject: subject}
}

// Emit serializes and publishes the event to the configured NATS subject.
// Currently a no-op stub until NATS wiring is complete.
func (e *NATSEmitter) Emit(_ context.Context, _ *federation.Event) error {
	// TODO: marshal event and publish via natsConn.Publish(e.subject, data)
	return nil
}
