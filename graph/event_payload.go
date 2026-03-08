// Package graph provides the semsource entity payload for the NATS message bus.
//
// All semsource components publish EntityPayload messages to JetStream.
// The payload implements graph.Graphable so graph-ingest can extract
// the entity ID and triples for storage in the ENTITY_STATES KV bucket.
package graph

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
)

func init() {
	err := component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "semsource",
		Category:    "entity",
		Version:     "v1",
		Description: "SemSource entity payload for graph ingestion",
		Factory:     func() any { return &EntityPayload{} },
	})
	if err != nil {
		panic("failed to register EntityPayload: " + err.Error())
	}
}

// EntityType is the message type for semsource entity payloads.
var EntityType = message.Type{Domain: "semsource", Category: "entity", Version: "v1"}

// EntityPayload implements message.Payload and graph.Graphable.
// This is the single payload type all semsource components use to publish
// entities to graph-ingest via JetStream.
type EntityPayload struct {
	ID         string           `json:"id"`
	TripleData []message.Triple `json:"triples"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

// EntityID implements graph.Graphable.
func (p *EntityPayload) EntityID() string { return p.ID }

// Triples implements graph.Graphable.
func (p *EntityPayload) Triples() []message.Triple { return p.TripleData }

// Schema implements message.Payload.
func (p *EntityPayload) Schema() message.Type { return EntityType }

// Validate implements message.Payload.
func (p *EntityPayload) Validate() error {
	if p.ID == "" {
		return errors.New("entity ID is required")
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
