package sourcemanifest

import "encoding/json"

// ReadinessNote is the canonical prose contract for the readiness signals,
// shared by the MCP source_status tool and the HTTP status endpoint. The
// "miss means genuine absence" guarantee is scoped to BOTH gates: the
// aggregate phase must be ready (every source finished its initial seed) and
// the relevant index signal must be ready — during seeding a miss may simply
// be not-yet-ingested (audit 2026-07-19, honest-readiness-and-errors).
const ReadinessNote = "readiness is honest (semstreams ADR-066): index.ready means the structural " +
	"index (NAME_INDEX) has caught up to the latest committed write — byName / code_context / " +
	"code_impact are reliable. embedding.ready means the semantic pipeline has caught up — code_search " +
	"is reliable (surfaced, not gated). A miss is a genuine absence only when status.phase is \"ready\" " +
	"(every configured source has finished its initial seed) AND the relevant index signal is ready; " +
	"while status.phase is \"seeding\" a miss may simply be not-yet-ingested. For exact read-your-write " +
	"freshness, compare the numeric indexed_revision field against your write's revision."

// indexReadinessFrom converts a raw graph index/embedding status reply (or the
// error from fetching it) into the canonical readiness object. Failures yield
// an explicit {available:false, reason} object — the signal agents gate on is
// never silently omitted.
func indexReadinessFrom(raw []byte, err error, label string) workbenchIndexReadiness {
	if err != nil || len(raw) == 0 {
		return unavailableReadiness(label + " readiness is unavailable")
	}
	var status workbenchIndexStatusWire
	if uErr := json.Unmarshal(raw, &status); uErr != nil {
		return unavailableReadiness(label + " readiness is unavailable")
	}
	state := status.State
	if state == "" {
		state = readinessUnknown
	}
	return workbenchIndexReadiness{
		Available:       true,
		Ready:           status.Ready,
		State:           state,
		IndexedRevision: status.IndexedRevision,
		TargetRevision:  status.TargetRevision,
		Lag:             status.Lag,
		Revision:        status.Revision,
		LastSynced:      status.LastSynced,
	}
}

// IndexReadinessJSON is the exported form of indexReadinessFrom for the other
// status surfaces (MCP gateway): one canonical readiness shape everywhere.
func IndexReadinessJSON(raw []byte, err error, label string) json.RawMessage {
	obj := indexReadinessFrom(raw, err, label)
	data, mErr := json.Marshal(obj)
	if mErr != nil {
		return json.RawMessage(`{"available":false,"reason":{"code":"marshal_failed","message":"readiness marshal failed","retryable":true}}`)
	}
	return data
}
