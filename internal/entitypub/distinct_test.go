package entitypub

import "testing"

// TestDistinctTracker_ReindexDoesNotInflate pins the audit's count-inflation
// regression: republishing the same entities (the 60s periodic reindex)
// must not change entity_count or type_counts (live-observed: code.folder
// 60→240, code.repo 1→4 within minutes while the graph itself was clean).
func TestDistinctTracker_ReindexDoesNotInflate(t *testing.T) {
	tr := NewDistinctTracker()
	ids := []string{
		"c360.semsource.code.workspace.repo.workspace",
		"c360.semsource.code.workspace.folder.entityid",
		"c360.semsource.golang.workspace.function.entityid-entityid-go-Build",
	}

	for round := range 4 {
		for _, id := range ids {
			isNew := tr.Observe(id)
			if round == 0 && !isNew {
				t.Errorf("round 0: Observe(%q) = false, want true", id)
			}
			if round > 0 && isNew {
				t.Errorf("round %d: Observe(%q) = true, want false", round, id)
			}
		}
	}

	if got := tr.Count(); got != int64(len(ids)) {
		t.Errorf("Count() = %d after 4 rounds, want %d", got, len(ids))
	}
	tc := tr.TypeCounts()
	want := map[string]int64{"code.repo": 1, "code.folder": 1, "golang.function": 1}
	for k, v := range want {
		if tc[k] != v {
			t.Errorf("TypeCounts[%q] = %d, want %d", k, tc[k], v)
		}
	}
}
