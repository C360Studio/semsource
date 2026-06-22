// Package graph provides the semsource entity payload for the NATS message bus.
//
// All semsource components publish EntityPayload messages to JetStream.
// The payload implements graph.Graphable so graph-ingest can extract
// the entity ID and triples for storage in the ENTITY_STATES KV bucket.
package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
	semvocab "github.com/c360studio/semstreams/vocabulary"
)

// RegisterPayloads registers EntityPayload with the supplied registry.
// Called from cmd/semsource/run.go during bootstrap.
func RegisterPayloads(reg *payloadregistry.Registry) error {
	return reg.Register(&payloadregistry.Registration{
		Domain:      "semsource",
		Category:    "entity",
		Version:     "v1",
		Description: "SemSource entity payload for graph ingestion",
		Factory:     func() any { return &EntityPayload{} },
	})
}

// EntityType is the message type for semsource entity payloads.
var EntityType = message.Type{Domain: "semsource", Category: "entity", Version: "v1"}

const (
	// IndexingProfileContent marks entities that should participate in semantic retrieval.
	IndexingProfileContent = semvocab.IndexingProfileContent
	// IndexingProfileControl marks operational or structural graph state.
	IndexingProfileControl = semvocab.IndexingProfileControl
	// IndexingProfileSignal marks sampled telemetry-like source observations.
	IndexingProfileSignal = semvocab.IndexingProfileSignal
	// IndexingProfileTrace marks generated audit, extraction, or replay traces.
	IndexingProfileTrace = semvocab.IndexingProfileTrace
)

// EntityPayload implements message.Payload, graph.Graphable, and message.Storable.
// This is the single payload type all semsource components use to publish
// entities to graph-ingest via JetStream.
type EntityPayload struct {
	ID                  string                    `json:"id"`
	TripleData          []message.Triple          `json:"triples"`
	UpdatedAt           time.Time                 `json:"updated_at"`
	Storage             *message.StorageReference `json:"storage_ref,omitempty"`
	IndexingProfileHint string                    `json:"indexing_profile,omitempty"`
}

// EntityID implements graph.Graphable.
func (p *EntityPayload) EntityID() string { return p.ID }

// Triples implements graph.Graphable.
func (p *EntityPayload) Triples() []message.Triple { return p.TripleData }

// StorageRef implements message.Storable. Returns nil when content is inline.
func (p *EntityPayload) StorageRef() *message.StorageReference { return p.Storage }

// IndexingProfile implements message.IndexingProfiler.
func (p *EntityPayload) IndexingProfile() string { return p.IndexingProfileHint }

// Schema implements message.Payload.
func (p *EntityPayload) Schema() message.Type { return EntityType }

// Validate implements message.Payload.
func (p *EntityPayload) Validate() error {
	if p.ID == "" {
		return errors.New("entity ID is required")
	}
	if p.IndexingProfileHint == "" {
		return errors.New("indexing profile is required")
	}
	if !semvocab.IsValidIndexingProfile(p.IndexingProfileHint) {
		return fmt.Errorf("invalid indexing profile %q", p.IndexingProfileHint)
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (p *EntityPayload) MarshalJSON() ([]byte, error) {
	type Alias EntityPayload
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *EntityPayload) UnmarshalJSON(data []byte) error {
	type Alias EntityPayload
	return json.Unmarshal(data, (*Alias)(p))
}
