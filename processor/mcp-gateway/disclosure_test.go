package mcpgateway

import (
	"encoding/json"
	"strings"
	"testing"

	gtypes "github.com/c360studio/semengine/graph"
)

// Recorded alpha.5 searchGraph replies. What is under test is SemSource's
// derivation, not SemEngine's grouping.
const (
	// No clustering: every group answers not_ready, so no record is read.
	replyNoClustering = `{"data":{"text":{"level":0,"groups":[{"community_id":"acme.semsource.golang","matches":1,"record_state":"not_ready"}],
		"matches":[{"id":"acme.semsource.golang.x.function.Foo","score":0.7,"entity":{"id":"acme.semsource.golang.x.function.Foo"}}],
		"matched":1,"missing":0,"bodies_not_indexed":0,"truncated":false,"unreadable_groups":0,"answer":"1 match for readiness."},
		"hints":[{"hint":"time_range","detail":"last 2 hours","honoured":false,"reason":"answered by words only"}]},
		"indexed_revision":12,"producer":"graph-query"}`

	// Clustering on: the group's COMMUNITY_INDEX record was read.
	replyCommunity = `{"data":{"text":{"level":1,"groups":[{"community_id":"acme.semsource.golang","matches":1,"record_state":"read",
		"record":{"member_count":9,"community_summary":"Readiness gating","keywords":["ready"]}}],
		"matches":[{"id":"acme.semsource.golang.x.function.Foo","score":0.7,"entity":{"id":"acme.semsource.golang.x.function.Foo"}}],
		"matched":1,"missing":0,"bodies_not_indexed":0,"truncated":false,"unreadable_groups":0,"answer":"1 match in 1 community."},
		"hints":[]},"indexed_revision":12,"producer":"graph-query"}`
)

func decodeReply(t *testing.T, raw string) graphSearchReply {
	t.Helper()
	var r graphSearchReply
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("decode recorded reply: %v", err)
	}
	return r
}

// spec: openspec/specs/graphrag-access/spec.md#Requirement: An answer discloses how far it escalated
func TestDeriveDisclosureAcrossTheCapabilityLadder(t *testing.T) {
	tests := []struct {
		name             string
		raw              string
		wantRung         string
		wantCommunity    bool
		wantNoteFragment string
	}{
		{"no clustering yields word-search hits only", replyNoClustering, rungEntitiesOnly, false, "NOT community-backed"},
		{"a read community record backs the answer", replyCommunity, rungCommunity, true, "Community-backed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := decodeReply(t, tc.raw)
			got := deriveDisclosure(&r.Data, deriveCommunities(r.Data.Text))
			if got.Rung != tc.wantRung || got.CommunityBacked != tc.wantCommunity {
				t.Errorf("rung=%q community_backed=%v, want %q/%v", got.Rung, got.CommunityBacked, tc.wantRung, tc.wantCommunity)
			}
			// The substrate's answer is a template built from the reply's own
			// fields; it must never be presented as model prose.
			if got.AnswerSource != answerSourceTemplate || !strings.Contains(got.Note, "not LLM-synthesized") {
				t.Errorf("answer_source=%q note=%q, want the template disclosed as such", got.AnswerSource, got.Note)
			}
			if !strings.Contains(got.Note, tc.wantNoteFragment) {
				t.Errorf("note = %q, want it to contain %q", got.Note, tc.wantNoteFragment)
			}
		})
	}
}

// Community attribution survives the gateway with its level, since an
// identifier resolves only within its level; a not_ready group is no evidence.
// spec: openspec/specs/graphrag-access/spec.md#Requirement: Results are citable
func TestCommunityAttributionCarriesLevel(t *testing.T) {
	got := deriveCommunities(decodeReply(t, replyCommunity).Data.Text)
	want := []communitySummary{{CommunityID: "acme.semsource.golang", Level: 1, MemberCount: 9, Matches: 1,
		Summary: "Readiness gating", Keywords: []string{"ready"}}}
	if !jsonEqual(got, want) {
		t.Errorf("communities = %+v, want %+v", got, want)
	}
	if got := deriveCommunities(decodeReply(t, replyNoClustering).Data.Text); got != nil {
		t.Errorf("a not_ready group was reported as a community: %+v", got)
	}
}

// TestDisclosureWithoutAnswerText: an empty answer is disclosed as absent, not
// as a template.
func TestDisclosureWithoutAnswerText(t *testing.T) {
	got := deriveDisclosure(&gtypes.GlobalSearchData{Text: &gtypes.TextSearchData{}}, nil)
	if got.AnswerSource != answerSourceNone || got.Rung != rungEntitiesOnly {
		t.Errorf("disclosure = %+v, want no answer and entities_only", got)
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
