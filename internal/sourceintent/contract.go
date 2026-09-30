// Package sourceintent defines the product-owned source lifecycle contracts shared
// by desired-state coordination, publication receipts, and graph projection.
package sourceintent

import (
	"context"
	"errors"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/types"
)

// Version is the journal and receipt schema version.
const Version = 1

// ErrNotFound means the exact requested operational record is absent.
var ErrNotFound = errors.New("source lifecycle record not found")

// ErrConflict means a journal revision changed; callers must reread before retrying.
var ErrConflict = errors.New("source lifecycle revision conflict")

// Binding identifies one source intent and one admitted publication epoch. A new
// boot advances BootEpoch before any publisher can issue eligible receipts.
type Binding struct {
	Authority    entityid.Authority `json:"authority"`
	Namespace    string             `json:"namespace"`
	Handle       string             `json:"handle"`
	Generation   uint64             `json:"generation"`
	BootEpoch    string             `json:"boot_epoch"`
	Factory      string             `json:"factory"`
	ConfigDigest string             `json:"config_digest"`
}

// Operation is the graph effect requested by the current desired source intent.
type Operation string

const (
	// Remove retains source entities and marks them source_removed.
	Remove Operation = "remove"
	// Reactivate clears source_removed only for current acknowledged source facts.
	Reactivate Operation = "reactivate"
)

// Phase describes durable execution state, not runtime component admission.
type Phase string

const (
	// Prepared precedes the desired component and manifest writes.
	Prepared Phase = "prepared"
	// Pending means the intent still requires retirement, publication, or projection proof.
	Pending Phase = "pending"
	// Complete means every required effect was verified for this generation.
	Complete Phase = "complete"
	// Superseded records that a newer desired operation invalidated the old work.
	Superseded Phase = "superseded"
)

// ErrorCode is a machine-readable blocker; callers never parse error messages.
type ErrorCode string

const (
	CodeStorage             ErrorCode = "storage_error"
	CodeCorruptRecord       ErrorCode = "corrupt_record"
	CodeOwnership           ErrorCode = "ownership_mismatch"
	CodeDesiredWrite        ErrorCode = "desired_write_failed"
	CodeDesiredAmbiguous    ErrorCode = "desired_state_ambiguous"
	CodeRetirement          ErrorCode = "retirement_unproven"
	CodeTailUnavailable     ErrorCode = "tail_unavailable"
	CodeTailBacklog         ErrorCode = "tail_backlog"
	CodeTailChanged         ErrorCode = "tail_identity_changed"
	CodeTailPolicy          ErrorCode = "tail_policy_unsupported"
	CodeTailDegraded        ErrorCode = "tail_degraded"
	CodeAmbiguousScope      ErrorCode = "ambiguous_source_scope"
	CodeUnsupportedScope    ErrorCode = "unsupported_source_scope"
	CodeEnumeration         ErrorCode = "enumeration_failed"
	CodeMutation            ErrorCode = "mutation_failed"
	CodeSeedIncomplete      ErrorCode = "seed_incomplete"
	CodeReceipt             ErrorCode = "receipt_failed"
	CodeSourceFactsMismatch ErrorCode = "source_facts_mismatch"
	CodeCanceled            ErrorCode = "canceled"
)

// Blocker preserves a typed failure and its human-readable diagnostic.
type Blocker struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
}

func (b *Blocker) Error() string { return string(b.Code) + ": " + b.Message }

// Selector identifies an exact source-owned taxonomy under one system. Prefix
// ends at a segment boundary; artifact restrictions narrow it when ownership is
// provable from retained source facts. An empty artifact list selects the prefix.
type Selector struct {
	Prefix            string   `json:"prefix"`
	System            string   `json:"system"`
	Domain            string   `json:"domain"`
	ArtifactPredicate string   `json:"artifact_predicate,omitempty"`
	Artifacts         []string `json:"artifacts,omitempty"`
}

// StreamInput names an admitted graph-ingest input; identities come from its
// configured port and observed consumer, never from a broad wildcard guess.
type StreamInput struct {
	Stream   string   `json:"stream"`
	Consumer string   `json:"consumer"`
	Filters  []string `json:"filters"`
}

// SourceScope captures producer identity before disable and preserves the exact
// admitted graph inputs whose previous-process work must settle before replay.
type SourceScope struct {
	Authority    entityid.Authority `json:"authority"`
	Handle       string             `json:"handle"`
	Factory      string             `json:"factory"`
	ConfigDigest string             `json:"config_digest"`
	Selectors    []Selector         `json:"selectors"`
	Inputs       []StreamInput      `json:"inputs"`
}

// InputEvidence binds a tail observation to the exact stream and consumer
// creation identities and the qualified policies that make zero backlog useful.
type InputEvidence struct {
	Input           StreamInput `json:"input"`
	StreamCreated   time.Time   `json:"stream_created"`
	ConsumerCreated time.Time   `json:"consumer_created"`
	Storage         string      `json:"storage"`
	Replicas        int         `json:"replicas"`
	AckPolicy       string      `json:"ack_policy"`
	DeliverPolicy   string      `json:"deliver_policy"`
	Pending         uint64      `json:"pending"`
	AckPending      int         `json:"ack_pending"`
}

// TailEvidence is a current authoritative observation, not a cached readiness
// claim. The prepared record retains the original identities across restarts.
type TailEvidence struct {
	Inputs     []InputEvidence `json:"inputs"`
	ObservedAt time.Time       `json:"observed_at"`
	Degraded   bool            `json:"degraded"`
}

// Progress reports honest partial work and the last completed unit.
type Progress struct {
	RetryCount     uint64    `json:"retry_count"`
	LastProgress   time.Time `json:"last_progress"`
	CompletedCount int       `json:"completed_count"`
	Blocker        *Blocker  `json:"blocker,omitempty"`
}

