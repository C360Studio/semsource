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
		Category:    "status",
		Version:     "v1",
		Description: "Ingestion status with per-source phase, entity counts, and aggregate lifecycle",
		Factory:     func() any { return &StatusPayload{} },
	})
	if err != nil {
		panic("failed to register StatusPayload: " + err.Error())
	}
}

// StatusType is the message type for ingestion status payloads.
var StatusType = message.Type{Domain: "semsource", Category: "status", Version: "v1"}

// StatusPayload reports the ingestion lifecycle phase and per-source status.
// Published to graph.ingest.status on the GRAPH stream.
type StatusPayload struct {
	Namespace     string         `json:"namespace"`
	Phase         string         `json:"phase"` // "seeding", "ready", "degraded"
	Sources       []SourceStatus `json:"sources"`
	TotalEntities int64          `json:"total_entities"`
	Timestamp     time.Time      `json:"timestamp"`
}

// SourceStatus reports the status of a single source instance.
type SourceStatus struct {
	InstanceName string           `json:"instance_name"`
	SourceType   string           `json:"source_type"`
	Phase        string           `json:"phase"` // "ingesting", "watching", "idle", "errored"
	EntityCount  int64            `json:"entity_count"`
	ErrorCount   int64            `json:"error_count"`
	TypeCounts   map[string]int64 `json:"type_counts,omitempty"`
}

// Schema implements message.Payload.
func (p *StatusPayload) Schema() message.Type { return StatusType }

// Validate implements message.Payload.
func (p *StatusPayload) Validate() error { return nil }

// MarshalJSON implements json.Marshaler.
func (p *StatusPayload) MarshalJSON() ([]byte, error) {
	type Alias StatusPayload
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *StatusPayload) UnmarshalJSON(data []byte) error {
	type Alias StatusPayload
	return json.Unmarshal(data, (*Alias)(p))
}
