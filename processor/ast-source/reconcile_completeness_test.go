package astsource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/projection"

	"github.com/c360studio/semsource/entityid"
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

// ReadAuthoritative is the first call of every reconcile the publisher runs,
// so recording it records which entities were reconciled. It then ends the
// reconcile: no graph stands behind this fixture.
func (r *recordingReconciler) ReadAuthoritative(_ context.Context, id string) (*gtypes.ExactEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, id)
	return nil, errors.New("recording fixture: no graph")
}

func (r *recordingReconciler) Reconcile(context.Context, projection.ReconcileMutation) (projection.MutationReceipt, error) {
	return projection.MutationReceipt{}, errors.New("recording fixture: no graph")
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

	// Owner ruling, task 5.2: reconcile only content this process knows
	// changed. A start over a retained store is a first pass, as is the seed.
	t.Run("a first pass and an unchanged reindex make no reconcile calls", func(t *testing.T) {
		f := newFoldFixture(t)
		rec := f.withReconcile()
		f.seed()
		f.reindex()
		if ids := rec.reconciled(); len(ids) != 0 {
			t.Errorf("reconciled %v without any content change", ids)
		}
	})

	t.Run("a changed file reconciles on reindex and watch, never folders or the repo", func(t *testing.T) {
		f := newFoldFixture(t)
		rec := f.withReconcile()
		f.seed()
		f.write("a/b.ts", "export function changed() {}\n")
		f.reindex()
		f.write("a/b.ts", "export function again() {}\n")
		f.watchModify("a/b.ts")
		f.settle()
		ids := rec.reconciled()
		var functions int
		for _, id := range ids {
			if strings.Contains(id, ".function.") {
				functions++
			}
		}
		if !hasType(ids, "file") || functions != 2 {
			t.Fatalf("reconciled %v; want the file and each pass's function (reindex, then watch)", ids)
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

// newLangFixture is newFoldFixture for one language over the given files.
func newLangFixture(t *testing.T, lang, ext string, files map[string]string) *foldFixture {
	t.Helper()
	root := t.TempDir()
	f := &foldFixture{t: t, root: root, client: &recordingClient{}}
	for rel, src := range files {
		f.write(rel, src)
	}
	auth := entityid.Authority{Org: "acme", Platform: "test"}
	parser, err := semsourceast.DefaultRegistry.CreateParser(lang, auth, "proj", root)
	if err != nil {
		t.Fatalf("create %s parser: %v", lang, err)
	}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	pub, err := entitypub.New(f.client, logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	f.ctx = ctx
	pub.Start(ctx)
	t.Cleanup(func() { _ = pub.Stop(context.Background()) })
	f.pw = &pathWatcher{
		root: root, scopedSystem: "proj", owner: "ast-source",
		config:   WatchPathConfig{Org: "acme", Project: "proj"},
		parsers:  map[string]semsourceast.FileParser{lang: parser},
		routes:   map[string]string{ext: lang},
		excludes: map[string]bool{},
	}
	f.c = &Component{
		authority: auth, logger: logger, bodyStore: newFakeStore(), publisher: pub,
		distinct: entitypub.NewDistinctTracker(), fileHashes: make(map[string]string),
		watchers: []*pathWatcher{f.pw},
	}
	return f
}

// TestWatchPublishCarriesTheSeedGroupSet_PerLanguage extends the
// completeness check to Go, Java and Python: a watch republish of an
// unchanged nested file carries, for the file and each symbol, the seed's
// group-`source` statements.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestWatchPublishCarriesTheSeedGroupSet_PerLanguage(t *testing.T) {
	cases := []struct {
		lang, ext, edited string
		files             map[string]string
	}{
		{"go", ".go", "pkg/svc/run.go", map[string]string{
			"go.mod":           "module example.com/proj\n\ngo 1.22\n",
			"pkg/svc/run.go":   "package svc\n\n// Run does the work.\nfunc Run() { helper() }\n\nfunc helper() {}\n\ntype T struct{ N int }\n\nfunc (t T) M() int { return t.N }\n",
			"pkg/svc/other.go": "package svc\n\nfunc Other() { Run() }\n",
		}},
		{"java", ".java", "src/main/java/acme/Svc.java", map[string]string{
			"src/main/java/acme/Svc.java":  "package acme;\n\n/** Svc does work. */\npublic class Svc {\n  private int n;\n  public int run() { return helper(n); }\n  int helper(int x) { return x; }\n}\n",
			"src/main/java/acme/Main.java": "package acme;\n\npublic class Main {\n  void go() { new Svc().run(); }\n}\n",
		}},
		{"python", ".py", "pkg/svc.py", map[string]string{
			"pkg/__init__.py": "",
			"pkg/svc.py":      "\"\"\"Svc module.\"\"\"\n\n\ndef helper(x):\n    return x\n\n\nclass Svc:\n    def run(self):\n        return helper(1)\n",
			"pkg/main.py":     "from pkg.svc import Svc\n\n\ndef go():\n    return Svc().run()\n",
		}},
	}
	notContainer := func(s string) bool {
		return strings.Contains(s, ".folder.") || strings.Contains(s, ".repo.")
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			f := newLangFixture(t, tc.lang, tc.ext, tc.files)
			f.seed()
			seeded := f.groupStatements(notContainer)
			f.reset()
			f.watchModify(tc.edited)
			f.settle()
			watched := f.groupStatements(notContainer)
			if len(watched) < 2 {
				t.Fatalf("watch published %d subjects; want the file and its symbols", len(watched))
			}
			for subject, got := range watched {
				if want := seeded[subject]; strings.Join(got, "\n") != strings.Join(want, "\n") {
					t.Errorf("%s: watch group set differs from seed\n got: %v\nwant: %v", subject, got, want)
				}
			}
		})
	}
}

// A file truncated between its parse and the body read leaves the parsed
// symbols without a body; they are faulted, so no reconcile clears the body
// handles a previous pass stored.
// spec: openspec/changes/reconcile-source-group-on-republish/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestBodyTruncatedBetweenParseAndOffloadIsFaulted(t *testing.T) {
	f := newLangFixture(t, "go", ".go", map[string]string{"svc.go": "package svc\n\n\n\nfunc Run() {}\n"})
	result, err := f.pw.parsers["go"].ParseFile(f.ctx, f.root+"/svc.go")
	if err != nil {
		t.Fatal(err)
	}
	f.write("svc.go", "package svc\n")
	bodies := f.c.bodiesForResult(f.ctx, result, f.root)
	found := false
	for _, e := range result.Entities {
		if e.Type != semsourceast.TypeFunction {
			continue
		}
		found = true
		if !bodies[e.ID].faulted {
			t.Errorf("%s lost its body but is not faulted, so reconcile would clear its handles", e.ID)
		}
	}
	if !found {
		t.Fatal("no function parsed")
	}
}
