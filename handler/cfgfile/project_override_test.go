package cfgfile_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler/cfgfile"
)

// TestIngestEntityStates_ProjectOverride pins design D9: an explicit project
// replaces the path-derived system slug so submodule expansion can register
// the same canonical identity from any consumer checkout path. Absence of the
// override keeping today's IDs byte-for-byte is pinned by every other test in
// this package (none sets Project).
func TestIngestEntityStates_ProjectOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/sub\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := cfgfile.New(&cfgfile.Config{Project: "github-com-acme-shared-sub"})
	cfg := &stubSourceConfig{sourceType: "config", path: dir}
	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no entity states")
	}
	base := filepath.Base(dir)
	for _, st := range states {
		if !strings.Contains(st.ID, "github-com-acme-shared-sub") {
			t.Errorf("ID %q does not carry the project system slug", st.ID)
		}
		if strings.Contains(st.ID, base) {
			t.Errorf("ID %q still carries the path-derived slug %q", st.ID, base)
		}
	}
}
