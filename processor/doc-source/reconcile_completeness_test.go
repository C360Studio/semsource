package docsource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/types"

	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/handler"
	dochandler "github.com/c360studio/semsource/handler/doc"
	"github.com/c360studio/semsource/internal/entitypub"
	"github.com/c360studio/semsource/internal/entitypub/entitypubtest"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// recordingClient captures every message the publisher delivers.
type recordingClient struct {
	mu   sync.Mutex
	msgs [][]byte
}

func (r *recordingClient) PublishToStream(_ context.Context, _ string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, append([]byte(nil), data...))
	return nil
}

func (r *recordingClient) PublishToStreamWithMsgID(ctx context.Context, subject string, data []byte, _ string) error {
	return r.PublishToStream(ctx, subject, data)
}

func (r *recordingClient) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = nil
}

// memBodyStore is an in-memory verbatim body store.
type memBodyStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *memBodyStore) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = map[string][]byte{}
	}
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *memBodyStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.data[key]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("key not found: %s", key)
}

func (s *memBodyStore) List(context.Context, string) ([]string, error) { return nil, nil }
func (s *memBodyStore) Delete(context.Context, string) error           { return nil }

// docFixture is a doc-source built by NewComponent, so its reconcile gate is
// the production one, publishing through a recording transport and a
// recording reconciler. The watch path is driven through handleChangeEvent
// with states the handler builds for one file, as its watcher does.
type docFixture struct {
	t      *testing.T
	ctx    context.Context
	c      *Component
	client *recordingClient
	rec    *entitypubtest.RecordingReconciler
	roots  []string
}

func newDocFixture(t *testing.T, project string, roots ...string) *docFixture {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"paths": roots, "project": project, "org": "acme"})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	disc, err := NewComponent(raw, component.Dependencies{
		Platform: types.PlatformMeta{Org: "acme", Platform: "test-a1b2c3"},
		Logger:   logger,
	})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	c := disc.(*Component)
	c.handler = dochandler.NewWithAuthority(c.authority,
		dochandler.WithProject(c.config.Project),
		dochandler.WithBodyStore(&memBodyStore{}, graph.BodyStoreInstance))

	f := &docFixture{t: t, ctx: context.Background(), c: c, client: &recordingClient{},
		rec: &entitypubtest.RecordingReconciler{}, roots: roots}
	pub, err := entitypub.New(f.client, logger, entitypub.WithReconcile(f.rec))
	if err != nil {
		t.Fatal(err)
	}
	pub.Start(f.ctx)
	t.Cleanup(func() { _ = pub.Stop(context.Background()) })
	c.publisher = pub
	return f
}

func (f *docFixture) write(root, rel, content string) string {
	f.t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *docFixture) seed() {
	f.t.Helper()
	if err := f.c.ingestOnce(f.ctx); err != nil {
		f.t.Fatalf("seed: %v", err)
	}
	f.settle()
}

// watchModify delivers the change event the doc watcher builds for path: the
// handler's states for that one file, re-read from disk.
func (f *docFixture) watchModify(path string) {
	f.t.Helper()
	var states []*handler.EntityState
	for _, root := range f.roots {
		all, err := f.c.handler.IngestEntityStates(f.ctx, &sourceCfg{paths: []string{root}}, f.c.authority)
		if err != nil {
			f.t.Fatalf("build watch states: %v", err)
		}
		rel, _ := filepath.Rel(root, path)
		for _, st := range all {
			if stateFilePath(st) == filepath.ToSlash(rel) {
				states = append(states, st)
			}
		}
	}
	if len(states) == 0 {
		f.t.Fatalf("no states for %s; the fixture is vacuous", path)
	}
	f.c.handleChangeEvent(f.ctx, handler.ChangeEvent{Path: path, Operation: handler.OperationModify, EntityStates: states})
	f.settle()
}

func (f *docFixture) settle() {
	f.t.Helper()
	if err := f.c.publisher.WaitSettled(f.ctx); err != nil {
		f.t.Fatalf("settle: %v", err)
	}
}

func stateFilePath(st *handler.EntityState) string {
	for _, tr := range st.Triples {
		if tr.Predicate == source.DocFilePath {
			s, _ := tr.Object.(string)
			return s
		}
	}
	return ""
}

