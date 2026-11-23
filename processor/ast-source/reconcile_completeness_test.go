package astsource

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semsource/graph"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

// groupStatements returns, per subject, the sorted group-`source` statements
// in every message recorded since the last reset, skipping dc.terms.created
// (wall-clock per parse) and the subjects skip rejects.
func (f *foldFixture) groupStatements(skip func(subject string) bool) map[string][]string {
	f.t.Helper()
	owned := map[string]bool{}
	for _, p := range graph.OwnedPredicates() {
		owned[p] = true
	}
	f.client.mu.Lock()
	msgs := append([][]byte(nil), f.client.msgs...)
	f.client.mu.Unlock()

	out := map[string][]string{}
	for _, m := range msgs {
		var v any
		if err := json.Unmarshal(m, &v); err != nil {
			f.t.Fatalf("decode published message: %v", err)
		}
		walkTriples(v, func(subject, predicate string, object any) {
			if !owned[predicate] || predicate == semsourceast.DcCreated || skip(subject) {
				return
			}
			out[subject] = append(out[subject], fmt.Sprintf("%s=%v", predicate, object))
		})
	}
	for s := range out {
		sort.Strings(out[s])
	}
	return out
}

// TestWatchPublishCarriesTheSeedGroupSet is the ast-source completeness check
// for the #336 reconcile: re-publishing an unchanged file through the watch
// path carries, for the file and every symbol in it, the same group-`source`
// statements the seed published. Folder and repo entities are excluded from
// reconcile (#347) and are skipped here.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestWatchPublishCarriesTheSeedGroupSet(t *testing.T) {
	f := newFoldFixture(t)
	f.seed()
	notContainer := func(s string) bool {
		return strings.Contains(s, ".folder.") || strings.Contains(s, ".repo.")
	}
	seeded := f.groupStatements(notContainer)

	f.reset()
	f.watchModify("a/b.ts")
	f.settle()
	watched := f.groupStatements(notContainer)

	if len(watched) == 0 {
		t.Fatal("watch published no file or symbol statements; the fixture is vacuous")
	}
	sawBelongs := false
	for subject, got := range watched {
		want := seeded[subject]
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s: watch group set differs from seed\n got: %v\nwant: %v", subject, got, want)
		}
		if strings.Contains(subject, ".file.") {
			for _, s := range got {
				sawBelongs = sawBelongs || strings.HasPrefix(s, semsourceast.CodeBelongsTo+"=")
			}
		}
	}
	if !sawBelongs {
		t.Error("watch-published file entity carries no code.structure.belongs")
	}
}
