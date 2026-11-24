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

// spec: entity-identity-safety / Config-file entities are keyed by their declaring file
func TestConfigHandler_Watch_MintsTheSeedIDs(t *testing.T) {
	// A watch republish must mint the seed walk's IDs, or one manifest
	// forks into two entities (#348). A relative root is the configured
	// shape in semsource.json.
	parent := t.TempDir()
	t.Chdir(parent)
	if err := os.Mkdir("repo", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	authority := testAuthority("acme")
	h := cfgfile.New(&cfgfile.Config{Authority: authority})
	cfg := &stubSourceConfig{sourceType: "config", path: "repo", watch: true}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, err := h.Watch(ctx, cfg)
	if err != nil || ch == nil {
		t.Fatalf("Watch: ch=%v err=%v", ch, err)
	}
	time.Sleep(100 * time.Millisecond) // same registration wait as the test above

	gomod := "module example.com/w\n\ngo 1.23\n\nrequire golang.org/x/text v0.14.0\n"
	if err := os.WriteFile(filepath.Join("repo", "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	var watched []string
	for len(watched) == 0 {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("Watch channel closed")
			}
			for _, s := range ev.EntityStates {
				watched = append(watched, s.ID)
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for a watch event with entity states")
		}
	}

	seeded, err := h.IngestEntityStates(context.Background(), cfg, authority)
	if err != nil {
		t.Fatalf("IngestEntityStates: %v", err)
	}
	want := map[string]bool{}
	for _, s := range seeded {
		want[s.ID] = true
	}
	for _, id := range watched {
		if !want[id] {
			t.Errorf("watch minted %s, which the seed walk does not", id)
		}
	}
}
