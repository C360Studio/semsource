package graph

import (
	"encoding/json"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
)

func init() {
	err := component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "semsource",
		Category:    "graph_event",
		Version:     "v1",
		Description: "SemSource graph event payload for knowledge graph ingestion",
		Factory:     func() any { return &GraphEventPayload{} },
	})
	if err != nil {
		panic("failed to register GraphEventPayload: " + err.Error())
	}
}

// GraphEventType is the message type for graph event payloads.
var GraphEventType = message.Type{Domain: "semsource", Category: "graph_event", Version: "v1"}

// GraphEventPayload implements message.Payload for graph events.
// It wraps GraphEvent for transport through the semstreams message bus.
type GraphEventPayload struct {
	Event GraphEvent `json:"event"`
}

// Schema returns the message type for the Payload interface.
func (p *GraphEventPayload) Schema() message.Type {
	return GraphEventType
}

// Validate validates the payload for the Payload interface.
func (p *GraphEventPayload) Validate() error {
	return p.Event.Validate()
}

// MarshalJSON implements json.Marshaler.
func (p *GraphEventPayload) MarshalJSON() ([]byte, error) {
	type Alias GraphEventPayload
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *GraphEventPayload) UnmarshalJSON(data []byte) error {
	type Alias GraphEventPayload
	return json.Unmarshal(data, (*Alias)(p))
}
