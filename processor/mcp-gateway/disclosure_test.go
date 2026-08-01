package mcpgateway

import (
	"encoding/json"
	"strings"
	"testing"
)

// The three rungs of the capability ladder, as the substrate actually emits
// them. Recorded shapes rather than a live clustered stack: what is under test
// is SemSource's derivation, not semstreams' clustering.
const (
	// No clustering: entity digests only. Synthesis needs community summaries,
	// so there is no answer at all.
	respEntitiesOnly = `{"entities":[],"entity_digests":[{"id":"acme.semsource.golang.x.function.Foo","relevance":0.7}],"count":1,"duration_ms":12}`

	// Clustering, no LLM: community summaries plus the TEMPLATE answer. Note
	// degraded is FALSE — the template is canonical for this operator.
	respCommunityTemplate = `{"entity_digests":[{"id":"acme.semsource.golang.x.function.Foo"}],"community_summaries":[{"community_id":"c-1","summary":"Readiness gating","keywords":["ready"],"level":0,"member_count":9}],"answer":"The graph contains 9 entities about readiness gating.","count":1,"duration_ms":30}`

	// Clustering + LLM: answer_model names the endpoint that produced the prose.
	respCommunityLLM = `{"entity_digests":[{"id":"acme.semsource.golang.x.function.Foo"}],"community_summaries":[{"community_id":"c-1","summary":"Readiness gating","level":0}],"answer":"Readiness gating works by ...","answer_model":"qwen3-8b","count":1,"duration_ms":900}`

	// LLM configured but synthesis failed: template text WITH degraded set.
	respLLMDegraded = `{"community_summaries":[{"community_id":"c-1","summary":"x","level":0}],"answer":"The graph contains 9 entities.","degraded":true,"degraded_reason":"answer_synthesis_timeout","count":1}`

	// searchGraph's own fallback: globalSearch came back empty, so results are
	// semantic hits. degraded is true, but for a RETRIEVAL reason.
	respSemanticFallback = `{"strategy":"semantic_fallback","entity_digests":[{"id":"acme.semsource.golang.x.function.Foo","relevance":0.4}],"count":1,"degraded":true,"degraded_reason":"global_search_empty_semantic_fallback"}`
)

func TestDeriveDisclosureAcrossTheCapabilityLadder(t *testing.T) {
	tests := []struct {
		name             string
		raw              string
		wantRung         string
		wantCommunity    bool
		wantAnswer       string
		wantModel        string
		wantNoteFragment string
	}{
		{
			name:             "no clustering yields entity hits only",
			raw:              respEntitiesOnly,
			wantRung:         rungEntitiesOnly,
			wantCommunity:    false,
			wantAnswer:       answerSourceNone,
			wantNoteFragment: "NOT community-backed",
		},
		{
			name:             "clustering without an LLM yields a template answer",
			raw:              respCommunityTemplate,
			wantRung:         rungCommunity,
			wantCommunity:    true,
			wantAnswer:       answerSourceTemplate,
			wantNoteFragment: "template floor",
		},
		{
			name:             "clustering with an LLM yields a synthesized answer",
			raw:              respCommunityLLM,
			wantRung:         rungLLMAnswer,
			wantCommunity:    true,
			wantAnswer:       answerSourceLLM,
			wantModel:        "qwen3-8b",
			wantNoteFragment: "synthesized by qwen3-8b",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveDisclosure([]byte(tc.raw))
			if got.Rung != tc.wantRung {
				t.Errorf("rung = %q, want %q", got.Rung, tc.wantRung)
			}
			if got.CommunityBacked != tc.wantCommunity {
				t.Errorf("community_backed = %v, want %v", got.CommunityBacked, tc.wantCommunity)
			}
			if got.AnswerSource != tc.wantAnswer {
				t.Errorf("answer_source = %q, want %q", got.AnswerSource, tc.wantAnswer)
			}
			if got.AnswerModel != tc.wantModel {
				t.Errorf("answer_model = %q, want %q", got.AnswerModel, tc.wantModel)
			}
			if !strings.Contains(got.Note, tc.wantNoteFragment) {
				t.Errorf("note = %q, want it to contain %q", got.Note, tc.wantNoteFragment)
			}
		})
	}
}

