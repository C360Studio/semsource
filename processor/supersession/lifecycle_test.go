package supersession

import (
	"testing"

	semsourceast "github.com/c360studio/semsource/source/ast"
	source "github.com/c360studio/semsource/source/vocabulary"
	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
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
		{"acme.semsource.golang.github-com-acme-repo.file.main-go", "github-com-acme-repo"},
		{"acme.semsource.web.docs-root.doc.readme-md", "docs-root"},
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
		codeEntityAt("acme.semsource.golang.sys.file.a-go", "a.go", false),
		docEntity("acme.semsource.web.sys.doc.b-md", "b.md", false),
	}
	toMark, toClear, paths := decideLifecycleActions(entities, "", source.LifecycleReasonSourceRemoved, nil)

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
		codeEntityAt("acme.semsource.golang.sys.file.a-go", "a.go", true), // already marked
	}
	toMark, toClear, _ := decideLifecycleActions(entities, "", source.LifecycleReasonSourceRemoved, nil)
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
		codeEntityAt("acme.semsource.golang.sys.file.a-go", "a.go", false),
	}
	stat := func(_ string) bool { return false } // every path missing
	toMark, toClear, paths := decideLifecycleActions(entities, "/repo", source.LifecycleReasonFileDeleted, stat)

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
		codeEntityAt("acme.semsource.golang.sys.file.a-go", "a.go", true), // already marked
	}
	stat := func(_ string) bool { return false }
	toMark, toClear, _ := decideLifecycleActions(entities, "/repo", source.LifecycleReasonPathMissing, stat)
	if len(toMark) != 0 || len(toClear) != 0 {
		t.Errorf("expected no action for an already-marked, still-missing entity: toMark=%v toClear=%v", toMark, toClear)
	}
}

func TestDecideLifecycleActions_PresentPath_ClearsMarkedEntity(t *testing.T) {
	entities := []gtypes.EntityState{
		codeEntityAt("acme.semsource.golang.sys.file.a-go", "a.go", true), // was marked, file is back
	}
	stat := func(_ string) bool { return true } // every path present
	toMark, toClear, _ := decideLifecycleActions(entities, "/repo", source.LifecycleReasonFileDeleted, stat)

	if len(toMark) != 0 {
		t.Errorf("toMark = %v, want empty", toMark)
	}
	if len(toClear) != 1 || toClear[0] != "acme.semsource.golang.sys.file.a-go" {
		t.Fatalf("toClear = %v, want the single reappeared entity", toClear)
	}
}

func TestDecideLifecycleActions_PresentPath_UnmarkedIsNoOp(t *testing.T) {
	entities := []gtypes.EntityState{
		codeEntityAt("acme.semsource.golang.sys.file.a-go", "a.go", false),
	}
	stat := func(_ string) bool { return true }
	toMark, toClear, _ := decideLifecycleActions(entities, "/repo", source.LifecycleReasonFileDeleted, stat)
	if len(toMark) != 0 || len(toClear) != 0 {
		t.Errorf("expected no action for a live, never-marked entity: toMark=%v toClear=%v", toMark, toClear)
	}
}

func TestDecideLifecycleActions_NoPathPredicate_Skipped(t *testing.T) {
	entities := []gtypes.EntityState{
		{ID: "acme.semsource.golang.sys.package.pkg", Triples: nil}, // no path predicate at all
	}
	stat := func(_ string) bool { t.Fatalf("stat should never be called for a pathless entity"); return false }
	toMark, toClear, paths := decideLifecycleActions(entities, "/repo", source.LifecycleReasonFileDeleted, stat)
	if len(toMark) != 0 || len(toClear) != 0 || paths != 0 {
		t.Errorf("expected no action for a pathless entity: toMark=%v toClear=%v paths=%d", toMark, toClear, paths)
	}
}

func TestDecideLifecycleActions_GroupsMultipleEntitiesUnderOnePath(t *testing.T) {
	// Two symbols from the same deleted file both get marked from one stat.
	entities := []gtypes.EntityState{
		codeEntityAt("acme.semsource.golang.sys.function.a-go-Foo", "a.go", false),
		codeEntityAt("acme.semsource.golang.sys.function.a-go-Bar", "a.go", false),
	}
	stat := func(_ string) bool { return false }
	toMark, _, paths := decideLifecycleActions(entities, "/repo", source.LifecycleReasonFileDeleted, stat)
	if paths != 1 {
		t.Errorf("Paths = %d, want 1 (single distinct path)", paths)
	}
	if len(toMark) != 2 {
		t.Errorf("toMark count = %d, want 2 (both symbols from the one deleted file)", len(toMark))
	}
}
