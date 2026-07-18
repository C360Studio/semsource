package urlsource

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/c360studio/semsource/handler"
)

func TestHandleChangeEventMissingTypedStateRecordsContractError(t *testing.T) {
	c := &Component{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	c.handleChangeEvent(context.Background(), handler.ChangeEvent{
		Path:      "https://example.com",
		Operation: handler.OperationCreate,
	})
	if got := c.ingestErrors.Load(); got != 1 {
		t.Fatalf("ingestErrors = %d, want 1", got)
	}
}

func TestHandleChangeEventDeleteNeedsNoTypedState(t *testing.T) {
	c := &Component{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	c.handleChangeEvent(context.Background(), handler.ChangeEvent{
		Path:      "https://example.com",
		Operation: handler.OperationDelete,
	})
	if got := c.ingestErrors.Load(); got != 0 {
		t.Fatalf("ingestErrors = %d, want 0", got)
	}
}

func TestHandleChangeEventNilTypedStateRecordsErrorWithoutPublishing(t *testing.T) {
	c := &Component{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	c.handleChangeEvent(context.Background(), handler.ChangeEvent{
		Path:         "https://example.com",
		Operation:    handler.OperationModify,
		EntityStates: []*handler.EntityState{nil},
	})
	if got := c.ingestErrors.Load(); got != 1 {
		t.Fatalf("ingestErrors = %d, want 1", got)
	}
}