// TestTemplateAnswerIsNeverReportedAsLLM pins the trap this whole disclosure
// exists to avoid. The substrate sets degraded=true ONLY when an LLM-configured
// deployment falls back; a deployment with no LLM returns a template answer with
// degraded=false, because the template is the canonical answer there. So any
// derivation that reads `degraded` as the "was this an LLM answer" flag reports
// every template answer on a no-LLM stack as LLM-synthesized. `answer_model` is
// the only discriminator.
func TestTemplateAnswerIsNeverReportedAsLLM(t *testing.T) {
	got := deriveDisclosure([]byte(respCommunityTemplate))

	if got.Degraded {
		t.Fatalf("fixture drift: the no-LLM template response must carry degraded=false, got %+v", got)
	}
	if got.AnswerSource == answerSourceLLM || got.Rung == rungLLMAnswer {
		t.Errorf("a template answer was reported as LLM-synthesized: %+v", got)
	}
	if got.AnswerModel != "" {
		t.Errorf("a template answer must have no model attribution, got %q", got.AnswerModel)
	}
	if strings.Contains(got.Note, "synthesized by") {
		t.Errorf("note claims LLM synthesis for a template answer: %q", got.Note)
	}
}

// TestDegradedSynthesisStaysVisible covers the other half: when an LLM WAS
// configured and fell back, the caller must be able to tell that per-query
// synthesis was expected and not delivered.
func TestDegradedSynthesisStaysVisible(t *testing.T) {
	got := deriveDisclosure([]byte(respLLMDegraded))

	if !got.Degraded || got.DegradedReason != "answer_synthesis_timeout" {
		t.Errorf("degraded flag/reason lost: %+v", got)
	}
	if got.AnswerSource != answerSourceTemplate {
		t.Errorf("answer_source = %q, want %q — the delivered text is the template", got.AnswerSource, answerSourceTemplate)
	}
	if !strings.Contains(got.Note, "fell back to the template") {
		t.Errorf("note does not explain the fallback: %q", got.Note)
	}
}

// TestSemanticFallbackIsNotNarratedAsFailedSynthesis distinguishes the two
// meanings of `degraded`. searchGraph sets it for a RETRIEVAL fallback
// (globalSearch returned empty), which has nothing to do with an LLM — telling
// the agent an LLM failed there would be a fabricated explanation.
func TestSemanticFallbackIsNotNarratedAsFailedSynthesis(t *testing.T) {
	got := deriveDisclosure([]byte(respSemanticFallback))

	if got.Rung != rungEntitiesOnly || got.CommunityBacked {
		t.Errorf("semantic fallback must not read as community-backed: %+v", got)
	}
	if strings.Contains(got.Note, "LLM was configured") {
		t.Errorf("note blames an LLM for a retrieval fallback: %q", got.Note)
	}
	if !strings.Contains(got.Note, "semantic-similarity fallback") {
		t.Errorf("note does not explain the retrieval fallback: %q", got.Note)
	}
	// The substrate populates strategy on exactly this path; when present it is
	// authoritative and must be carried through.
	if got.Strategy != "semantic_fallback" {
		t.Errorf("substrate-reported strategy dropped: %+v", got)
	}
}

// TestDisclosureNeverFailsOnUnreadableResponse: an unparseable payload must
// downgrade the claim, not error the tool call — and it must not silently
// present itself as a community-backed answer.
func TestDisclosureNeverFailsOnUnreadableResponse(t *testing.T) {
	got := deriveDisclosure([]byte(`not json at all`))

	if got.Rung != rungUnknown {
		t.Errorf("rung = %q, want %q", got.Rung, rungUnknown)
	}
	if got.CommunityBacked {
		t.Error("an unreadable response must never be reported as community-backed")
	}
	if got.Note == "" {
		t.Error("an unknown rung must still explain itself")
	}
}

func jsonEqual(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}
