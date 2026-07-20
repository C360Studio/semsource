package doc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// docTypeCounts tallies the states of a change event by source.DocType. One
// changed file now enriches to a parent document plus its passages, so "the
// file produced typed state" is a statement about the parent count, not the
// slice length.
func docTypeCounts(states []*handler.EntityState) (parents, passages int) {
	for _, state := range states {
		for _, tr := range state.Triples {
			if tr.Predicate != source.DocType {
				continue
			}
			switch tr.Object {
			case "document":
				parents++
			case "passage":
				passages++
			}
		}
	}
	return parents, passages
}

func TestEnrichEventCreateUsesOnlyTypedState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "readme.md")
	if err := os.WriteFile(path, []byte("# Read me\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	h := NewWithOrg("acme")
	event := h.enrichEvent(context.Background(), handler.ChangeEvent{
		Path:      path,
		Operation: handler.OperationCreate,
	}, root, h.org)

	parents, passages := docTypeCounts(event.EntityStates)
	if parents != 1 {
		t.Fatalf("parent EntityStates count = %d, want 1 (readme.md); %d states in total",
			parents, len(event.EntityStates))
	}
	if passages < 1 {
		t.Fatalf("passage EntityStates count = %d, want at least 1 for readme.md; %d states in total",
			passages, len(event.EntityStates))
	}
	if parents+passages != len(event.EntityStates) {
		t.Fatalf("classified %d of %d EntityStates; every state must be a document or a passage",
			parents+passages, len(event.EntityStates))
	}
	if len(event.Entities) != 0 {
		t.Fatalf("RawEntity count = %d, want 0", len(event.Entities))
	}
}

func TestEnrichEventWithoutOrgDoesNotFallBackToRawEntity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "readme.md")
	if err := os.WriteFile(path, []byte("# Read me\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	h := New()
	event := h.enrichEvent(context.Background(), handler.ChangeEvent{
		Path:      path,
		Operation: handler.OperationModify,
	}, root, h.org)

	if len(event.EntityStates) != 0 || len(event.Entities) != 0 {
		t.Fatalf("unscoped event must contain no typed or raw entities: %+v", event)
	}
}

func TestEnrichEventDeleteIsPathOnly(t *testing.T) {
	h := NewWithOrg("acme")
	event := h.enrichEvent(context.Background(), handler.ChangeEvent{
		Path:      "/removed/readme.md",
		Operation: handler.OperationDelete,
	}, "/removed", h.org)

	if event.Path != "/removed/readme.md" || event.Operation != handler.OperationDelete {
		t.Fatalf("delete signal changed: %+v", event)
	}
	if len(event.EntityStates) != 0 || len(event.Entities) != 0 {
		t.Fatalf("delete must be path-only: %+v", event)
	}
}
