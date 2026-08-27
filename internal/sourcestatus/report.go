// Package sourcestatus is the single wire contract for source status
// reports on semsource.internal.status.
//
// Every source component constructs Report; source-manifest decodes Report —
// the same Go type on both sides, strictly (unknown fields reject the
// report). Re-declaring this shape anywhere (inline structs, mirrored types)
// is the defect that produced #188: nine hand-synced duck-typed copies, and
// json.Unmarshal silently dropping the fields the consumer's copy lacked.
package sourcestatus

import (
	"time"

	"github.com/c360studio/semsource/internal/seedsup"
)

// Report is one source instance's status report. It carries the full field
// union across source types; producers populate what they measure and
// omitempty keeps the wire lean. Add fields HERE — never on a producer-side
// literal — so the aggregator can never lose them.
type Report struct {
	InstanceName string `json:"instance_name"`
	SourceType   string `json:"source_type"`
	Phase        string `json:"phase"`
	// EntityCount is the DISTINCT entity count, invariant under periodic
	// republication, so counts are never a readiness proxy.
	EntityCount int64 `json:"entity_count"`
	// OfferedTotal, DeliveredTotal and LostTotal are the delivery figures,
	// named separately because offering is not arrival: Send() returns after
	// a buffer write, before any delivery outcome exists. A single combined
	// figure cannot answer the only question asked of it — whether the corpus
	// actually arrived — and overstates delivery under loss.
	//
	// OfferedTotal counts every entity the source handed to its publisher,
	// including those the publisher then refused on overflow. Counting only
	// the ones it accepted would leave drops on one side of the arithmetic:
	// they are in LostTotal, so they must be in OfferedTotal too.
	//
	// They reconcile exactly as offered = delivered + lost + in-flight. None
	// carry omitempty: a zero LostTotal is the assertion "nothing was lost",
	// which is not the same claim as an absent field.
	OfferedTotal   int64 `json:"offered_total"`
	DeliveredTotal int64 `json:"delivered_total"`
	LostTotal      int64 `json:"lost_total"`
	// SeedLost is the loss attributable to the most recently COMPLETED seed
	// pass, as distinct from LostTotal's process lifetime. The publisher's
	// counters are monotonic, so lifetime loss can never fall back to zero;
	// without a per-pass figure a loss-degraded readiness phase could never
	// be cleared by a clean re-seed.
	SeedLost int64 `json:"seed_lost"`
	// FilesParsed and BodiesOffloaded are pre-publish seed liveness: they
	// advance during parse and body-offload windows where DeliveredTotal is
	// flat, distinguishing a working seed from a wedged one.
	FilesParsed     int64 `json:"files_parsed,omitempty"`
	BodiesOffloaded int64 `json:"bodies_offloaded,omitempty"`
	// BoundariesSkipped counts nested git trees the ingest walk refused to
	// enter (submodules, foreign repos) — informational: those trees are
	// other sources' scope.
	BoundariesSkipped int64            `json:"boundaries_skipped,omitempty"`
	ErrorCount        int64            `json:"error_count"`
	TypeCounts        map[string]int64 `json:"type_counts,omitempty"`
	// Backpressure is true while the entity publisher is retrying against a
	// refusing or saturated transport: no drops, no failures, no errors —
	// functionally stalled. The flag is what separates "slow" from
	// "stalled" without reading logs.
	Backpressure bool `json:"backpressure,omitempty"`
	// Submodules lists every submodule path a repo source declares and its
	// state — missing code is never silent (git-submodule-ingestion spec).
	Submodules []SubmoduleStatus `json:"submodules,omitempty"`
	// ObjectsSkipped counts objects an object-store source enumerated and did
	// not ingest, keyed by reason: unsupported_format, empty_object,
	// unreadable. It is the difference between "there is no such document"
	// and "that document was never parsed", which no other field on this
	// report can express.
	//
	// Counts rather than the keys themselves, because a bucket's unparseable
	// objects are unbounded — a corpus of reports beside ten thousand images
	// would put ten thousand strings on every status surface — and because
	// these are the figures for the LAST COMPLETED PASS, not a lifetime
	// total. A skip repeats every pass by nature (an unsupported extension
	// stays unsupported), so a running total would climb forever while
	// describing the same few objects.
	ObjectsSkipped map[string]int64 `json:"objects_skipped,omitempty"`
	LastError      *seedsup.Error   `json:"last_error,omitempty"`
	Timestamp      time.Time        `json:"timestamp"`
}

// SubmoduleStatus describes one declared submodule path of a repo source.
type SubmoduleStatus struct {
	// Path is the submodule working-tree path relative to the repo root.
	Path string `json:"path"`
	// SHA is the pinned gitlink commit (12-hex short form); empty for a
	// stale declaration with no gitlink.
	SHA string `json:"sha,omitempty"`
	// State is one of: materialized, unmaterialized, excluded_by_config,
	// declared_but_absent, beyond_cap.
	State string `json:"state"`
}
