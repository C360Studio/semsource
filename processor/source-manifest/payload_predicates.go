package sourcemanifest

import (
	"encoding/json"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
)

func init() {
	err := component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "semsource",
		Category:    "predicates",
		Version:     "v1",
		Description: "Predicate schema advertising predicates emitted per source type with semantic roles",
		Factory:     func() any { return &PredicateSchemaPayload{} },
	})
	if err != nil {
		panic("failed to register PredicateSchemaPayload: " + err.Error())
	}
}

// PredicatesType is the message type for predicate schema payloads.
var PredicatesType = message.Type{Domain: "semsource", Category: "predicates", Version: "v1"}

// PredicateSchemaPayload advertises the predicates emitted by each source type
// with semantic role classification. Published to graph.ingest.predicates.
type PredicateSchemaPayload struct {
	Sources   []SourcePredicateSchema `json:"sources"`
	Timestamp time.Time               `json:"timestamp"`
}

// SourcePredicateSchema describes the predicates a source type emits.
type SourcePredicateSchema struct {
	SourceType string                `json:"source_type"`
	Predicates []PredicateDescriptor `json:"predicates"`
}

// PredicateDescriptor describes a single predicate with its semantic role.
type PredicateDescriptor struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	DataType    string `json:"data_type"`
	Role        string `json:"role"` // "identity", "content", "location", "relationship", "metric", "metadata"
}

// Schema implements message.Payload.
func (p *PredicateSchemaPayload) Schema() message.Type { return PredicatesType }

// Validate implements message.Payload.
func (p *PredicateSchemaPayload) Validate() error { return nil }

// MarshalJSON implements json.Marshaler.
func (p *PredicateSchemaPayload) MarshalJSON() ([]byte, error) {
	type Alias PredicateSchemaPayload
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *PredicateSchemaPayload) UnmarshalJSON(data []byte) error {
	type Alias PredicateSchemaPayload
	return json.Unmarshal(data, (*Alias)(p))
}