// SeedSeal is the bounded terminal record for a seed's per-entity manifest.
type SeedSeal struct {
	BatchID    string   `json:"batch_id"`
	Initial    bool     `json:"initial"`
	Successful bool     `json:"successful"`
	Count      int      `json:"count"`
	Digest     string   `json:"digest"`
	Failure    *Blocker `json:"failure,omitempty"`
}

// Record is the retained current intent for one source handle. Updates and
// completion require the KV revision returned by Get, never a blind overwrite.
type Record struct {
	Version               int                   `json:"version"`
	Binding               Binding               `json:"binding"`
	Operation             Operation             `json:"operation"`
	Phase                 Phase                 `json:"phase"`
	RetiredConfig         types.ComponentConfig `json:"retired_config"`
	Scope                 SourceScope           `json:"scope"`
	Tail                  TailEvidence          `json:"tail"`
	DesiredCommitted      bool                  `json:"desired_committed"`
	ReplacementGeneration uint64                `json:"replacement_generation,omitempty"`
	Seed                  *SeedSeal             `json:"seed,omitempty"`
	Progress              Progress              `json:"progress"`
	CreatedAt             time.Time             `json:"created_at"`
	UpdatedAt             time.Time             `json:"updated_at"`
}

// Publication carries immutable source evidence frozen before enqueue. The
// observer receives it only after transport acknowledges the entity publication.
type Publication struct {
	Binding Binding              `json:"binding"`
	BatchID string               `json:"batch_id"`
	Initial bool                 `json:"initial"`
	Payload *graph.EntityPayload `json:"payload"`
}

// ManifestEntry names one expected entity and its canonical source-fact digest.
type ManifestEntry struct {
	EntityID    string `json:"entity_id"`
	Fingerprint string `json:"fingerprint"`
}

// SeedManifest closes one explicit producer enumeration. Entries is an
// in-process value; storage writes separate bounded entries and a terminal seal.
type SeedManifest struct {
	Binding    Binding         `json:"binding"`
	BatchID    string          `json:"batch_id"`
	Initial    bool            `json:"initial"`
	Successful bool            `json:"successful"`
	Entries    []ManifestEntry `json:"entries"`
	Digest     string          `json:"digest"`
	Failure    *Blocker        `json:"failure,omitempty"`
}

// Receipt confirms transport acknowledgment for exact immutable source facts.
type Receipt struct {
	Binding      Binding `json:"binding"`
	BatchID      string  `json:"batch_id"`
	EntityID     string  `json:"entity_id"`
	Fingerprint  string  `json:"fingerprint"`
	Acknowledged bool    `json:"acknowledged"`
}

// EntityFailure attributes a partial projection failure to an exact entity.
type EntityFailure struct {
	EntityID string  `json:"entity_id"`
	Failure  Blocker `json:"failure"`
}

// ProjectionResult remains partial when any page or entity operation fails.
type ProjectionResult struct {
	Enumerated int             `json:"enumerated"`
	Marked     int             `json:"marked"`
	Cleared    int             `json:"cleared"`
	Complete   bool            `json:"complete"`
	Failures   []EntityFailure `json:"failures,omitempty"`
}

// RemovalProjection carries the exact current journal identity and retired scope.
type RemovalProjection struct {
	Binding Binding     `json:"binding"`
	Scope   SourceScope `json:"scope"`
}

// ReactivationProjection grants freshness only to this epoch's sealed current
// IDs whose acknowledged fingerprints also match authoritative graph facts.
type ReactivationProjection struct {
	Binding  Binding      `json:"binding"`
	Manifest SeedManifest `json:"manifest"`
	Receipts []Receipt    `json:"receipts"`
}

// Journal stores retained desired/projection facts and generation-bound receipts.
type Journal interface {
	Get(ctx context.Context, handle string) (Record, uint64, error)
	Create(ctx context.Context, record Record) (uint64, error)
	Update(ctx context.Context, record Record, revision uint64) (uint64, error)
	List(ctx context.Context) ([]Record, error)
	PutReceipt(ctx context.Context, receipt Receipt) error
	ListReceipts(ctx context.Context, binding Binding, batchID string) ([]Receipt, error)
	SealSeed(ctx context.Context, manifest SeedManifest) error
	LoadSeed(ctx context.Context, binding Binding, batchID string) (SeedManifest, error)
}

// Projector is the sole serialized graph lifecycle mutation owner.
type Projector interface {
	ApplyRemoval(ctx context.Context, request RemovalProjection) (ProjectionResult, error)
	ApplyReactivation(ctx context.Context, request ReactivationProjection) (ProjectionResult, error)
}

// TailObserver observes current settlement without taking consumer ownership.
type TailObserver interface {
	Settled(ctx context.Context, scope SourceScope) (TailEvidence, error)
}

// PublicationObserver persists bound post-ack evidence without acquiring the
// desired-operation gate. It remains usable throughout producer shutdown.
type PublicationObserver interface {
	Published(ctx context.Context, publication Publication) error
	SeedFinished(ctx context.Context, manifest SeedManifest) error
}

// SeedBatch owns the exact accepted publication set, including in-flight work.
// Every enumeration explicitly calls Finish, including failure and empty seeds.
type SeedBatch interface {
	Send(payload *graph.EntityPayload) error
	Finish(ctx context.Context, enumerationErr error) error
}

// BindablePublisher is injected before producer Start by the root's verified
// factory decorator; a pending reactivation without this capability fails boot.
type BindablePublisher interface {
	BindSourceLifecycle(binding Binding, observer PublicationObserver) error
}
