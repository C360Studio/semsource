package graph

import (
	"errors"
	"time"
)

// EventType is the type of a graph event emitted by SemSource.
type EventType string

const (
	// EventTypeSEED is emitted on startup and consumer reconnect with a full graph snapshot.
	EventTypeSEED EventType = "SEED"

	// EventTypeDELTA is emitted for incremental upserts from watch events.
	EventTypeDELTA EventType = "DELTA"

	// EventTypeRETRACT is emitted when entities are explicitly removed.
	EventTypeRETRACT EventType = "RETRACT"

	// EventTypeHEARTBEAT is emitted during quiet periods as a liveness signal.
	EventTypeHEARTBEAT EventType = "HEARTBEAT"
)

// GraphEvent represents a single graph mutation event per spec S6.1.
// Events flow from source handlers through the normalizer to the WebSocket output.
type GraphEvent struct {
	// Type is the event type enum.
	Type EventType `json:"type"`

	// SourceID identifies the source that produced this event.
	SourceID string `json:"source_id"`

	// Namespace is the org namespace for this event (e.g., "acme", "public").
	Namespace string `json:"namespace"`

	// Timestamp is when the event was generated.
	Timestamp time.Time `json:"timestamp"`

	// Entities contains graph entities for SEED and DELTA events.
	Entities []GraphEntity `json:"entities,omitempty"`

	// Retractions contains entity IDs to remove for RETRACT events.
	Retractions []string `json:"retractions,omitempty"`

	// Provenance records the event origin.
	Provenance SourceProvenance `json:"provenance"`
}

// Validate checks that the GraphEvent contains all required fields.
func (e *GraphEvent) Validate() error {
	if e.Type == "" {
		return errors.New("event type is required")
	}
	if e.SourceID == "" {
		return errors.New("source ID is required")
	}
	if e.Namespace == "" {
		return errors.New("namespace is required")
	}
	if e.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}
	return nil
}
