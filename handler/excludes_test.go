package handler

import "testing"

func TestIsDefaultExcludedDir(t *testing.T) {
	for _, name := range []string{"node_modules", ".git"} {
		if !IsDefaultExcludedDir(name) {
			t.Errorf("%q must be excluded by every source", name)
		}
	}
	// vendor is deliberately NOT in the floor: Go's vendor directory holds real
	// dependency source a deployment may want indexed, so it stays a per-source
	// decision. Pinned so re-adding it is a deliberate act.
	for _, name := range []string{"vendor", "src", "docs", "internal", ""} {
		if IsDefaultExcludedDir(name) {
			t.Errorf("%q must not be in the floor", name)
		}
	}
}

func TestIsDefaultExcludedPath(t *testing.T) {
	excluded := []string{
		"ui/node_modules/pkg/README.md",
		"node_modules/x.js",
		"a/b/.git/config",
	}
	for _, p := range excluded {
		if !IsDefaultExcludedPath(p) {
			t.Errorf("%q should be excluded (a segment is a floor directory)", p)
		}
	}
	kept := []string{
		"README.md",
		"handler/doc/handler.go",
		// Substring, not a segment: a directory merely NAMED like one must not
		// be pruned, or a project with a "node_modules_notes" folder loses it.
		"docs/node_modules_notes/x.md",
		"src/vendor/lib.go",
	}
	for _, p := range kept {
		if IsDefaultExcludedPath(p) {
			t.Errorf("%q should be kept", p)
		}
	}
}

func TestDefaultExcludedDirNamesMatchesPredicate(t *testing.T) {
	names := DefaultExcludedDirNames()
	if len(names) == 0 {
		t.Fatal("floor is empty")
	}
	for _, n := range names {
		if !IsDefaultExcludedDir(n) {
			t.Errorf("%q listed but not matched — the two views disagree", n)
		}
	}
}
