package golang_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/source/ast/golang"
)

// TestParseDirectory_SkipsGitBoundaries pins the no-double-ingestion contract
// (git-submodule-ingestion spec) for the Go parser walk: code inside a nested
// git working tree is another source's scope.
func TestParseDirectory_SkipsGitBoundaries(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main\n\nfunc Parent() {}\n")
	write("submod/.git", "gitdir: ../.git/modules/submod\n")
	write("submod/sub.go", "package sub\n\nfunc Hidden() {}\n")

	p := golang.NewParser("acme", "parent", root)
	results, err := p.ParseDirectory(context.Background(), root)
	if err != nil {
		t.Fatalf("ParseDirectory: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("parent main.go produced no parse results")
	}
	for _, r := range results {
		if r.FileEntity != nil && strings.Contains(filepath.ToSlash(r.FileEntity.Path), "submod/") {
			t.Errorf("file inside nested git tree parsed under parent scope: %s", r.FileEntity.Path)
		}
	}
}
