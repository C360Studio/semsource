package astsource

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/entitypub"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

// foldFixture is one TS watch path holding a/b.ts and a-b.ts, which path
// sanitizing folds onto one file entity ID. a/b.ts walks first and survives.
type foldFixture struct {
	t      *testing.T
	ctx    context.Context
	root   string
	client *recordingClient
	c      *Component
	pw     *pathWatcher
}

func newFoldFixture(t *testing.T) *foldFixture {
	t.Helper()
	root := t.TempDir()
	f := &foldFixture{t: t, root: root, client: &recordingClient{}}
	f.write("a/b.ts", "export function foo() {}\n")
	f.write("a-b.ts", "export function baz() {}\n")

	auth := entityid.Authority{Org: "acme", Platform: "test"}
	parser, err := semsourceast.DefaultRegistry.CreateParser("typescript", auth, "proj", root)
	if err != nil {
		t.Fatalf("create parser: %v", err)
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
		root:         root,
		scopedSystem: "proj",
		owner:        "ast-source",
		config:       WatchPathConfig{Org: "acme", Project: "proj"},
		parsers:      map[string]semsourceast.FileParser{"typescript": parser},
		routes:       map[string]string{".ts": "typescript"},
		excludes:     map[string]bool{},
	}
	f.c = &Component{
		authority:  auth,
		logger:     logger,
		bodyStore:  newFakeStore(),
		publisher:  pub,
		distinct:   entitypub.NewDistinctTracker(),
		fileHashes: make(map[string]string),
		watchers:   []*pathWatcher{f.pw},
	}
	return f
}

func (f *foldFixture) write(rel, src string) {
	f.t.Helper()
	full := filepath.Join(f.root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *foldFixture) seed() {
	f.t.Helper()
	if _, err := f.c.seedPath(f.ctx, f.pw); err != nil {
		f.t.Fatalf("seedPath: %v", err)
	}
	f.settle()
}

// reindex runs the periodic sweep and joins the lifecycle trigger it starts
// (it fails fast on the nil NATS client).
func (f *foldFixture) reindex() {
	f.t.Helper()
	f.c.performFullIndex(f.ctx)
	if err := f.c.workers.Wait(f.ctx); err != nil {
		f.t.Fatalf("workers did not join: %v", err)
	}
	f.settle()
}

func (f *foldFixture) settle() {
	f.t.Helper()
	if err := f.c.publisher.WaitSettled(f.ctx); err != nil {
		f.t.Fatalf("publisher did not settle: %v", err)
	}
}

// watchModify drives one fsnotify modify event for rel through the watch path.
func (f *foldFixture) watchModify(rel string) {
	f.t.Helper()
	res, err := f.c.parseFileWithWatcher(f.ctx, f.pw, filepath.Join(f.root, rel))
	if err != nil || res == nil {
		f.t.Errorf("parse %s: result=%v err=%v", rel, res, err)
		return
	}
	f.c.handleWatchEvent(f.ctx, f.pw, semsourceast.WatchEvent{
		Operation: semsourceast.OpModify, Path: rel, Result: res,
	})
}

// repoContains returns the sorted objects of every repo contains triple
// published since the recorder was last reset.
func (f *foldFixture) repoContains() []string {
	f.t.Helper()
	var out []string
	for _, m := range f.client.containing(semsourceast.CodeContains) {
		var v any
		if err := json.Unmarshal(m, &v); err != nil {
			f.t.Fatalf("decode published message: %v", err)
		}
		walkTriples(v, func(subject, predicate string, object any) {
			if predicate == semsourceast.CodeContains && strings.Contains(subject, ".repo.") {
				if s, ok := object.(string); ok {
					out = append(out, s)
				}
			}
		})
	}
	sort.Strings(out)
	return out
}

func (f *foldFixture) reset() {
	f.client.mu.Lock()
	defer f.client.mu.Unlock()
	f.client.msgs = nil
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

// TestReindexKeepsSeedHierarchyOverAFold pins that the periodic reindex
// applies the seed's cross-file dedup: the folded a-b.ts never becomes a
// child of the repo, and the drops the seed counted are not counted again.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestReindexKeepsSeedHierarchyOverAFold(t *testing.T) {
	f := newFoldFixture(t)
	f.seed()
	seeded := f.repoContains()
	if len(seeded) == 0 {
		t.Fatal("seed published no repo contains triples; the fixture is vacuous")
	}
	for _, id := range seeded {
		if strings.Contains(id, ".file.") {
			t.Fatalf("seed already links the repo to a file: %v", seeded)
		}
	}
	collisions := f.c.idCollisions.Load()
	if collisions == 0 {
		t.Fatal("seed counted no collisions; the fixture does not fold")
	}

	for i := 0; i < 2; i++ {
		f.reset()
		f.reindex()
		// An unchanged repo is not republished (#401); a reindex that re-added
		// the folded file would change its set and so would publish it.
		if got := f.repoContains(); len(got) != 0 && strings.Join(got, ",") != strings.Join(seeded, ",") {
			t.Fatalf("reindex %d: repo contains = %v, seed published %v", i+1, got, seeded)
		}
	}
	if got := f.c.idCollisions.Load(); got != collisions {
		t.Errorf("id_collisions = %d after unchanged reindexes, want the seed's %d", got, collisions)
	}
}

// TestWatchRefusesAFoldedFile pins that editing a file the seed dropped as a
// folded duplicate publishes nothing on the watch path: its declarations
// would land under the survivor's IDs as a hybrid.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestWatchRefusesAFoldedFile(t *testing.T) {
	f := newFoldFixture(t)
	f.seed()
	f.reset()
	before := f.c.idCollisions.Load()

	f.write("a-b.ts", "export function qux() {}\n")
	f.watchModify("a-b.ts")
	f.settle()

	if got := f.client.containing("qux"); len(got) != 0 {
		t.Errorf("watch published %d messages for the folded file's edit", len(got))
	}
	if f.c.idCollisions.Load() <= before {
		t.Error("refused watch publish was not counted on id_collisions")
	}

	// The survivor is still watched normally.
	f.write("a/b.ts", "export function quux() {}\n")
	f.watchModify("a/b.ts")
	f.settle()
	if len(f.client.containing("quux")) == 0 {
		t.Error("the survivor's edit was not published")
	}
}

