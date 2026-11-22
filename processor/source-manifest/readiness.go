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
	}
}

// embeddingIndexLabel names graph-embedding's half of the tier 1 semantic index.
const embeddingIndexLabel = "Embedding index"

// foldSemanticReadiness folds the word index's and graph-embedding's readiness
// into the one tier 1 semantic-index signal: ready only when both are, else the
// half that is unavailable or behind, so the reason and revisions shown are
// the ones holding it back. graph-embedding is checked first because its gap
// is the one search hides: graph-query answers words alone.
//
// Readiness is index coverage, not provider liveness (owner ruling, #234). An
// embedding that failed while the provider was down leaves graph-embedding
// degraded and not ready (semengine graph/follower envelope), but a provider
// that goes down after the index caught up, with no new write to embed, still
// reads ready here. Liveness is reported per answer: merge.honoured on
// graph_search; fusion provenance cannot tell provider loss apart
// (semengine#723).
func foldSemanticReadiness(lexical, embedding workbenchIndexReadiness) workbenchIndexReadiness {
	halves := []workbenchIndexReadiness{embedding, lexical}
	for _, half := range halves {
		if !half.Available {
			return half
		}
	}
	for _, half := range halves {
		if !half.Ready {
			return half
		}
	}
	return lexical
}

// SemanticReadinessJSON is the tier 1 form of IndexReadinessJSON for the
// semantic index: the word index's and graph-embedding's envelopes (or the
// errors fetching them) folded by foldSemanticReadiness.
func SemanticReadinessJSON(lexicalRaw []byte, lexicalErr error, embeddingRaw []byte, embeddingErr error,
	label string,
) json.RawMessage {
	obj := foldSemanticReadiness(indexReadinessFrom(lexicalRaw, lexicalErr, label),
		indexReadinessFrom(embeddingRaw, embeddingErr, embeddingIndexLabel))
	data, err := json.Marshal(obj)
	if err != nil {
		return json.RawMessage(`{"available":false,"reason":{"code":"marshal_failed","message":"readiness marshal failed","retryable":true}}`)
	}
	return data
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

// disabledIndexReadiness returns the canonical readiness object for an index
// producer that is intentionally omitted from the composition (e.g.
// graph.lexical_search: false).
func disabledIndexReadiness(label string) workbenchIndexReadiness {
	return workbenchIndexReadiness{
		Available: false,
		Ready:     false,
		State:     "disabled",
		Reason: &workbenchCapabilityReason{
			Code:      "not_enabled",
			Message:   label + " is not enabled in this composition",
			Retryable: false,
		},
	}
}

// DisabledIndexReadinessJSON is the exported JSON form of disabledIndexReadiness
// for status surfaces such as the MCP gateway.
func DisabledIndexReadinessJSON(label string) json.RawMessage {
	data, err := json.Marshal(disabledIndexReadiness(label))
	if err != nil {
		return json.RawMessage(`{"available":false,"state":"disabled","reason":{"code":"not_enabled","message":"not enabled","retryable":false}}`)
	}
	return data
}
