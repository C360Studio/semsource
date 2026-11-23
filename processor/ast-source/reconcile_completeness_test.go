package astsource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semengine/pkg/projection"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/internal/entitypub"
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

type recordingReconciler struct {
	mu  sync.Mutex
	ids []string
}

func (r *recordingReconciler) Reconcile(_ context.Context, m projection.ReconcileMutation) (projection.MutationReceipt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, m.EntityID)
	return projection.MutationReceipt{}, nil
}

func (r *recordingReconciler) reconciled() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ids...)
}

// withReconcile swaps the fixture's publisher for one that records reconciles.
func (f *foldFixture) withReconcile() *recordingReconciler {
	f.t.Helper()
	rec := &recordingReconciler{}
	pub, err := entitypub.New(f.client, f.c.logger, entitypub.WithReconcile(rec))
	if err != nil {
		f.t.Fatal(err)
	}
	pub.Start(f.ctx)
	f.t.Cleanup(func() { _ = pub.Stop(context.Background()) })
	f.c.publisher = pub
	return rec
}

type failingPutStore struct{ *fakeStore }

func (failingPutStore) Put(context.Context, string, []byte) error { return errors.New("store down") }

// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestReconcileSkipsContainersAndBodyFaults(t *testing.T) {
	hasType := func(ids []string, typ string) bool {
		for _, id := range ids {
			if strings.Contains(id, "."+typ+".") {
				return true
			}
		}
		return false
	}

	t.Run("seed reconciles files and symbols, never folders or the repo", func(t *testing.T) {
		f := newFoldFixture(t)
		rec := f.withReconcile()
		f.seed()
		ids := rec.reconciled()
		if !hasType(ids, "file") || !hasType(ids, "function") {
			t.Fatalf("seed reconciled %v; want the file and its function", ids)
		}
		if hasType(ids, "folder") || hasType(ids, "repo") {
			t.Errorf("a folder or the repo was reconciled: %v", ids)
		}
	})

	faults := map[string]func(c *Component){
		"body Put fails":            func(c *Component) { c.bodyStore = failingPutStore{newFakeStore()} },
		"body store failed to load": func(c *Component) { c.bodyStore, c.bodyStoreDown = nil, true },
	}
	for name, induce := range faults {
		t.Run(name, func(t *testing.T) {
			f := newFoldFixture(t)
			f.seed()
			rec := f.withReconcile()
			induce(f.c)
			f.write("a/b.ts", "export function changed() {}\n")
			f.watchModify("a/b.ts")
			f.settle()
			ids := rec.reconciled()
			if !hasType(ids, "file") {
				t.Fatalf("the body-less file entity was not reconciled: %v", ids)
			}
			if hasType(ids, "function") {
				t.Errorf("a symbol whose body offload faulted was reconciled: %v", ids)
			}
		})
	}
}
