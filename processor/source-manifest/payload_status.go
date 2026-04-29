package sourcemanifest

import (
	"encoding/json"
	"time"

	"github.com/c360studio/semstreams/message"
)

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
	LastError    *SourceError     `json:"last_error,omitempty"`
}

// SourceErrorCode is a typed code for asynchronous source-runtime failures.
// Codes flow to remote callers (subscribers of graph.ingest.status) so they
// can branch on retryability without scraping log strings.
type SourceErrorCode string

const (
	// SourceUnreachable indicates the source's origin could not be reached
	// (git clone 404, URL DNS fail, path missing).
	SourceUnreachable SourceErrorCode = "SOURCE_UNREACHABLE"

	// SourceAuthFailed indicates an authentication or authorization failure
	// reaching the source (private repo without creds, 401/403 from URL).
	SourceAuthFailed SourceErrorCode = "SOURCE_AUTH_FAILED"

	// WatchFailed indicates the watch subsystem (fsnotify, polling) failed
	// after the initial seed completed.
	WatchFailed SourceErrorCode = "WATCH_FAILED"
)

// SourceError describes the most recent asynchronous failure for a source
// instance. Cleared (nil) when the source recovers and is reporting healthy.
type SourceError struct {
	Code      SourceErrorCode `json:"code"`
	Message   string          `json:"message"`
	Timestamp time.Time       `json:"timestamp"`
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
