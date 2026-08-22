package cutover

import (
	"sort"
	"testing"

	semgraph "github.com/c360studio/semstreams/graph"
)

// TestFrameworkBucketsAreClassified is the interlock. It runs in normal CI —
// no NATS, no docker, no build tag — because its whole value is failing at the
// moment a dependency bump changes the framework's owned set, rather than
// whenever someone next runs the tagged e2e suite locally.
//
// It compares as a SET, not in catalog order: the question is whether every
// framework-owned bucket has a reviewed disposition, and deletion order carries
// no meaning. An earlier revision compared order-sensitively against a single
// literal, which could not express a deliberate retention at all.
func TestFrameworkBucketsAreClassified(t *testing.T) {
	classified := append(append([]string{}, Purged...), Retained...)
	runtime := append([]string{}, semgraph.FrameworkOwnedBuckets()...)
	sort.Strings(classified)
	sort.Strings(runtime)

	missing := diff(runtime, classified)
	extra := diff(classified, runtime)

	for _, b := range missing {
		t.Errorf("framework-owned bucket %q has no reviewed disposition.\n"+
			"A cutover must not decide this implicitly: add it to Purged if it re-derives\n"+
			"from source on the next seed, or to Retained (with the reason) if it does not.", b)
	}
	for _, b := range extra {
		t.Errorf("bucket %q is classified but the framework no longer owns it; drop it", b)
	}
}

// TestPurgedAndRetainedAreDisjoint guards the obvious foot-gun: a bucket listed
// in both would be deleted by the purge loop and then asserted to have survived.
func TestPurgedAndRetainedAreDisjoint(t *testing.T) {
	for _, b := range diff(Retained, diff(Retained, Purged)) {
		t.Errorf("bucket %q is both purged and retained", b)
	}
}

func diff(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, s := range b {
		in[s] = true
	}
	var out []string
	for _, s := range a {
		if !in[s] {
			out = append(out, s)
		}
	}
	return out
}