// groupStatements returns, per subject, the sorted group-`source` statements
// in every message recorded since the last reset. The doc handler emits no
// wall-clock predicate (no dc.terms.created), so none is excluded.
func (f *docFixture) groupStatements() map[string][]string {
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
			if owned[predicate] {
				out[subject] = append(out[subject], fmt.Sprintf("%s=%v", predicate, object))
			}
		})
	}
	for s := range out {
		sort.Strings(out[s])
	}
	return out
}

func walkTriples(v any, fn func(subject, predicate string, object any)) {
	switch x := v.(type) {
	case map[string]any:
		s, sok := x["subject"].(string)
		p, pok := x["predicate"].(string)
		if sok && pok {
			fn(s, p, x["object"])
		}
		for _, child := range x {
			walkTriples(child, fn)
		}
	case []any:
		for _, child := range x {
			walkTriples(child, fn)
		}
	}
}

const twoSections = "# Guide\n\n## Install\n\nRun the installer.\n\n## Usage\n\nCall it.\n"

// A watch republish of a document that now emits less carries, for the
// document and each passage, the complete group set a fresh seed of the new
// content produces, so the reconcile that follows removes only what is gone.
// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestDocWatchRepublishCarriesTheCompleteGroupSet(t *testing.T) {
	root := t.TempDir()
	f := newDocFixture(t, "", root)
	path := f.write(root, "guide.md", twoSections)
	f.seed()
	seeded := f.groupStatements()

	// The edit drops the headings: passages lose doc.section.
	f.write(root, "guide.md", "Run the installer, then call it.\n")
	f.client.reset()
	f.watchModify(path)
	watched := f.groupStatements()

	fresh := newDocFixture(t, "", root)
	fresh.seed()
	want := fresh.groupStatements()

	if len(watched) == 0 {
		t.Fatal("watch published nothing; the fixture is vacuous")
	}
	shrank := false
	for subject, got := range watched {
		if strings.Join(got, "\n") != strings.Join(want[subject], "\n") {
			t.Errorf("%s: watch group set differs from a seed of the same content\n got: %v\nwant: %v",
				subject, got, want[subject])
		}
		shrank = shrank || len(got) < len(seeded[subject])
	}
	if !shrank {
		t.Error("the edit emitted no fewer statements for any entity; the test proves nothing")
	}
}

// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: Reconcile is enabled only for handlers proven to publish complete sets
func TestDocReconcileOnlyKnownChanges(t *testing.T) {
	t.Run("a first pass, an unchanged republish and a new file make no reconcile calls", func(t *testing.T) {
		root := t.TempDir()
		f := newDocFixture(t, "", root)
		path := f.write(root, "guide.md", twoSections)
		f.seed()
		f.watchModify(path)
		f.watchModify(f.write(root, "added.md", "# Added\n\nNew.\n"))
		if ids := f.rec.IDs(); len(ids) != 0 {
			t.Errorf("reconciled %v without a known content change", ids)
		}
	})

	t.Run("a changed file reconciles exactly its changed entities", func(t *testing.T) {
		root := t.TempDir()
		f := newDocFixture(t, "", root)
		path := f.write(root, "guide.md", twoSections)
		f.seed()
		f.write(root, "guide.md", strings.Replace(twoSections, "Call it.", "Call it twice.", 1))
		f.watchModify(path)
		ids := f.rec.IDs()
		var doc, chunks int
		for _, id := range ids {
			switch {
			case strings.Contains(id, ".doc."):
				doc++
			case strings.Contains(id, ".chunk."):
				chunks++
			}
		}
		// The document's hash changed and so did the Usage passage; the
		// Install passage did not.
		if doc != 1 || chunks != 1 || len(ids) != 2 {
			t.Errorf("reconciled %v; want the document and the one changed passage", ids)
		}
	})

	t.Run("roots sharing one project never reconcile", func(t *testing.T) {
		a, b := t.TempDir(), t.TempDir()
		f := newDocFixture(t, "shared", a, b)
		path := f.write(a, "guide.md", twoSections)
		f.write(b, "guide.md", "# Other\n\nOther root.\n")
		f.seed()
		f.write(a, "guide.md", "Changed.\n")
		f.watchModify(path)
		if ids := f.rec.IDs(); len(ids) != 0 {
			t.Errorf("reconciled %v though two roots write one entity ID (#349)", ids)
		}
	})
}
