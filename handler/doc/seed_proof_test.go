package doc_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/internal/seedproof"
)

func TestReactivationSeedReportsSkippedDocument(t *testing.T) {
	root := t.TempDir()
	writeMD(t, root, "current.md", "# Current\nCurrent content.")
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(root, "unreadable.md")); err != nil {
		t.Fatal(err)
	}
	h, _ := docsHandler(t)
	ctx, proof := seedproof.Begin(context.Background())
	states, err := h.IngestEntityStates(ctx, sourceConfig{typ: "docs", path: root}, testAuthority("acme"))
	if err != nil {
		t.Fatal(err)
	}
	if len(states) == 0 {
		t.Fatal("valid sibling did not ingest")
	}
	if proof.Err() == nil {
		t.Fatal("partial document enumeration falsely proved complete")
	}
	if _, err := h.IngestEntityStates(context.Background(), sourceConfig{typ: "docs", path: root}, testAuthority("acme")); err != nil {
		t.Fatalf("ordinary ingest semantics changed: %v", err)
	}
}
