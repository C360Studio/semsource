//go:build integration

package doc_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semsource/handler"
	dochandler "github.com/c360studio/semsource/handler/doc"
)

// ---------------------------------------------------------------------------
// Watch — requires real fsnotify
// ---------------------------------------------------------------------------

func TestDocHandler_Watch_ReturnsChannel(t *testing.T) {
	dir := t.TempDir()

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir, watch: true}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := h.Watch(ctx, cfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	if ch == nil {
		t.Fatal("Watch() returned nil channel")
	}
}

func TestDocHandler_Watch_EmitsOnCreate(t *testing.T) {
	dir := t.TempDir()

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir, watch: true}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := h.Watch(ctx, cfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	time.Sleep(30 * time.Millisecond) // let watcher register

	// writeMD is defined in handler_test.go (unit tests) which shares the package.
	// Re-define inline to avoid cross-file dependency with build tags.
	target := filepath.Join(dir, "new.md")
	if err := os.WriteFile(target, []byte("# New\nContent."), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		if ev.Operation != handler.OperationCreate {
			t.Errorf("Operation = %q, want %q", ev.Operation, handler.OperationCreate)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for watch event")
	}
}
