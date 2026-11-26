package fswatcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounceFixture drives the pending map with a fake clock, so readiness is
// tested without sleeping.
type debounceFixture struct {
	t    *testing.T
	w    *FSWatcher
	path string
	now  time.Time
}

func newDebounceFixture(t *testing.T, delay string) *debounceFixture {
	t.Helper()
	dir := t.TempDir()
	w, err := New(WatchConfig{Enabled: true, DebounceDelay: delay, FileExtensions: []string{".md"}}, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.watcher.Close() })
	f := &debounceFixture{t: t, w: w, path: filepath.Join(dir, "a.md"), now: time.Unix(1_000_000, 0)}
	w.now = func() time.Time { return f.now }
	return f
}

// write changes the file's content and records the fsnotify event, as the
// event loop would.
func (f *debounceFixture) write(content string) {
	f.t.Helper()
	if err := os.WriteFile(f.path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.w.handleFSEvent(fsnotify.Event{Name: f.path, Op: fsnotify.Write})
}

// flush runs one flush tick and returns how many events it emitted.
func (f *debounceFixture) flush() int {
	f.t.Helper()
	f.w.flushPending(context.Background())
	n := 0
	for {
		select {
		case <-f.w.events:
			n++
		default:
			return n
		}
	}
}

// spec: openspec/specs/typed-source-change-events/spec.md#Requirement: A watched file emits one change after it goes quiet
func TestDebounce_WaitsUntilThePathIsQuiet(t *testing.T) {
	f := newDebounceFixture(t, "300ms")
	got := 0
	for i := range 5 { // five writes 100 ms apart: never quiet for 300 ms
		f.write(string(rune('a' + i)))
		got += f.flush()
		f.now = f.now.Add(100 * time.Millisecond)
	}
	if got != 0 {
		t.Fatalf("emitted %d events while the file was still being written", got)
	}
	f.now = f.now.Add(200 * time.Millisecond) // 300 ms after the last write
	if got := f.flush(); got != 1 {
		t.Errorf("after the path went quiet the watcher emitted %d events, want 1", got)
	}
}

// spec: openspec/specs/typed-source-change-events/spec.md#Requirement: A watched file emits one change after it goes quiet
func TestDebounce_ContinuousWritesFlushAtTheCap(t *testing.T) {
	f := newDebounceFixture(t, "100ms")
	got := 0
	for i := range 20 { // a write every 50 ms for one second
		f.write(string(rune('a' + i)))
		got += f.flush()
		f.now = f.now.Add(50 * time.Millisecond)
	}
	if got == 0 {
		t.Error("a file written continuously for 10x the delay was never flushed")
	}
	if got > 2 {
		t.Errorf("continuous writes emitted %d events in 1 s with a 500 ms cap, want at most 2", got)
	}
}
