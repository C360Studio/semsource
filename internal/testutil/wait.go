// Package testutil provides shared test helpers for synchronization and assertions.
package testutil

import (
	"testing"
	"time"
)

// WaitFor polls fn every 5ms until it returns true or timeout expires.
// Returns true if the condition was met, false on timeout.
func WaitFor(t *testing.T, timeout time.Duration, fn func() bool) bool {
	t.Helper()
	deadline := time.After(timeout)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			return false
		case <-ticker.C:
			if fn() {
				return true
			}
		}
	}
}

// ReceiveWithin reads from ch within timeout, returning the value and true.
// Returns the zero value and false on timeout.
func ReceiveWithin[T any](t *testing.T, ch <-chan T, timeout time.Duration) (T, bool) {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			var zero T
			return zero, false
		}
		return v, true
	case <-time.After(timeout):
		var zero T
		return zero, false
	}
}
