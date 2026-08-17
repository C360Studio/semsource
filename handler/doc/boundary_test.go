package doc_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	source "github.com/c360studio/semsource/source/vocabulary"
)

// TestIngestEntityStates_SkipsGitBoundaries pins the no-double-ingestion
// contract (git-submodule-ingestion spec) for the docs walk: files inside a
// nested git working tree (submodule gitlink or foreign repo) are another
// source's scope.
func TestIngestEntityStates_SkipsGitBoundaries(t *testing.T) {
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
	write("README.md", "# Parent\n\nparent doc body\n")
	write("submod/.git", "gitdir: ../.git/modules/submod\n")
	write("submod/README.md", "# Submodule\n\nsubmodule doc body\n")
	if err := os.MkdirAll(filepath.Join(root, "foreign/.git"), 0o755); err != nil {
		t.Fatal(err)
	}
	write("foreign/NOTES.md", "# Foreign\n\nforeign repo body\n")

	h, _ := docsHandler(t)
	states, err := h.IngestEntityStates(
		context.Background(),
		sourceConfig{typ: "docs", path: root},
		"acme",
	)
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}

	var paths []string
	for _, st := range states {
		for _, tr := range st.Triples {
			if tr.Predicate == source.DocFilePath {
				if fp, ok := tr.Object.(string); ok {
					paths = append(paths, filepath.ToSlash(fp))
				}
			}
		}
	}
	if len(paths) == 0 {
		t.Fatal("parent README.md produced no doc entities")
	}
	for _, p := range paths {
		if p != "README.md" {
			t.Errorf("doc from nested git tree attributed to parent scope: %s", p)
		}
	}
}
