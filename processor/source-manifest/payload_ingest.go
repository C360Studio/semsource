package sourcemanifest

import (
	"encoding/json"
	"time"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semstreams/message"
)

// AddRequestType is the message type for ingest add requests.
var AddRequestType = message.Type{Domain: "semsource", Category: "ingest.add.request", Version: "v1"}

// AddReplyType is the message type for ingest add replies.
var AddReplyType = message.Type{Domain: "semsource", Category: "ingest.add.reply", Version: "v1"}

// RemoveRequestType is the message type for ingest remove requests.
var RemoveRequestType = message.Type{Domain: "semsource", Category: "ingest.remove.request", Version: "v1"}

// RemoveReplyType is the message type for ingest remove replies.
var RemoveReplyType = message.Type{Domain: "semsource", Category: "ingest.remove.reply", Version: "v1"}

// IngestErrorCode is a typed error code returned in ingest reply payloads.
// Codes flow to remote callers so they can branch on retryability without
// scraping log strings. See ADR-0003 for the full code list.
type IngestErrorCode string

const (
	// CodeValidationFailed indicates the SourceEntry did not pass type-specific
	// validation (missing required fields, invalid duration strings, etc.).
	// Not retryable — the caller must fix the input.
	CodeValidationFailed IngestErrorCode = "VALIDATION_FAILED"

	// CodeInstanceExists indicates a component with the deterministic instance
	// name already exists with a different config. Idempotent re-submits with
	// matching configs return Created=false and no error instead.
	CodeInstanceExists IngestErrorCode = "INSTANCE_EXISTS"

	// CodeKVWriteFailed indicates the underlying ConfigManager KV write
	// failed. Retryable.
	CodeKVWriteFailed IngestErrorCode = "KV_WRITE_FAILED"

	// CodeUnsupportedType indicates the SourceEntry.Type is not yet
	// implementable through this API (e.g., multi-branch repo).
	CodeUnsupportedType IngestErrorCode = "UNSUPPORTED_TYPE"

	// CodeNotFound indicates a remove request named an instance that does not
	// exist. Idempotent removes (delete-of-missing) succeed without this code;
	// it is reserved for explicit-conflict modes.
	CodeNotFound IngestErrorCode = "NOT_FOUND"
)

// IngestError is the typed error envelope embedded in reply payloads.
type IngestError struct {
	Code    IngestErrorCode `json:"code"`
	Message string          `json:"message"`
}

// Provenance carries caller-supplied attribution for an add or remove
// request. Recorded on source metadata and forward-stamped on entity-event
// provenance, but otherwise opaque to SemSource — its shape is whatever
// SemTeams or other callers send (per ADR-0003 §"Authorization").
type Provenance struct {
	Actor      string `json:"actor,omitempty"`
	OnBehalfOf string `json:"on_behalf_of,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
}

// AddRequest registers a new source on the receiving SemSource instance.
// Sent to graph.ingest.add.{namespace} as request/reply.
type AddRequest struct {
	Source     config.SourceEntry `json:"source"`
	Provenance Provenance         `json:"provenance,omitzero"`
}

// AddedComponent describes one component spawned by an add. A flat source
// produces one; a "repo" meta-source produces multiple (git, ast, doc,
// cfgfile).
type AddedComponent struct {
	InstanceName string `json:"instance_name"`
	FactoryName  string `json:"factory_name"`
	SourceType   string `json:"source_type"`
	Created      bool   `json:"created"`
}

// AddReply is the response to an AddRequest. Components is empty when Error
// is non-nil. ReadyWhen describes the condition the caller can poll on
// StatusSubject to know the source is graph-queryable (see ADR-0003).
type AddReply struct {
	Components    []AddedComponent `json:"components,omitempty"`
	StatusSubject string           `json:"status_subject,omitempty"`
	ReadyWhen     string           `json:"ready_when,omitempty"`
	Error         *IngestError     `json:"error,omitempty"`
	Timestamp     time.Time        `json:"timestamp"`
}

// RemoveRequest deregisters a source by instance name. Sent to
// graph.ingest.remove.{namespace} as request/reply.
type RemoveRequest struct {
	InstanceName string     `json:"instance_name"`
	Provenance   Provenance `json:"provenance,omitzero"`
}

// RemoveReply is the response to a RemoveRequest.
type RemoveReply struct {
	InstanceName string       `json:"instance_name"`
	Removed      bool         `json:"removed"`
	Error        *IngestError `json:"error,omitempty"`
	Timestamp    time.Time    `json:"timestamp"`
}

// Schema implements message.Payload.
func (p *AddRequest) Schema() message.Type { return AddRequestType }

// Validate implements message.Payload.
func (p *AddRequest) Validate() error { return nil }

// MarshalJSON implements json.Marshaler.
func (p *AddRequest) MarshalJSON() ([]byte, error) {
	type Alias AddRequest
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *AddRequest) UnmarshalJSON(data []byte) error {
	type Alias AddRequest
	return json.Unmarshal(data, (*Alias)(p))
}

// Schema implements message.Payload.
func (p *AddReply) Schema() message.Type { return AddReplyType }

// Validate implements message.Payload.
func (p *AddReply) Validate() error { return nil }

// MarshalJSON implements json.Marshaler.
func (p *AddReply) MarshalJSON() ([]byte, error) {
	type Alias AddReply
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *AddReply) UnmarshalJSON(data []byte) error {
	type Alias AddReply
	return json.Unmarshal(data, (*Alias)(p))
}

// Schema implements message.Payload.
func (p *RemoveRequest) Schema() message.Type { return RemoveRequestType }

// Validate implements message.Payload.
func (p *RemoveRequest) Validate() error { return nil }

// MarshalJSON implements json.Marshaler.
func (p *RemoveRequest) MarshalJSON() ([]byte, error) {
	type Alias RemoveRequest
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *RemoveRequest) UnmarshalJSON(data []byte) error {
	type Alias RemoveRequest
	return json.Unmarshal(data, (*Alias)(p))
}

// Schema implements message.Payload.
func (p *RemoveReply) Schema() message.Type { return RemoveReplyType }

// Validate implements message.Payload.
func (p *RemoveReply) Validate() error { return nil }

// MarshalJSON implements json.Marshaler.
func (p *RemoveReply) MarshalJSON() ([]byte, error) {
	type Alias RemoveReply
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *RemoveReply) UnmarshalJSON(data []byte) error {
	type Alias RemoveReply
	return json.Unmarshal(data, (*Alias)(p))
}
