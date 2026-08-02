// Package seedsup supervises a source component's asynchronous initial seed.
//
// It exists because a lifecycle `Start()` is a hook, not a place to do minutes
// of work. semstreams' component-start barrier waits for every component's
// Start to return before the composition root proceeds to HTTP setup, so a
// component that seeds synchronously prevents the status and metrics listeners
// from binding at all — for exactly the window in which an operator needs them
// (semstreams#867).
//
// A Supervisor runs the seed in its own goroutine, records a failure where it
// can still be seen (the seed can no longer fail Start), and lets Stop cancel
// and WAIT for the seed before the caller tears down its publisher.
package seedsup

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// defaultStopTimeout bounds the shutdown wait when the caller passes none, so a
// wedged seed cannot hang shutdown forever.
const defaultStopTimeout = 30 * time.Second

// Error describes a seed failure in the shape source-manifest expects on the
// wire. The two are coupled by JSON, not by a Go type.
type Error struct {
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// Supervisor owns one component's seed goroutine. The zero value is ready.
type Supervisor struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	lastErr  *Error
	nowFn    func() time.Time // injectable for tests
	stopOnce sync.Once
}

// Start runs seed in a goroutine and returns immediately.
//
// A seed that fails is NOT returned to the caller — by the time it runs, Start
// has returned — so the failure is recorded for the status report and logged at
// WARN. A seed cancelled by Stop is a clean shutdown, not a failure.
func (s *Supervisor) Start(ctx context.Context, logger *slog.Logger, seed func(context.Context) error) {
	seedCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	s.mu.Lock()
	s.cancel = cancel
	s.done = done
	s.mu.Unlock()

	go func() {
		defer close(done)
		if err := seed(seedCtx); err != nil {
			if seedCtx.Err() != nil {
				return // cancelled by Stop
			}
			s.recordError(err)
			if logger != nil {
				logger.Warn("initial seed failed — this source will not become ready",
					"error", err)
			}
		}
	}()
}

// Stop cancels the seed and waits for it, bounded by timeout.
//
// Callers MUST call this before stopping their publisher: stopping a publisher
// closes its buffer, so a seed still running would publish into a closed one.
// Callers must also not hold a lock the seed itself takes — this waits, and
// waiting under a shared lock deadlocks against the seed's own critical section.
func (s *Supervisor) Stop(timeout time.Duration, logger *slog.Logger) {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel, s.done = nil, nil
	s.mu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	if done == nil {
		return
	}
	if timeout <= 0 {
		timeout = defaultStopTimeout
	}
	select {
	case <-done:
	case <-time.After(timeout):
		if logger != nil {
			logger.Warn("timed out waiting for the initial seed to stop", "timeout", timeout)
		}
	}
}

// Running reports whether a seed is currently in flight.
func (s *Supervisor) Running() bool {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return false
	}
	select {
	case <-done:
		return false
	default:
		return true
	}
}

// LastError returns the most recent seed failure, or nil. It is the only
// channel a seed failure has once Start has returned.
func (s *Supervisor) LastError() *Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *Supervisor) recordError(err error) {
	now := time.Now
	s.mu.Lock()
	if s.nowFn != nil {
		now = s.nowFn
	}
	// "Unreachable" is the honest code: a seed that failed did not reach its
	// source content.
	s.lastErr = &Error{Code: "SOURCE_UNREACHABLE", Message: err.Error(), Timestamp: now()}
	s.mu.Unlock()
}
