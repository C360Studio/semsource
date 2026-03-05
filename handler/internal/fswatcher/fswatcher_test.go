package fswatcher_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/handler/internal/fswatcher"
)

const (
	// testTimeout is the maximum time to wait for a watch event.
	testTimeout = 3 * time.Second

	// debounceDelay must be shorter than testTimeout to allow debounce to flush.
	debounceDelay = 50 * time.Millisecond
)

// makeWatcher creates an FSWatcher over dir with the given extensions.
// t.Cleanup stops it automatically.
func makeWatcher(t *testing.T, dir string, exts []string) *fswatcher.FSWatcher {
	t.Helper()
	cfg := fswatcher.WatchConfig{
		Enabled:        true,
		DebounceDelay:  debounceDelay.String(),
		FileExtensions: exts,
		ExcludeDirs:    []string{".git"},
	}
	w, err := fswatcher.New(cfg, dir)
	if err != nil {
		t.Fatalf("fswatcher.New: %v", err)
	}
	t.Cleanup(func() { _ = w.Stop() })
	return w
}

// waitEvent drains the Events channel until an event arrives or timeout.
func waitEvent(t *testing.T, w *fswatcher.FSWatcher) (handler.ChangeEvent, bool) {
	t.Helper()
	select {
	case ev, ok := <-w.Events():
		if !ok {
			return handler.ChangeEvent{}, false
		}
		return ev, true
	case <-time.After(testTimeout):
		return handler.ChangeEvent{}, false
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestFSWatcher_Create(t *testing.T) {
	dir := t.TempDir()
	w := makeWatcher(t, dir, []string{".md"})

	ctx := t.Context()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give the watcher a moment to register inotify watches.
	time.Sleep(20 * time.Millisecond)

	target := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(target, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ev, ok := waitEvent(t, w)
	if !ok {
		t.Fatal("timed out waiting for Create event")
	}
	if ev.Operation != handler.OperationCreate {
		t.Errorf("Operation = %q, want %q", ev.Operation, handler.OperationCreate)
	}
	// Entities are populated by the handler layer (DocHandler/ConfigHandler),
	// not by FSWatcher itself — FSWatcher emits the path/operation signal only.
	if ev.Path == "" {
		t.Error("expected Path to be set in ChangeEvent")
	}
}

// ---------------------------------------------------------------------------
// Modify
// ---------------------------------------------------------------------------

func TestFSWatcher_Modify(t *testing.T) {
	dir := t.TempDir()

	// Pre-create the file so the watcher sees it as a modify.
	target := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(target, []byte("v1"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := makeWatcher(t, dir, []string{".md"})
	ctx := t.Context()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Seed the hash so the watcher knows about the initial content.
	w.SetHash(target, fswatcher.ContentHash([]byte("v1")))

	time.Sleep(20 * time.Millisecond)

	if err := os.WriteFile(target, []byte("v2 changed content"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ev, ok := waitEvent(t, w)
	if !ok {
		t.Fatal("timed out waiting for Modify event")
	}
	if ev.Operation != handler.OperationModify {
		t.Errorf("Operation = %q, want %q", ev.Operation, handler.OperationModify)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestFSWatcher_Delete(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "gone.md")
	if err := os.WriteFile(target, []byte("bye"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := makeWatcher(t, dir, []string{".md"})
	ctx := t.Context()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	if err := os.Remove(target); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	ev, ok := waitEvent(t, w)
	if !ok {
		t.Fatal("timed out waiting for Delete event")
	}
	if ev.Operation != handler.OperationDelete {
		t.Errorf("Operation = %q, want %q", ev.Operation, handler.OperationDelete)
	}
}

// ---------------------------------------------------------------------------
// Extension filtering
// ---------------------------------------------------------------------------

func TestFSWatcher_ExtensionFiltering(t *testing.T) {
	dir := t.TempDir()
	w := makeWatcher(t, dir, []string{".md"})

	ctx := t.Context()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	// Write a non-.md file — should NOT produce an event.
	if err := os.WriteFile(filepath.Join(dir, "ignore.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Also write a .md file — should produce an event.
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# hi"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ev, ok := waitEvent(t, w)
	if !ok {
		t.Fatal("timed out waiting for event from .md file")
	}
	if ev.Operation != handler.OperationCreate {
		t.Errorf("unexpected Operation %q for .md file", ev.Operation)
	}

	// Ensure no further events arrive (the .go file should have been filtered).
	select {
	case extra, ok := <-w.Events():
		if ok {
			t.Errorf("unexpected extra event: %+v", extra)
		}
	case <-time.After(debounceDelay * 4):
		// Good — nothing came through.
	}
}

// ---------------------------------------------------------------------------
// Debounce: rapid writes produce a single event
// ---------------------------------------------------------------------------

func TestFSWatcher_Debounce(t *testing.T) {
	dir := t.TempDir()
	// Use a slightly longer debounce so rapid writes collapse reliably.
	cfg := fswatcher.WatchConfig{
		Enabled:        true,
		DebounceDelay:  "100ms",
		FileExtensions: []string{".md"},
		ExcludeDirs:    []string{},
	}
	w, err := fswatcher.New(cfg, dir)
	if err != nil {
		t.Fatalf("fswatcher.New: %v", err)
	}
	t.Cleanup(func() { _ = w.Stop() })

	ctx := t.Context()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	target := filepath.Join(dir, "burst.md")

	// Rapid writes within the debounce window.
	for i := range 5 {
		content := []byte("write " + string(rune('0'+i)))
		if err := os.WriteFile(target, content, 0644); err != nil {
			t.Fatalf("WriteFile %d: %v", i, err)
		}
	}

	// Wait for the debounce to flush.
	ev, ok := waitEvent(t, w)
	if !ok {
		t.Fatal("timed out waiting for debounced event")
	}
	_ = ev

	// Should not receive more than one event for the burst.
	count := 1
	deadline := time.After(200 * time.Millisecond)
loop:
	for {
		select {
		case _, ok := <-w.Events():
			if !ok {
				break loop
			}
			count++
		case <-deadline:
			break loop
		}
	}

	// Debounce collapses rapid same-file writes; allow at most 2 (edge timing).
	if count > 2 {
		t.Errorf("debounce failed: got %d events for 5 rapid writes, want ≤2", count)
	}
}

// ---------------------------------------------------------------------------
// ContentHash is exported for test seeding
// ---------------------------------------------------------------------------

func TestContentHash_Deterministic(t *testing.T) {
	content := []byte("hello world")
	h1 := fswatcher.ContentHash(content)
	h2 := fswatcher.ContentHash(content)
	if h1 != h2 {
		t.Errorf("ContentHash not deterministic: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Error("ContentHash returned empty string")
	}
}

func TestContentHash_DifferentContent(t *testing.T) {
	h1 := fswatcher.ContentHash([]byte("v1"))
	h2 := fswatcher.ContentHash([]byte("v2"))
	if h1 == h2 {
		t.Error("ContentHash collision for different content")
	}
}
