package cfgfile_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler/cfgfile"
)

// TestIngestEntityStates_SkipsGitBoundaries pins the no-double-ingestion
// contract (git-submodule-ingestion spec): a nested git working tree — a
// submodule (gitlink .git file) or a foreign repo (.git directory) — is
// another source's scope, and its config files must not be attributed to
// this walk's identity. An empty (unmaterialized) submodule dir yields
// nothing and no error.
func TestIngestEntityStates_SkipsGitBoundaries(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("go.mod", "module example.com/parent\n\ngo 1.21\n")
	// Submodule: .git is a gitlink FILE.
	write("submod/.git", "gitdir: ../.git/modules/submod\n")
	write("submod/go.mod", "module example.com/submod\n\ngo 1.21\n")
	// Foreign nested repo: .git is a directory.
	if err := os.MkdirAll(filepath.Join(dir, "foreign/.git"), 0o755); err != nil {
		t.Fatal(err)
	}
	write("foreign/go.mod", "module example.com/foreign\n\ngo 1.21\n")
	// Unmaterialized submodule: declared dir, no content at all.
	if err := os.MkdirAll(filepath.Join(dir, "emptysub"), 0o755); err != nil {
		t.Fatal(err)
	}

	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("parent go.mod produced no entity states")
	}
	// Paths travel in triples (IDs are hash-based); no triple may reference
	// a file under either nested tree.
	for _, st := range states {
		for _, tr := range st.Triples {
			if s, ok := tr.Object.(string); ok &&
				(strings.Contains(s, "submod") || strings.Contains(s, "foreign")) {
				t.Errorf("triple from nested git tree attributed to parent scope: %s %v", st.ID, s)
			}
		}
	}
}
