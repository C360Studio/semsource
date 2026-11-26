package supersession

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semsource/graph"
	semsourceast "github.com/c360studio/semsource/source/ast"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// docEntity builds an enumerated EntityState carrying the doc path predicate,
// optionally already marked stale.
func docEntity(id, path string, marked bool) gtypes.EntityState {
	tr := []message.Triple{
		{Subject: id, Predicate: source.DocFilePath, Object: path},
	}
	if marked {
		tr = append(tr, message.Triple{Subject: id, Predicate: source.EntityLifecycleStale, Object: "path_missing"})
	}
	return gtypes.EntityState{ID: id, Triples: tr}
}

// codeEntityAt builds an enumerated code EntityState carrying only the path
// predicate the lifecycle pass reads, optionally already marked stale.
func codeEntityAt(id, path string, marked bool) gtypes.EntityState {
	tr := []message.Triple{
		{Subject: id, Predicate: semsourceast.CodePath, Object: path},
	}
	if marked {
		tr = append(tr, message.Triple{Subject: id, Predicate: source.EntityLifecycleStale, Object: "path_missing"})
	}
	return gtypes.EntityState{ID: id, Triples: tr}
}

func TestEntityIDSystem(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"acme.semsource.github-com-acme-repo.golang.file.main-go", "github-com-acme-repo"},
		{"acme.semsource.docs-root.web.doc.readme-md", "docs-root"},
		{"malformed", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := entityIDSystem(tt.id); got != tt.want {
			t.Errorf("entityIDSystem(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestIsMarkedStale(t *testing.T) {
	if isMarkedStale(nil) {
		t.Error("nil triples should not be marked")
	}
	marked := []message.Triple{{Predicate: source.EntityLifecycleStale, Object: "file_deleted"}}
	if !isMarkedStale(marked) {
		t.Error("expected marked triples to report true")
	}
}

func TestPathOf(t *testing.T) {
	codeTriples := []message.Triple{{Predicate: semsourceast.CodePath, Object: "pkg/run.go"}}
	if p, ok := pathOf(codeTriples); !ok || p != "pkg/run.go" {
		t.Errorf("pathOf(code) = (%q,%v), want (pkg/run.go,true)", p, ok)
	}

	docTriples := []message.Triple{{Predicate: source.DocFilePath, Object: "readme.md"}}
	if p, ok := pathOf(docTriples); !ok || p != "readme.md" {
		t.Errorf("pathOf(doc) = (%q,%v), want (readme.md,true)", p, ok)
	}

	if _, ok := pathOf(nil); ok {
		t.Error("pathOf(nil) should report false")
	}
}

// --- decideLifecycleActions: remove_source shape (RootPath empty) ----------

func TestDecideLifecycleActions_RemoveSource_MarksEveryUnmarkedEntity(t *testing.T) {
	entities := []gtypes.EntityState{
		codeEntityAt("acme.semsource.sys.golang.file.a-go", "a.go", false),
		docEntity("acme.semsource.sys.web.doc.b-md", "b.md", false),
	}
	toMark, toClear, paths := decideLifecycleActions(entities, source.LifecycleReasonSourceRemoved, nil)

	if paths != 0 {
		t.Errorf("Paths = %d, want 0 (no filesystem check on remove_source)", paths)
	}
	if len(toClear) != 0 {
		t.Errorf("toClear = %v, want empty", toClear)
	}
	if len(toMark) != 2 {
		t.Fatalf("toMark count = %d, want 2", len(toMark))
	}
	for _, tr := range toMark {
		if tr.Predicate != source.EntityLifecycleStale || tr.Object != source.LifecycleReasonSourceRemoved {
			t.Errorf("unexpected marker triple: %+v", tr)
		}
	}
}

func TestDecideLifecycleActions_RemoveSource_SkipsAlreadyMarked(t *testing.T) {
	entities := []gtypes.EntityState{
		codeEntityAt("acme.semsource.sys.golang.file.a-go", "a.go", true), // already marked
	}
	toMark, toClear, _ := decideLifecycleActions(entities, source.LifecycleReasonSourceRemoved, nil)
	if len(toMark) != 0 {
		t.Errorf("toMark = %v, want empty (idempotent — already marked)", toMark)
	}
	if len(toClear) != 0 {
		t.Errorf("toClear = %v, want empty (remove_source never clears)", toClear)
	}
}

// --- decideLifecycleActions: RootPath set (file-delete / periodic sweep) ---

func TestDecideLifecycleActions_MissingPath_MarksUnmarkedEntity(t *testing.T) {
	entities := []gtypes.EntityState{
		codeEntityAt("acme.semsource.sys.golang.file.a-go", "a.go", false),
	}
	stat := func(_ string) bool { return false } // every path missing
	toMark, toClear, paths := decideLifecycleActions(entities, source.LifecycleReasonFileDeleted, stat)

	if paths != 1 {
		t.Errorf("Paths = %d, want 1", paths)
	}
	if len(toClear) != 0 {
		t.Errorf("toClear = %v, want empty", toClear)
	}
	if len(toMark) != 1 || toMark[0].Object != source.LifecycleReasonFileDeleted {
		t.Fatalf("toMark = %+v, want one file_deleted marker", toMark)
	}
}

func TestDecideLifecycleActions_MissingPath_AlreadyMarkedIsNoOp(t *testing.T) {
	entities := []gtypes.EntityState{
		codeEntityAt("acme.semsource.sys.golang.file.a-go", "a.go", true), // already marked
	}
	stat := func(_ string) bool { return false }
	toMark, toClear, _ := decideLifecycleActions(entities, source.LifecycleReasonPathMissing, stat)
	if len(toMark) != 0 || len(toClear) != 0 {
		t.Errorf("expected no action for an already-marked, still-missing entity: toMark=%v toClear=%v", toMark, toClear)
	}
}

func TestDecideLifecycleActions_PresentPath_ClearsMarkedEntity(t *testing.T) {
	entities := []gtypes.EntityState{
		codeEntityAt("acme.semsource.sys.golang.file.a-go", "a.go", true), // was marked, file is back
	}
	stat := func(_ string) bool { return true } // every path present
	toMark, toClear, _ := decideLifecycleActions(entities, source.LifecycleReasonFileDeleted, stat)

	if len(toMark) != 0 {
		t.Errorf("toMark = %v, want empty", toMark)
	}
	if len(toClear) != 1 || toClear[0] != "acme.semsource.sys.golang.file.a-go" {
		t.Fatalf("toClear = %v, want the single reappeared entity", toClear)
	}
}

func TestDecideLifecycleActions_PresentPath_UnmarkedIsNoOp(t *testing.T) {
	entities := []gtypes.EntityState{
		codeEntityAt("acme.semsource.sys.golang.file.a-go", "a.go", false),
	}
	stat := func(_ string) bool { return true }
	toMark, toClear, _ := decideLifecycleActions(entities, source.LifecycleReasonFileDeleted, stat)
	if len(toMark) != 0 || len(toClear) != 0 {
		t.Errorf("expected no action for a live, never-marked entity: toMark=%v toClear=%v", toMark, toClear)
	}
}

func TestDecideLifecycleActions_NoPathPredicate_Skipped(t *testing.T) {
	entities := []gtypes.EntityState{
		{ID: "acme.semsource.sys.golang.package.pkg", Triples: nil}, // no path predicate at all
	}
	stat := func(_ string) bool { t.Fatalf("stat should never be called for a pathless entity"); return false }
	toMark, toClear, paths := decideLifecycleActions(entities, source.LifecycleReasonFileDeleted, stat)
	if len(toMark) != 0 || len(toClear) != 0 || paths != 0 {
		t.Errorf("expected no action for a pathless entity: toMark=%v toClear=%v paths=%d", toMark, toClear, paths)
	}
}

func TestDecideLifecycleActions_GroupsMultipleEntitiesUnderOnePath(t *testing.T) {
	// Two symbols from the same deleted file both get marked from one stat.
	entities := []gtypes.EntityState{
		codeEntityAt("acme.semsource.sys.golang.function.a-go-Foo", "a.go", false),
		codeEntityAt("acme.semsource.sys.golang.function.a-go-Bar", "a.go", false),
	}
	stat := func(_ string) bool { return false }
	toMark, _, paths := decideLifecycleActions(entities, source.LifecycleReasonFileDeleted, stat)
	if paths != 1 {
		t.Errorf("Paths = %d, want 1 (single distinct path)", paths)
	}
	if len(toMark) != 2 {
		t.Errorf("toMark count = %d, want 2 (both symbols from the one deleted file)", len(toMark))
	}
}

// TestLifecycleScopeJudgesOnlyItsOwnRoot pins #286: every root configured
// with one project shares a system slug, and a sweep may judge only the
// entities its own root produced — an AST root against a docs root, two docs
// roots with disjoint filenames, and never an entity that carries no owner.
// spec: openspec/specs/entity-staleness/spec.md#Requirement: A staleness sweep judges only the entities its own root produced
func TestLifecycleScopeJudgesOnlyItsOwnRoot(t *testing.T) {
	owned := func(id, pathPred, path, owner string) gtypes.EntityState {
		tr := []message.Triple{{Predicate: pathPred, Object: path}}
		if owner != "" {
			tr = append(tr, message.Triple{Predicate: source.EntityLifecycleOwner, Object: owner})
		}
		return gtypes.EntityState{ID: id, Triples: tr}
	}
	astOwner := graph.LifecycleOwner("ast-source-svc", "/r")
	docsA := graph.LifecycleOwner("doc-source-svc", "/r/docs-a")
	docsB := graph.LifecycleOwner("doc-source-svc", "/r/docs-b")
	code := owned("acme.p.svc.golang.function.Live", semsourceast.CodePath, "live.go", astOwner)
	a := owned("acme.p.svc.web.doc.a-md", source.DocFilePath, "a.md", docsA)
	b := owned("acme.p.svc.web.doc.b-md", source.DocFilePath, "b.md", docsB)
	unowned := owned("acme.p.svc.web.doc.c-md", source.DocFilePath, "c.md", "")
	foreign := owned("acme.p.other.web.doc.d-md", source.DocFilePath, "d.md", docsA)
	all := []gtypes.EntityState{code, a, b, unowned, foreign}
	systems := map[string]struct{}{"svc": {}}

	ids := func(es []gtypes.EntityState) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.ID)
		}
		return out
	}
	for name, tc := range map[string]struct {
		owner string
		want  []string
	}{
		"AST root":    {astOwner, []string{code.ID}},
		"docs root A": {docsA, []string{a.ID}},
		"docs root B": {docsB, []string{b.ID}},
	} {
		if got := ids(lifecycleScope(all, systems, tc.owner, true)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: scope = %v, want %v", name, got, tc.want)
		}
	}
	if docsA == docsB || docsA == astOwner {
		t.Fatalf("owner keys must separate roots and instances: %q %q %q", astOwner, docsA, docsB)
	}
}