// TestWatchRefusalRacesReindex runs watch events for the folded file against
// concurrent reindex passes that rewrite the folded set (-race is the check),
// and asserts the edit never publishes from either goroutine.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestWatchRefusalRacesReindex(t *testing.T) {
	f := newFoldFixture(t)
	f.seed()
	f.reset()
	f.write("a-b.ts", "export function qux() {}\n")

	const rounds = 20
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			f.watchModify("a-b.ts")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			if _, _, err := f.c.reindexPath(f.ctx, f.pw); err != nil {
				t.Errorf("reindexPath: %v", err)
				return
			}
		}
	}()
	wg.Wait()
	f.settle()

	if got := f.client.containing("qux"); len(got) != 0 {
		t.Errorf("published %d messages for the folded file's edit", len(got))
	}
}

// TestReindexPromotesAFoldWhoseSurvivorIsGone pins what un-drops a folded
// path: once the survivor is deleted, the next reindex publishes the
// formerly folded file even though its content hash never changed.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestReindexPromotesAFoldWhoseSurvivorIsGone(t *testing.T) {
	f := newFoldFixture(t)
	f.seed()
	f.reset()

	if err := os.Remove(filepath.Join(f.root, "a", "b.ts")); err != nil {
		t.Fatal(err)
	}
	f.reindex()

	if len(f.client.containing("baz")) == 0 {
		t.Error("reindex did not publish a-b.ts after its survivor was deleted")
	}
	if f.pw.isFolded("a-b.ts") {
		t.Error("a-b.ts is still refused after its survivor was deleted")
	}
}

// TestReindexCountsANewFoldOfAnUnchangedFile pins that a fold first formed
// between passes is surfaced. The newcomer a/b.ts walks before the
// already-published a-b.ts, so it takes the shared file ID, and a-b.ts, whose
// hash never changed, is dropped whole. That drop must count, or
// id_collisions would read zero over a hybrid.
// spec: openspec/specs/entity-identity-safety/spec.md#Requirement: One entity ID per declaration within a parse pass
func TestReindexCountsANewFoldOfAnUnchangedFile(t *testing.T) {
	f := newFoldFixture(t)
	if err := os.RemoveAll(filepath.Join(f.root, "a")); err != nil {
		t.Fatal(err)
	}
	f.seed()
	if n := f.c.idCollisions.Load(); n != 0 {
		t.Fatalf("seed without a fold counted %d collisions", n)
	}

	f.write("a/b.ts", "export function foo() {}\n")
	f.reindex()
	if !f.pw.isFolded("a-b.ts") {
		t.Fatal("a-b.ts was not dropped as the new fold")
	}
	first := f.c.idCollisions.Load()
	if first == 0 {
		t.Error("the new fold was dropped silently: id_collisions = 0")
	}

	f.reindex()
	if n := f.c.idCollisions.Load(); n != first {
		t.Errorf("an unchanged reindex re-counted the known fold: %d -> %d", first, n)
	}
}
