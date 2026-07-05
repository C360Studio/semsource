package astsource

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	semsourceast "github.com/c360studio/semsource/source/ast"
)

// TestParseFileWithWatcher_ConcurrentSafe guards the fix for the data race the
// adversarial review of task #44 proved: the fsnotify watcher and the periodic
// reindex goroutine drive the SAME parser instance, whose per-file state (the
// tree-sitter parser and the language import-binding map) is unsafe to interleave.
// parseFileWithWatcher must serialize ParseFile per source path. Without the mutex
// this fails under -race (and can hard-crash on the shared native parser).
func TestParseFileWithWatcher_ConcurrentSafe(t *testing.T) {
	root := t.TempDir()
	writePy := func(rel, src string) string {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
		return full
	}

	// Two referrers with DIFFERENT imports so an interleaved parse would resolve
	// one file's references against the other's import bindings.
	fileA := writePy("pkg/a.py", "from pkg.base import BaseA\n\nclass A(BaseA):\n    pass\n")
	fileB := writePy("pkg/b.py", "from pkg.other import BaseB\n\nclass B(BaseB):\n    pass\n")
	writePy("pkg/base.py", "class BaseA:\n    pass\n")
	writePy("pkg/other.py", "class BaseB:\n    pass\n")

	parser, err := semsourceast.DefaultRegistry.CreateParser("python", "acme", "proj", root)
	if err != nil {
		t.Fatalf("create parser: %v", err)
	}
	pw := &pathWatcher{parsers: map[string]semsourceast.FileParser{"python": parser}}
	c := &Component{}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = c.parseFileWithWatcher(context.Background(), pw, fileA) }()
		go func() { defer wg.Done(); _, _ = c.parseFileWithWatcher(context.Background(), pw, fileB) }()
	}
	wg.Wait()
}
