package ast

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

type blockedWatchParser struct {
	entered   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
}

func (p *blockedWatchParser) ParseFile(ctx context.Context, _ string) (*ParseResult, error) {
	close(p.entered)
	<-ctx.Done()
	close(p.cancelled)
	<-p.release
	return &ParseResult{Hash: "after-cancellation"}, nil
}

func TestWatcherStopRetainsOutputUntilParserJoins(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.go")
	if err := os.WriteFile(path, []byte("package example"), 0600); err != nil {
		t.Fatal(err)
	}
	parser := &blockedWatchParser{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	watcher, err := NewWatcherWithParser(WatcherConfig{RepoRoot: root, DebounceDelay: time.Millisecond}, parser)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		select {
		case <-parser.release:
		default:
			close(parser.release)
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		if err := watcher.Stop(cleanupCtx); err != nil {
			t.Error("cleanup watcher:", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = watcher.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err = watcher.Start(ctx); err == nil {
		t.Fatal("duplicate Start accepted")
	}
	watcher.handleFSEvent(fsnotify.Event{Name: path, Op: fsnotify.Write})
	select {
	case <-parser.entered:
	case <-ctx.Done():
		t.Fatal("parser did not start")
	}
	stopCtx, stopCancel := context.WithCancel(context.Background())
	stopCancel()
	if err = watcher.Stop(stopCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop=%v, want explicit incomplete join", err)
	}
	select {
	case <-parser.cancelled:
	case <-ctx.Done():
		t.Fatal("owned parser context not cancelled")
	}
	select {
	case _, ok := <-watcher.Events():
		if !ok {
			t.Fatal("output closed while parser can still send")
		}
	default:
	}
	close(parser.release)
	if err = watcher.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	drained := 0
	for range watcher.Events() {
		drained++
	}
	t.Logf("drained %d events buffered before joined shutdown", drained)
	if err = watcher.Stop(ctx); err != nil {
		t.Fatal("repeat Stop:", err)
	}
	if err = watcher.Start(ctx); err == nil {
		t.Fatal("stopped watcher restarted")
	}
}

func TestWatcherFailedStartAndUnstartedStopReleaseOwner(t *testing.T) {
	for _, failedStart := range []bool{false, true} {
		watcher, err := NewWatcherWithParser(WatcherConfig{RepoRoot: filepath.Join(t.TempDir(), "missing")}, &blockedWatchParser{})
		if err != nil {
			t.Fatal(err)
		}
		if failedStart && watcher.Start(context.Background()) == nil {
			t.Fatal("missing directory start succeeded")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err = watcher.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		cancel()
		if _, ok := <-watcher.Events(); ok {
			t.Fatal("unstarted/failed-start output left open")
		}
		if watcher.config.DebounceDelay <= 0 {
			t.Fatal("default debounce was not persisted")
		}
	}
}

func TestWatcherCancelledStartClosesOwnedResources(t *testing.T) {
	watcher, err := NewWatcherWithParser(WatcherConfig{RepoRoot: t.TempDir()}, &blockedWatchParser{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = watcher.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start=%v, want cancelled traversal", err)
	}
	if err = watcher.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-watcher.Events(); ok {
		t.Fatal("cancelled start output left open")
	}
}
