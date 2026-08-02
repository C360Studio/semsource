package seedsup

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestStartReturnsBeforeSeedFinishes is the property the whole change exists
// for: the caller's lifecycle hook must not wait on the seed.
func TestStartReturnsBeforeSeedFinishes(t *testing.T) {
	var s Supervisor
	release := make(chan struct{})
	entered := make(chan struct{})

	s.Start(context.Background(), nil, func(context.Context) error {
		close(entered)
		<-release
		return nil
	})
	<-entered

	if !s.Running() {
		t.Fatal("Running() = false while the seed is blocked; Start did not supervise it")
	}
	close(release)
	waitFor(t, func() bool { return !s.Running() }, "seed to finish")
}

// TestSeedFailureIsRecorded — the seed can no longer fail the caller's Start, so
// an unrecorded failure would be invisible to every consumer.
func TestSeedFailureIsRecorded(t *testing.T) {
	var s Supervisor
	s.Start(context.Background(), nil, func(context.Context) error {
		return errors.New("boom")
	})
	waitFor(t, func() bool { return s.LastError() != nil }, "failure to be recorded")

	got := s.LastError()
	if got.Code == "" || got.Message != "boom" || got.Timestamp.IsZero() {
		t.Errorf("LastError() = %+v, want code, message %q, and a timestamp", got, "boom")
	}
}

// TestCancelledSeedIsNotAFailure — Stop cancelling a seed is a clean shutdown.
// Recording it would make every restart look like an error.
func TestCancelledSeedIsNotAFailure(t *testing.T) {
	var s Supervisor
	started := make(chan struct{})
	s.Start(context.Background(), nil, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	<-started

	s.Stop(2*time.Second, nil)

	if err := s.LastError(); err != nil {
		t.Errorf("LastError() = %+v after cancellation, want nil — a cancelled seed is not a failure", err)
	}
}

// TestStopWaitsForTheSeed is the shutdown-ordering guarantee: the caller stops
// its publisher after this returns, and a seed still running would publish into
// a closed buffer.
func TestStopWaitsForTheSeed(t *testing.T) {
	var s Supervisor
	var mu sync.Mutex
	finished := false

	started := make(chan struct{})
	s.Start(context.Background(), nil, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond) // unwinding after cancellation
		mu.Lock()
		finished = true
		mu.Unlock()
		return ctx.Err()
	})
	<-started

	s.Stop(2*time.Second, nil)

	mu.Lock()
	defer mu.Unlock()
	if !finished {
		t.Error("Stop() returned before the seed finished unwinding")
	}
}

// TestStopIsSafeWhenNoSeedRan guards the ordinary path where a component is
// stopped before it ever started one.
func TestStopIsSafeWhenNoSeedRan(t *testing.T) {
	var s Supervisor
	s.Stop(time.Second, nil) // must not panic or block
	if s.Running() {
		t.Error("Running() = true with no seed started")
	}
}

// TestStopTimesOutRatherThanHanging — a wedged seed must not hang shutdown.
func TestStopTimesOutRatherThanHanging(t *testing.T) {
	var s Supervisor
	block := make(chan struct{})
	defer close(block)

	started := make(chan struct{})
	s.Start(context.Background(), nil, func(context.Context) error {
		close(started)
		<-block // ignores cancellation entirely
		return nil
	})
	<-started

	done := make(chan struct{})
	go func() { defer close(done); s.Stop(100*time.Millisecond, nil) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() hung on a seed that ignores cancellation")
	}
}
