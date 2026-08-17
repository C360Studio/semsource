package doc_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dochandler "github.com/c360studio/semsource/handler/doc"
)

// TestIngestEntityStates_ProjectOverride pins design D9 for docs: an explicit
// project replaces the path-derived system slug so submodule expansion can
// register the same canonical identity from any consumer checkout path.
// Absence of the override keeping today's IDs byte-for-byte is pinned by every
// other test in this package (none sets a project).
func TestIngestEntityStates_ProjectOverride(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"),
		[]byte("# Sub\n\nsubmodule doc body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := dochandler.NewWithOrg("acme",
		dochandler.WithProject("github-com-acme-shared-sub"),
		dochandler.WithBodyStore(newMemStore(), bodyStoreInstance))
	states, err := h.IngestEntityStates(
		context.Background(),
		sourceConfig{typ: "docs", path: root},
		"acme",
	)
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no entity states")
	}
	base := filepath.Base(root)
	for _, st := range states {
		if !strings.Contains(st.ID, "github-com-acme-shared-sub") {
			t.Errorf("ID %q does not carry the project system slug", st.ID)
		}
		if strings.Contains(st.ID, base) {
			t.Errorf("ID %q still carries the path-derived slug %q", st.ID, base)
		}
	}
}
