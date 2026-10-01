//go:build qualification && removal

package setup03a

import (
	"encoding/json"
	"testing"
)

// This before-binary control performs no source-management operation. The raw
// strict failure is preserved in sibling-before-control evidence; the reviewed
// oracle records AST reseed timestamps separately and keeps source facts strict.
func TestRemovalReplaySiblingRestartControl(t *testing.T) {
	p := newReplayQualification(t, false)
	p.restart("plain_restart_checked_exit", false)
	p.settled("plain_restart_same_sources_settled", true)
	p.unchangedSiblings("plain_restart_sibling_graph_and_exact_content_unchanged")
}

func replayOracleEntity(t *testing.T, id, created, label, stale string) exact {
	t.Helper()
	triples := []map[string]any{{"predicate": "dc.terms.created", "object": created}, {"predicate": "code.symbol.name", "object": label}}
	if stale != "" {
		triples = append(triples, map[string]any{"predicate": "entity.lifecycle.stale", "object": stale})
	}
	raw, err := json.Marshal(map[string]any{"entity": map[string]any{"id": id, "triples": triples}})
	if err != nil {
		t.Fatal(err)
	}
	var entity exact
	if err := json.Unmarshal(raw, &entity); err != nil {
		t.Fatal(err)
	}
	return entity
}
func TestReplaySiblingOracleOnlyNormalizesKnownASTReseedTimestamp(t *testing.T) {
	p := &replayQualification{platform: "semsource-a1b2c3"}
	id := "setup03a." + p.platform + "." + replayTargetProject + ".golang.function.Run"
	initial := replayOracleEntity(t, id, "2026-09-30T19:45:37-05:00", "Run", "")
	repeated := replayOracleEntity(t, id, "2026-09-30T19:46:07-05:00", "Run", "")
	metadata, err := replayCompareSibling(id, replaySourceFacts(initial), repeated, p.isASTSibling(id))
	if err != nil || metadata == nil || metadata.Before != "2026-09-30T19:45:37-05:00" || metadata.After != "2026-09-30T19:46:07-05:00" {
		t.Fatalf("timestamp evidence=%+v err=%v", metadata, err)
	}
	for _, test := range []struct{ name, id, created, label, stale string }{
		{"other fact", id, "2026-09-30T19:46:07-05:00", "Changed", ""},
		{"lifecycle marker", id, "2026-09-30T19:46:07-05:00", "Run", "source_removed"},
		{"different id", id + "-other", "2026-09-30T19:46:07-05:00", "Run", ""},
		{"invalid timestamp", id, "not-a-time", "Run", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := replayOracleEntity(t, test.id, test.created, test.label, test.stale)
			if _, err := replayCompareSibling(id, replaySourceFacts(initial), got, true); err == nil {
				t.Fatal("changed sibling accepted")
			}
		})
	}
	for _, id := range []string{
		"setup03a." + p.platform + "." + replayTargetProject + ".web.doc.manual",
		"setup03a." + p.platform + ".other.golang.function.Run",
		"foreign." + p.platform + "." + replayTargetProject + ".golang.function.Run",
	} {
		t.Run(id, func(t *testing.T) {
			if p.isASTSibling(id) {
				t.Fatal("unknown producer admitted for timestamp normalization")
			}
			before := replayOracleEntity(t, id, "2026-09-30T19:45:37-05:00", "Run", "")
			after := replayOracleEntity(t, id, "2026-09-30T19:46:07-05:00", "Run", "")
			if _, err := replayCompareSibling(id, replaySourceFacts(before), after, false); err == nil {
				t.Fatal("non-AST timestamp change accepted")
			}
		})
	}
}
func TestReplaySiblingOracleKeepsContentBytesStrict(t *testing.T) {
	for _, test := range []struct {
		name, got string
		wantError bool
	}{{"same", "exact\x00body", false}, {"changed", "different\x00body", true}, {"truncated", "exact", true}, {"empty", "", true}} {
		t.Run(test.name, func(t *testing.T) {
			err := replayCompareContent("key", "exact\x00body", []byte(test.got))
			if (err != nil) != test.wantError {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
