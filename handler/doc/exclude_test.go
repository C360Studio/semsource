package doc_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/handler/doc"
)

// TestIngest_DefaultExcludesArchivedPlanningDocs pins search-ranking-and-reach
// D3: archived OpenSpec planning artifacts and node_modules never enter the
// docs corpus (the audit's doc_context misses cited archive entries over
// canonical docs), while active changes, specs, and docs/adr stay indexed.
func TestIngest_DefaultExcludesArchivedPlanningDocs(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("README.md", "# Canonical\n")
	write("docs/adr/0001-decision.md", "# ADR\n")
	write("openspec/changes/active-change/proposal.md", "# Active Proposal\n")
	write("openspec/changes/archive/2026-01-01-old/proposal.md", "# Archived Proposal\n")
	write("openspec/specs/cap/spec.md", "# Spec\n")
	write("ui/node_modules/pkg/README.md", "# Vendored\n")

	entities, err := doc.New().Ingest(context.Background(), sourceConfig{typ: "docs", path: root})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	paths := map[string]bool{}
	for _, e := range entities {
		fp, _ := e.Properties["file_path"].(string)
		paths[filepath.ToSlash(fp)] = true
	}
	for _, want := range []string{
		"README.md",
		"docs/adr/0001-decision.md",
		"openspec/changes/active-change/proposal.md",
		"openspec/specs/cap/spec.md",
	} {
		if !paths[want] {
			t.Errorf("expected %s in corpus; got %v", want, paths)
		}
	}
	for _, banned := range []string{
		"openspec/changes/archive/2026-01-01-old/proposal.md",
		"ui/node_modules/pkg/README.md",
	} {
		if paths[banned] {
			t.Errorf("excluded path %s entered the corpus", banned)
		}
	}
}
