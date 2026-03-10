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
		Category:    "manifest",
		Version:     "v1",
		Description: "Source manifest listing all configured ingestion sources",
		Factory:     func() any { return &ManifestPayload{} },
	})
	if err != nil {
		panic("failed to register ManifestPayload: " + err.Error())
	}
}

// ManifestType is the message type for source manifest payloads.
var ManifestType = message.Type{Domain: "semsource", Category: "manifest", Version: "v1"}

// ManifestPayload describes the configured sources for this SemSource instance.
// Published to the GRAPH stream at startup and available via graph.query.sources.
type ManifestPayload struct {
	Namespace string           `json:"namespace"`
	Sources   []ManifestSource `json:"sources"`
	Timestamp time.Time        `json:"timestamp"`
}

// Schema implements message.Payload.
func (p *ManifestPayload) Schema() message.Type { return ManifestType }

// Validate implements message.Payload.
func (p *ManifestPayload) Validate() error { return nil }

// MarshalJSON implements json.Marshaler.
func (p *ManifestPayload) MarshalJSON() ([]byte, error) {
	type Alias ManifestPayload
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *ManifestPayload) UnmarshalJSON(data []byte) error {
	type Alias ManifestPayload
	return json.Unmarshal(data, (*Alias)(p))
}
