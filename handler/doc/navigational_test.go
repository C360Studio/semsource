package doc_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	source "github.com/c360studio/semsource/source/vocabulary"
)

// TestParentIsMarkedNavigationalAndPassagesAreNot pins the fix for the defect the
// scorecard found: body-less parents outranking their own passages, so a caller's
// first citation contained nothing. The parent carries the demotion marker; a
// passage, which does carry a body, must never carry it.
func TestParentIsMarkedNavigationalAndPassagesAreNot(t *testing.T) {
	root := t.TempDir()
	body := "# Guide\n\n" + strings.Repeat("Prose about the thing. ", 60) +
		"\n\n## Details\n\n" + strings.Repeat("More prose here. ", 60) + "\n"
	if err := os.WriteFile(filepath.Join(root, "guide.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	h, _ := docsHandler(t)
	states, err := h.IngestEntityStates(context.Background(), sourceConfig{typ: "docs", path: root}, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}

	parents, passages := parentStates(states), passageStates(states)
	if len(parents) != 1 || len(passages) < 2 {
		t.Fatalf("want 1 parent and multiple passages, got %d/%d", len(parents), len(passages))
	}

	if got, ok := optionalTripleValue(parents[0], source.EntityRoleNavigational); !ok {
		t.Error("parent carries no navigational marker; it would outrank its own passages")
	} else if got != source.NavigationalDocument {
		t.Errorf("navigational marker = %q, want %q", got, source.NavigationalDocument)
	}

	for _, p := range passages {
		if _, ok := optionalTripleValue(p, source.EntityRoleNavigational); ok {
			t.Errorf("passage %s is marked navigational — it carries a body and must rank on it", p.ID)
		}
	}
}
