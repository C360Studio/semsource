//go:build integration

package cfgfile_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	cfgfile "github.com/c360studio/semsource/handler/cfgfile"
)

// ---------------------------------------------------------------------------
// Watch — requires real fsnotify
// ---------------------------------------------------------------------------

func TestConfigHandler_Watch_EmitsOnFileCreate(t *testing.T) {
	dir := t.TempDir()
	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir, watch: true}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := h.Watch(ctx, cfg)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if ch == nil {
		t.Fatal("Watch returned nil channel with watch enabled")
	}

	// Give watcher time to register directory
	time.Sleep(100 * time.Millisecond)

	// Write a go.mod file
	gomod := "module github.com/example/watched\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("Watch channel closed unexpectedly")
		}
		if ev.Path == "" {
			t.Error("ChangeEvent.Path must not be empty")
		}
		if ev.Operation == "" {
			t.Error("ChangeEvent.Operation must not be empty")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for watch event")
	}
}

func TestConfigHandler_Watch_ChannelClosedOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	h := cfgfile.New(nil)
	cfg := &stubSourceConfig{sourceType: "config", path: dir, watch: true}

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := h.Watch(ctx, cfg)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if ch == nil {
		t.Fatal("Watch returned nil channel with watch enabled")
	}

	cancel()

	// Channel should close within a reasonable time
	timeout := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // channel closed as expected
			}
		case <-timeout:
			t.Fatal("Watch channel not closed after context cancellation")
		}
	}
}
