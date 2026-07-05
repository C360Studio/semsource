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

// TestParseFileWithWatcher_ConcurrentSafe_MultiLang extends the guard above to the
// Java, TypeScript, and Go parsers, which gained per-file resolver state in task
// #46 (import maps, same-file kind tables, the Go same-package type map). The same
// two-goroutine interleave over a reused parser must stay race-free under the
// parseFileWithWatcher mutex.
func TestParseFileWithWatcher_ConcurrentSafe_MultiLang(t *testing.T) {
	cases := []struct {
		lang       string
		fileA      [2]string // {relPath, source}
		fileB      [2]string
		extraFiles map[string]string
	}{
		{
			lang:  "java",
			fileA: [2]string{"a/A.java", "package a;\nimport a.base.BaseA;\npublic class A extends BaseA {}\n"},
			fileB: [2]string{"a/B.java", "package a;\nimport a.other.BaseB;\npublic class B extends BaseB {}\n"},
			extraFiles: map[string]string{
				"a/base/BaseA.java":  "package a.base;\npublic class BaseA {}\n",
				"a/other/BaseB.java": "package a.other;\npublic class BaseB {}\n",
			},
		},
		{
			lang:  "typescript",
			fileA: [2]string{"a.ts", "import { BaseA } from './basea';\nexport class A extends BaseA {}\n"},
			fileB: [2]string{"b.ts", "import { BaseB } from './baseb';\nexport class B extends BaseB {}\n"},
			extraFiles: map[string]string{
				"basea.ts": "export class BaseA {}\n",
				"baseb.ts": "export class BaseB {}\n",
			},
		},
		{
			lang:       "go",
			fileA:      [2]string{"a.go", "package p\ntype A struct{ Base }\n"},
			fileB:      [2]string{"b.go", "package p\ntype B struct{ Base }\n"},
			extraFiles: map[string]string{"base.go": "package p\ntype Base struct{}\n"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			root := t.TempDir()
			write := func(rel, src string) string {
				full := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
					t.Fatalf("write %s: %v", rel, err)
				}
				return full
			}
			fileA := write(tc.fileA[0], tc.fileA[1])
			fileB := write(tc.fileB[0], tc.fileB[1])
			for rel, src := range tc.extraFiles {
				write(rel, src)
			}

			parser, err := semsourceast.DefaultRegistry.CreateParser(tc.lang, "acme", "proj", root)
			if err != nil {
				t.Fatalf("create parser: %v", err)
			}
			pw := &pathWatcher{parsers: map[string]semsourceast.FileParser{tc.lang: parser}}
			c := &Component{}

			var wg sync.WaitGroup
			for i := 0; i < 50; i++ {
				wg.Add(2)
				go func() { defer wg.Done(); _, _ = c.parseFileWithWatcher(context.Background(), pw, fileA) }()
				go func() { defer wg.Done(); _, _ = c.parseFileWithWatcher(context.Background(), pw, fileB) }()
			}
			wg.Wait()
		})
	}
}
