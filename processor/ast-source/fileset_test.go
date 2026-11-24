package astsource

import (
	"slices"
	"testing"

	semsourceast "github.com/c360studio/semsource/source/ast"
)

func result(path string) *semsourceast.ParseResult {
	return &semsourceast.ParseResult{Path: path, FileEntity: &semsourceast.CodeEntity{ID: "id-" + path}}
}

// spec: openspec/specs/entity-publish-integrity/spec.md#Requirement: AST folder and repo publishes carry their complete contains set
func TestFileSet(t *testing.T) {
	pw := &pathWatcher{}

	// A full pass reports every path it added; a folded result (nil file
	// entity) is never in the set.
	folded := &semsourceast.ParseResult{Path: "a/folded.go"}
	got := pw.replaceFiles([]*semsourceast.ParseResult{result("a/one.go"), result("a/b/x.go"), result("a/bc.go"), folded})
	slices.Sort(got)
	if want := []string{"a/b/x.go", "a/bc.go", "a/one.go"}; !slices.Equal(got, want) {
		t.Fatalf("first replace changed %v, want %v", got, want)
	}

	// A modify of an indexed path changes nothing; a new path does.
	if pw.addFile("a/one.go", "id-a/one.go") {
		t.Error("addFile of an indexed path reported a change")
	}
	if !pw.addFile("a/b/y.go", "id-a/b/y.go") {
		t.Error("addFile of a new path reported no change")
	}

	// A watch-created file folding onto an indexed one's ID is not added.
	if pw.addFile("a-one.go", "id-a/one.go") {
		t.Error("addFile of a path whose ID another path holds reported a change")
	}

	// A directory delete removes everything under it and nothing that only
	// shares its name as a prefix (a/bc.go).
	removed := pw.removeFiles("a/b")
	slices.Sort(removed)
	if want := []string{"a/b/x.go", "a/b/y.go"}; !slices.Equal(removed, want) {
		t.Fatalf("removeFiles(a/b) = %v, want %v", removed, want)
	}
	if removed := pw.removeFiles("a/b"); len(removed) != 0 {
		t.Errorf("second removeFiles(a/b) = %v, want nothing", removed)
	}

	// The next full pass reports both additions and removals.
	got = pw.replaceFiles([]*semsourceast.ParseResult{result("a/one.go"), result("a/new.go")})
	slices.Sort(got)
	if want := []string{"a/bc.go", "a/new.go"}; !slices.Equal(got, want) {
		t.Fatalf("second replace changed %v, want %v", got, want)
	}
}