// TestLivenessOracleRequiresAnOwner: an oracle request with no owner would
// judge every root of the system, so it is refused.
// spec: openspec/specs/entity-staleness/spec.md#Requirement: A staleness sweep judges only the entities its own root produced
func TestLivenessOracleRequiresAnOwner(t *testing.T) {
	for name, req := range map[string]graph.LifecycleRunRequest{
		"root without owner":   {RootPath: "/r"},
		"absent without owner": {Absent: []string{}},
	} {
		if _, err := livenessOracle(req); err == nil {
			t.Errorf("%s: accepted, want refused", name)
		}
	}
}

// A config-file entity is judged by its declaring manifest, whose path fact
// is absolute; a relative code or doc path is still joined to the root (#393).
// spec: entity-staleness / Config-file entities go stale with their manifest
func TestLifecyclePassJudgesConfigFilesByTheirAbsolutePath(t *testing.T) {
	root := t.TempDir()
	present := filepath.Join(root, "svc", "go.mod")
	if err := os.MkdirAll(filepath.Dir(present), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(present, []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "run.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(root, "web", "package.json")

	cfgEntity := func(id, path string, marked bool) gtypes.EntityState {
		tr := []message.Triple{{Subject: id, Predicate: source.ConfigFilePath, Object: path}}
		if marked {
			tr = append(tr, message.Triple{Subject: id, Predicate: source.EntityLifecycleStale, Object: "file_deleted"})
		}
		return gtypes.EntityState{ID: id, Triples: tr}
	}
	if p, ok := pathOf(cfgEntity("x", present, false).Triples); !ok || p != present {
		t.Fatalf("pathOf(config) = (%q,%v), want (%q,true)", p, ok, present)
	}

	stat, err := livenessOracle(graph.LifecycleRunRequest{RootPath: root, Owner: "cfgfile-source/abc"})
	if err != nil {
		t.Fatal(err)
	}
	toMark, toClear, _ := decideLifecycleActions([]gtypes.EntityState{
		cfgEntity("dep-of-live", present, true),
		cfgEntity("dep-of-gone", gone, false),
		codeEntityAt("code", "run.go", true),
	}, graph.LifecycleReasonFileDeleted, stat)

	var marked []string
	for _, tr := range toMark {
		marked = append(marked, tr.Subject)
	}
	if want := []string{"dep-of-gone"}; !reflect.DeepEqual(marked, want) {
		t.Errorf("marked = %v, want %v", marked, want)
	}
	sort.Strings(toClear)
	if want := []string{"code", "dep-of-live"}; !reflect.DeepEqual(toClear, want) {
		t.Errorf("cleared = %v, want %v", toClear, want)
	}
}
