package mcpgateway

import (
	"strings"

	gtypes "github.com/c360studio/semengine/graph"
)

// Retrieval rungs. A graph-query answer escalates with what the running stack
// provides, and the tool must say which rung it reached — an agent that reads a
// word-search hit list as community/thematic reasoning draws conclusions the
// evidence does not support. The pinned substrate synthesizes no LLM answer
// (semengine#485), so the ladder stops at community records.
const (
	rungEntitiesOnly = "entities_only"
	rungCommunity    = "community_summaries"
	rungUnknown      = "unknown"
)

// Answer sources. The substrate's answer is a plain text built from the reply's
// own fields — a template, never model prose.
const (
	answerSourceNone     = "none"
	answerSourceTemplate = "template"
)

// retrievalDisclosure states how far an answer escalated. It is derived only
// from fields the substrate returns — SemSource computes no membership, summary,
// or ranking of its own.
type retrievalDisclosure struct {
	Rung            string `json:"rung"`
	CommunityBacked bool   `json:"community_backed"`
	AnswerSource    string `json:"answer_source"`
	Note            string `json:"note"`
}

// unknownDisclosure is the disclosure for a reply that could not be read.
func unknownDisclosure() retrievalDisclosure {
	return retrievalDisclosure{
		Rung:         rungUnknown,
		AnswerSource: answerSourceNone,
		Note: "Retrieval path unknown: the substrate response could not be inspected. " +
			"Treat this answer as unattributed rather than community-backed.",
	}
}

// deriveDisclosure states the rung a search reply reached: community-backed
// exactly when at least one matching group's COMMUNITY_INDEX record was read.
// A deployment without clustering answers every group not_ready, so its
// matches are word-search hits only.
func deriveDisclosure(data *gtypes.GlobalSearchData, communities []communitySummary) retrievalDisclosure {
	d := retrievalDisclosure{
		Rung:            rungEntitiesOnly,
		CommunityBacked: len(communities) > 0,
		AnswerSource:    answerSourceNone,
	}
	if d.CommunityBacked {
		d.Rung = rungCommunity
	}
	if data.Text != nil && data.Text.Answer != "" {
		d.AnswerSource = answerSourceTemplate
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
		b.WriteString("Community-backed: matches are grouped by graph communities whose records are included. ")
	} else {
		b.WriteString("NOT community-backed: these are word-search hits, not community or thematic " +
			"reasoning — enable graph.enable_clustering for community-backed answers. ")
	}
	if d.AnswerSource == answerSourceTemplate {
		b.WriteString("The answer text is a deterministic template built from these results, not LLM-synthesized.")
	} else {
		b.WriteString("No answer text was returned.")
	}
	return b.String()
}
