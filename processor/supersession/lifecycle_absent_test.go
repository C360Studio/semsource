package supersession

import (
	"testing"

	"github.com/c360studio/semsource/graph"
)

// The absent-set oracle exists for sources whose artifacts are not files. An
// object store tells nobody what it holds except by being listed, so the
// caller's completed enumeration is the only liveness evidence there is — and
// it is better evidence than a stat, not worse.

func TestLivenessOracle_NoOracleWhenNeitherIsGiven(t *testing.T) {
	stat, err := livenessOracle(graph.LifecycleRunRequest{Org: "acme", Systems: []string{"s"}})
	if err != nil {
		t.Fatalf("livenessOracle: %v", err)
	}
	// nil is what makes decideLifecycleActions mark everything — the
	// remove_source shape, unchanged.
	if stat != nil {
		t.Error("a request with no liveness evidence must yield no oracle")
	}
}

func TestLivenessOracle_RejectsTwoOracles(t *testing.T) {
	_, err := livenessOracle(graph.LifecycleRunRequest{
		Org:      "acme",
		Systems:  []string{"s"},
		RootPath: "/docs",
		Absent:   []string{"reports/q4.md"},
	})
	if err == nil {
		t.Fatal("a request carrying both a root path and an absent set must be rejected")
	}
}

func TestLivenessOracle_AbsentSetAnswersLiveness(t *testing.T) {
	stat, err := livenessOracle(graph.LifecycleRunRequest{
		Org:     "acme",
		Systems: []string{"s"},
		Absent:  []string{"reports/q4.md", "reports/old.md"},
	})
	if err != nil {
		t.Fatalf("livenessOracle: %v", err)
	}
	if stat == nil {
		t.Fatal("an absent set must yield an oracle")
	}

	if stat("reports/q4.md") {
		t.Error("a named path must read as gone")
	}
	// Anything the caller did not name is present. That is what lets an
	// artifact which came back clear its marker on this same pass instead of
	// waiting for a re-seed.
	if !stat("reports/q3.md") {
		t.Error("a path the caller did not name must read as present")
	}
}

// TestLivenessOracle_EmptyAbsentSetIsAnAssertion is the case the field's
// missing omitempty protects. "My listing completed and nothing is gone" must
// not decay into "mark everything in scope".
func TestLivenessOracle_EmptyAbsentSetIsAnAssertion(t *testing.T) {
	stat, err := livenessOracle(graph.LifecycleRunRequest{
		Org:     "acme",
		Systems: []string{"s"},
		Absent:  []string{},
	})
	if err != nil {
		t.Fatalf("livenessOracle: %v", err)
	}
	if stat == nil {
		t.Fatal("an empty absent set is still an oracle — it says everything is present")
	}
	if !stat("reports/q3.md") {
		t.Error("with nothing named absent, every path is present")
	}
}

// TestAbsentSet_MarksParentAndPassagesTogether is why this rides the existing
// pass rather than being marked by the source. The source knows the object key
// and the parent document's ID; the passages carry their own IDs and the
// parent's path. Grouping by path is what reaches all of them.
func TestAbsentSet_MarksParentAndPassagesTogether(t *testing.T) {
	fx := docFixture{path: "reports/q4.md", count: 3, passages: 3}

	stat, err := livenessOracle(graph.LifecycleRunRequest{
		Org:     "acme",
		Systems: []string{"s"},
		Absent:  []string{"reports/q4.md"},
	})
	if err != nil {
		t.Fatalf("livenessOracle: %v", err)
	}

	toMark, toClear, paths := decideLifecycleActions(fx.build(), graph.LifecycleReasonFileDeleted, stat)

	if paths != 1 {
		t.Errorf("grouped %d paths, want 1", paths)
	}
	// The parent and all three passages.
	if len(toMark) != 4 {
		t.Errorf("marked %d entities, want 4 (parent plus three passages)", len(toMark))
	}
	if len(toClear) != 0 {
		t.Errorf("cleared %v, want none", toClear)
	}
	for _, marker := range toMark {
		if marker.Object != graph.LifecycleReasonFileDeleted {
			t.Errorf("%s marked with reason %v", marker.Subject, marker.Object)
		}
	}
}

// TestAbsentSet_LeavesPresentObjectsAlone keeps the blast radius proportional:
// naming one gone object must not disturb the rest of the prefix.
func TestAbsentSet_LeavesPresentObjectsAlone(t *testing.T) {
	present := docFixture{path: "reports/q3.md", count: 2, passages: 2}
	gone := docFixture{path: "reports/q4.md", count: 2, passages: 2}
	entities := append(present.build(), gone.build()...)

	stat, err := livenessOracle(graph.LifecycleRunRequest{
		Org:     "acme",
		Systems: []string{"s"},
		Absent:  []string{"reports/q4.md"},
	})
	if err != nil {
		t.Fatalf("livenessOracle: %v", err)
	}

	toMark, _, paths := decideLifecycleActions(entities, graph.LifecycleReasonFileDeleted, stat)

	if paths != 2 {
		t.Errorf("grouped %d paths, want 2", paths)
	}
	if len(toMark) != 3 {
		t.Fatalf("marked %d entities, want 3 (only the gone document)", len(toMark))
	}

	goneIDs := map[string]bool{gone.parentID(): true}
	for i := range gone.passages {
		goneIDs[gone.passageID(i)] = true
	}
	for _, marker := range toMark {
		if !goneIDs[marker.Subject] {
			t.Errorf("%s belongs to a document that is still there", marker.Subject)
		}
	}
}

// TestAbsentSet_ReappearedArtifactIsCleared covers the other direction, which
// the absent set gets for free by naming only what is gone: a path the caller
// does not mention is present, so an entity already carrying a marker has it
// removed on the same pass that observes the artifact again.
//
// Without this the marker would survive until a re-seed, and a document that
// came back would stay ranked as stale while being perfectly readable.
func TestAbsentSet_ReappearedArtifactIsCleared(t *testing.T) {
	// The document is marked stale from an earlier pass, and its passages
	// with it.
	fx := docFixture{
		path:       "reports/q4.md",
		count:      2,
		passages:   2,
		markedIdx:  []int{0, 1},
		parentMark: true,
	}

	// The bucket now holds it again, so the completed pass names something
	// else as absent — or nothing at all.
	stat, err := livenessOracle(graph.LifecycleRunRequest{
		Org:     "acme",
		Systems: []string{"s"},
		Absent:  []string{"reports/some-other-doc.md"},
	})
	if err != nil {
		t.Fatalf("livenessOracle: %v", err)
	}

	toMark, toClear, _ := decideLifecycleActions(fx.build(), graph.LifecycleReasonFileDeleted, stat)

	if len(toMark) != 0 {
		t.Errorf("marked %d entities for a document that is present", len(toMark))
	}
	if len(toClear) != 3 {
		t.Fatalf("cleared %v, want the parent and both passages", toClear)
	}
}
