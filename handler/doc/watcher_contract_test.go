package doc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/handler"
)

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

	if len(event.EntityStates) != 1 {
		t.Fatalf("EntityStates count = %d, want 1", len(event.EntityStates))
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
