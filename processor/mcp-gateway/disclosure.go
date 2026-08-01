package mcpgateway

import (
	"encoding/json"
	"strings"
)

// Retrieval rungs. A graph-query answer escalates with what the running stack
// provides, and the tool must say which rung it reached — an agent that reads a
// similarity hit list as community/thematic reasoning draws conclusions the
// evidence does not support.
const (
	rungEntitiesOnly = "entities_only"
	rungCommunity    = "community_summaries"
	rungLLMAnswer    = "llm_answer"
	rungUnknown      = "unknown"
)

// Answer sources. The substrate always populates an answer once community
// summaries exist — via the LLM when one is configured, via the template floor
// otherwise — so "an answer is present" says nothing about how it was made.
const (
	answerSourceNone     = "none"
	answerSourceTemplate = "template"
	answerSourceLLM      = "llm"
)

// substrateSemanticFallbackReason is the substrate's degraded_reason when
// searchGraph's globalSearch pass returned empty and it fell back to semantic
// search. It is a RETRIEVAL fallback, not an answer-synthesis one, so it must
// not be narrated as a failed LLM call.
const substrateSemanticFallbackReason = "global_search_empty_semantic_fallback"

// graphSearchFields is the subset of the substrate's GlobalSearchResponse the
// disclosure is derived from. Every field is optional across tiers, so decoding
// is lenient: a field we cannot read lowers the disclosure's confidence, it never
// fails the tool call.
type graphSearchFields struct {
	// Strategy is the substrate's own account of which path answered. It is
	// authoritative when present, but at the pinned semstreams version it is
	// populated on exactly one path (searchGraph's semantic fallback) — hence
	// the derived fields below. See the upstream ask.
	Strategy           string            `json:"strategy"`
	CommunitySummaries []json.RawMessage `json:"community_summaries"`
	Answer             string            `json:"answer"`
	AnswerModel        string            `json:"answer_model"`
	Degraded           bool              `json:"degraded"`
	DegradedReason     string            `json:"degraded_reason"`
}

// retrievalDisclosure states how far an answer escalated. It is derived only
// from fields the substrate returns — SemSource computes no membership, summary,
// or ranking of its own.
type retrievalDisclosure struct {
	Rung            string `json:"rung"`
	CommunityBacked bool   `json:"community_backed"`
	AnswerSource    string `json:"answer_source"`
	AnswerModel     string `json:"answer_model,omitempty"`
	Degraded        bool   `json:"degraded,omitempty"`
	DegradedReason  string `json:"degraded_reason,omitempty"`
	Strategy        string `json:"strategy,omitempty"`
	Note            string `json:"note"`
}

// disclosedResult is what a multi-path graph-query tool returns: the substrate's
// response VERBATIM under `result`, with the derived disclosure beside it under
// `retrieval`. The disclosure never merges into or rewrites the substrate
// payload, so once the substrate populates `strategy` on every path a consumer
// can simply prefer its value over our derived rung — the two can disagree
// without either being corrupted.
type disclosedResult struct {
	Retrieval retrievalDisclosure `json:"retrieval"`
	Result    json.RawMessage     `json:"result"`
}

// deriveDisclosure reads the substrate's response and states the rung reached.
//
// The load-bearing rule is that `answer_model` — NOT `degraded` — discriminates
// an LLM answer from the template floor. A deployment with no LLM configured
// returns a template answer with degraded=false deliberately: the template IS
// the canonical answer for that operator, so degraded is reserved for an
// LLM-configured deployment that fell back. Treating degraded as the LLM flag
// would report every template answer on a no-LLM stack as LLM-synthesized.
func deriveDisclosure(raw []byte) retrievalDisclosure {
	var f graphSearchFields
	if err := json.Unmarshal(raw, &f); err != nil {
		return retrievalDisclosure{
			Rung:         rungUnknown,
			AnswerSource: answerSourceNone,
			Note: "Retrieval path unknown: the substrate response could not be inspected. " +
				"Treat this answer as unattributed rather than community-backed.",
		}
	}

	d := retrievalDisclosure{
		CommunityBacked: len(f.CommunitySummaries) > 0,
		AnswerModel:     f.AnswerModel,
		Degraded:        f.Degraded,
		DegradedReason:  f.DegradedReason,
		Strategy:        f.Strategy,
	}

	switch {
	case f.AnswerModel != "":
		d.AnswerSource = answerSourceLLM
	case f.Answer != "":
		d.AnswerSource = answerSourceTemplate
	default:
		d.AnswerSource = answerSourceNone
	}

	switch {
	case d.AnswerSource == answerSourceLLM:
		d.Rung = rungLLMAnswer
	case d.CommunityBacked:
		d.Rung = rungCommunity
	default:
		d.Rung = rungEntitiesOnly
	}

	d.Note = disclosureNote(d)
	return d
}

// disclosureNote renders the disclosure as a sentence an agent reads directly.
// The structured fields are for routing; this is what stops a thin answer from
// being read as a rich one.
func disclosureNote(d retrievalDisclosure) string {
	var b strings.Builder

	if d.CommunityBacked {
		b.WriteString("Community-backed: this answer drew on graph community summaries. ")
	} else {
		b.WriteString("NOT community-backed: these are entity retrieval hits, not community " +
			"or thematic reasoning — enable graph.enable_clustering for community-backed answers. ")
	}

	switch d.AnswerSource {
	case answerSourceLLM:
		b.WriteString("The answer text was synthesized by " + d.AnswerModel + ".")
	case answerSourceTemplate:
		b.WriteString("The answer text is the deterministic template floor, not LLM-synthesized — " +
			"enable graph.clustering_llm with a community_summary model capability for per-query synthesis.")
	default:
		b.WriteString("No synthesized answer: synthesis requires community summaries.")
	}

	if d.Degraded {
		if d.DegradedReason == substrateSemanticFallbackReason {
			b.WriteString(" Thematic search returned nothing, so these results came from a " +
				"semantic-similarity fallback.")
		} else {
			b.WriteString(" The substrate reported degraded synthesis (" + d.DegradedReason +
				"): an LLM was configured but the answer fell back to the template.")
		}
	}

	return b.String()
}
