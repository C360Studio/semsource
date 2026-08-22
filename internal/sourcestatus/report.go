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
	// EntityCount is the DISTINCT entity count (invariant under periodic
	// republication); PublishTotal is raw publish throughput — separately
	// named so counts are never a readiness proxy.
	EntityCount  int64 `json:"entity_count"`
	PublishTotal int64 `json:"publish_total,omitempty"`
	// AcceptedTotal, DeliveredTotal and LostTotal are the delivery figures,
	// named separately because acceptance is not arrival: Send() returns
	// after a buffer write, before any delivery outcome exists. A single
	// combined figure cannot answer the only question asked of it — whether
	// the corpus actually arrived — and overstates delivery under loss.
	//
	// They reconcile as accepted = delivered + lost + in-flight. None carry
	// omitempty: a zero LostTotal is the assertion "nothing was lost", which
	// is not the same claim as an absent field.
	AcceptedTotal  int64 `json:"accepted_total"`
	DeliveredTotal int64 `json:"delivered_total"`
	LostTotal      int64 `json:"lost_total"`
	// FilesParsed and BodiesOffloaded are pre-publish seed liveness: they
	// advance during parse and body-offload windows where PublishTotal is
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
	LastError  *seedsup.Error    `json:"last_error,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
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
