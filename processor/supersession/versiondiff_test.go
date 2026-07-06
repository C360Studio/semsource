package supersession

import (
	"testing"
	"time"
)

// mkCand builds a candidate for the diff tests. hash/kind default to a bodykey so
// same-version identical bodies compare unchanged unless overridden.
func mkCand(project, version, name, hash string) candidate {
	return candidate{
		id:           project + "." + version + "." + name,
		org:          "acme",
		project:      project,
		version:      version,
		path:         name + ".go",
		name:         name,
		ctype:        "function",
		pkg:          "pkg",
		bodyHash:     hash,
		bodyHashKind: "bodykey",
		indexedAt:    time.Unix(0, 0),
	}
}

func TestDiffCandidates(t *testing.T) {
	cands := []candidate{
		// unchanged: same body hash across versions
		mkCand("p", "1.0", "Stable", "h1"),
		mkCand("p", "2.0", "Stable", "h1"),
		// changed: different body hash
		mkCand("p", "1.0", "Edited", "h2"),
		mkCand("p", "2.0", "Edited", "h2new"),
		// removed: only in 1.0
		mkCand("p", "1.0", "Gone", "h3"),
		// added: only in 2.0
		mkCand("p", "2.0", "New", "h4"),
		// a different project — must be ignored
		mkCand("other", "1.0", "Stable", "x"),
		mkCand("other", "2.0", "Stable", "y"),
	}

	changes, counts, _ := diffCandidates(cands, "p", "1.0", "2.0")

	if counts.Added != 1 || counts.Removed != 1 || counts.Changed != 1 || counts.Unchanged != 1 || counts.Indeterminate != 0 {
		t.Fatalf("counts = %+v; want added1 removed1 changed1 unchanged1 indeterminate0", counts)
	}
	// unchanged is counted but not listed → 3 entries (added/removed/changed).
	if len(changes) != 3 {
		t.Fatalf("changes = %d; want 3 (unchanged omitted)", len(changes))
	}
	byName := map[string]Change{}
	for _, ch := range changes {
		byName[ch.Name] = ch
	}
	if byName["New"].Status != statusAdded || byName["New"].ToID == "" || byName["New"].FromID != "" {
		t.Errorf("New should be added with a to_id only: %+v", byName["New"])
	}
	if byName["Gone"].Status != statusRemoved || byName["Gone"].FromID == "" || byName["Gone"].ToID != "" {
		t.Errorf("Gone should be removed with a from_id only: %+v", byName["Gone"])
	}
	if byName["Edited"].Status != statusChanged || byName["Edited"].FromID == "" || byName["Edited"].ToID == "" {
		t.Errorf("Edited should be changed with both ids: %+v", byName["Edited"])
	}
	if _, listed := byName["Stable"]; listed {
		t.Errorf("Stable (unchanged) must not be listed")
	}
}

func TestDiffCandidates_Indeterminate(t *testing.T) {
	// Missing hash on one side and a kind mismatch both classify indeterminate,
	// never changed/unchanged.
	missing := mkCand("p", "2.0", "NoHash", "")
	crossKind := mkCand("p", "2.0", "CrossKind", "h")
	crossKind.bodyHashKind = "hash" // differs from the 1.0 side's "bodykey"

	cands := []candidate{
		mkCand("p", "1.0", "NoHash", "h"),
		missing,
		mkCand("p", "1.0", "CrossKind", "h"),
		crossKind,
	}

	changes, counts, _ := diffCandidates(cands, "p", "1.0", "2.0")
	if counts.Indeterminate != 2 || counts.Changed != 0 || counts.Unchanged != 0 {
		t.Fatalf("counts = %+v; want indeterminate2, changed0, unchanged0", counts)
	}
	for _, ch := range changes {
		if ch.Status != statusIndeterminate {
			t.Errorf("%s should be indeterminate, got %s", ch.Name, ch.Status)
		}
	}
}
