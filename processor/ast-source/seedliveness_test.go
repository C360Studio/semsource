package astsource

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	semsourceast "github.com/c360studio/semsource/source/ast"
)

// TestParseDirectoryAdvancesFilesParsed — parse work must advance the
// pre-publish liveness counter (async-source-seed 5.7): during a watch path's
// parse phase the publish count is flat, and this counter is what proves the
// seed is working rather than wedged. A file with no routed parser is not
// parse work and must not count.
func TestParseDirectoryAdvancesFilesParsed(t *testing.T) {
	root := t.TempDir()
	write := func(rel, src string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.py", "class A:\n    pass\n")
	write("pkg/b.py", "class B:\n    pass\n")
	write("notes.txt", "not code — no parser routes .txt\n")

	parser, err := semsourceast.DefaultRegistry.CreateParser("python", "acme", "proj", root)
	if err != nil {
		t.Fatalf("create parser: %v", err)
	}
	pw := &pathWatcher{
		root:     root,
		parsers:  map[string]semsourceast.FileParser{"python": parser},
		routes:   map[string]string{".py": "python"},
		excludes: map[string]bool{},
	}
	c := &Component{logger: slog.Default()}

	results, err := c.parseDirectory(context.Background(), pw)
	if err != nil {
		t.Fatalf("parseDirectory: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("parsed results = %d, want 2", len(results))
	}
	if got := c.filesParsed.Load(); got != 2 {
		t.Errorf("filesParsed = %d, want 2 (the .txt file has no parser and is not parse work)", got)
	}
}
