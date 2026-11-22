package mcpgateway

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The reply in testdata was recorded on 2026-11-17 from graph.query.searchGraph
// on semengine v0.1.0-alpha.6 with embedding_enabled unset, against the OSH
// corpus of the #301 regrade (scripts/scorecard/results/SUMMARY-alpha6-tier0.md).
// The request beside it is the exact payload sent.
// spec: openspec/specs/graphrag-access/spec.md#Requirement: An answer discloses how far it escalated
func TestRecordedAlpha6TierZeroReplyDecodes(t *testing.T) {
	raw, err := os.ReadFile("testdata/searchgraph-alpha6-tier0.reply.json")
	if err != nil {
		t.Fatal(err)
	}
	r := decodeReply(t, string(raw))
	if r.Data.Text == nil {
		t.Fatal("recorded tier 0 reply decoded with no word list")
	}

	// Tier 0 carries no merge step: these keys appear only with embedding_enabled true.
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"merge", "semantic"} {
		if _, ok := envelope.Data[key]; ok {
			t.Errorf("tier 0 reply carries %q", key)
		}
	}

	matches, total, truncated := deriveMatches(r.Data.Text)
	if len(matches) != 2 || total != 1486 || !truncated {
		t.Fatalf("matches=%d total=%d truncated=%v, want 2/1486/true", len(matches), total, truncated)
	}
	top := matches[0]
	if top.Type != "class" || top.Label != "SensorDriverConfig" {
		t.Errorf("top match type=%q label=%q, want class SensorDriverConfig", top.Type, top.Label)
	}
	// A word-index score, not the merged score (at most 2/61) of tier 1.
	if top.Relevance <= 1 || top.Relevance < matches[1].Relevance {
		t.Errorf("relevance %v then %v, want word-index scores best first", top.Relevance, matches[1].Relevance)
	}
	if top.WordRank != 0 || top.VectorRank != 0 || r.Data.Merge != nil {
		t.Errorf("tier 0 reply carried ranks %d/%d or a merge %+v", top.WordRank, top.VectorRank, r.Data.Merge)
	}
	got := deriveDisclosure(&r.Data, deriveCommunities(r.Data.Text))
	if got.Rung != rungEntitiesOnly {
		t.Errorf("rung=%q, want %q", got.Rung, rungEntitiesOnly)
	}
	// Tier 0's note is unchanged by the tier 1 merge wording.
	if !strings.Contains(got.Note, "these are word-search hits, not community") {
		t.Errorf("tier 0 note = %q", got.Note)
	}
}

// alpha.6 sends data.text as null when the word index was not read. That shape
// is not reachable with embeddings off, so this reply is written by hand from
// processor/graph-query/MIGRATION.md, not recorded. It must read as "matches
// unknown", never as "no matches".
// spec: openspec/specs/graphrag-access/spec.md#Requirement: An answer discloses how far it escalated
func TestNullWordListIsNotAnEmptyResult(t *testing.T) {
	r := decodeReply(t, `{"data":{"text":null,"hints":[]},"indexed_revision":1,"producer":"graph-query"}`)
	if r.Data.Text != nil {
		t.Fatal("null text decoded as a word list")
	}
	if matches, total, truncated := deriveMatches(r.Data.Text); matches != nil || total != 0 || truncated {
		t.Errorf("deriveMatches(nil) = %v/%d/%v, want nothing", matches, total, truncated)
	}
}
