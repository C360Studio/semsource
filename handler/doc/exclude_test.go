package doc_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	source "github.com/c360studio/semsource/source/vocabulary"
)

// TestIngestEntityStates_DefaultExcludesPlanningDocs pins search-ranking-and-reach
// D3: OpenSpec planning artifacts (active AND archived — the graded re-run
// showed even active proposals outrank the README for product questions) and
// node_modules never enter the docs corpus; canonical docs incl. docs/adr
// stay indexed.
//
// This exercises IngestEntityStates because that is the path doc-source runs in
// production; the RawEntity path this test used to drive had no production
// callers and was deleted.
func TestIngestEntityStates_DefaultExcludesPlanningDocs(t *testing.T) {
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

	h, _ := docsHandler(t)
	states, err := h.IngestEntityStates(
		context.Background(),
		sourceConfig{typ: "docs", path: root},
		"acme",
	)
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}

	paths := map[string]bool{}
	for _, st := range states {
		for _, tr := range st.Triples {
			if tr.Predicate == source.DocFilePath {
				if fp, ok := tr.Object.(string); ok {
					paths[filepath.ToSlash(fp)] = true
				}
			}
		}
	}
	for _, want := range []string{
		"README.md",
		"docs/adr/0001-decision.md",
	} {
		if !paths[want] {
			t.Errorf("expected %s in corpus; got %v", want, paths)
		}
	}
	for _, banned := range []string{
		"openspec/changes/active-change/proposal.md",
		"openspec/changes/archive/2026-01-01-old/proposal.md",
		"openspec/specs/cap/spec.md",
		"ui/node_modules/pkg/README.md",
	} {
		if paths[banned] {
			t.Errorf("excluded path %s entered the corpus", banned)
		}
	}
}
