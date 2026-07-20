//go:build integration

package governance

import (
	"errors"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/pkg/errs"
)

// readReadyTimeout bounds how long a graph read may keep hitting the readiness
// gate before the test gives up. Generous because CI runners are cold and the
// gate is strict catch-up, not because any single read is slow.
const readReadyTimeout = 20 * time.Second

// isIndexNotReady reports whether err is the classified readiness transient a
// graph read emits while the index is catching up to ENTITY_STATES.
//
// Matched on the STABLE CODE, not errs.IsTransient. A real connection timeout is
// also classified transient, and retrying one until the deadline would convert a
// genuine outage into a mute timeout rather than a reported failure. This mirrors
// pkg/fusion's own isIndexNotReady (semstreams#593), which made the same
// correction for the same reason.
func isIndexNotReady(err error) bool {
	var ce *errs.ClassifiedError
	return errors.As(err, &ce) && ce.Code == graph.ErrorCodeIndexNotReady
}

// retryUntilReady runs a graph read until it stops hitting the readiness gate,
// failing the test on any other error.
//
// graph-index gates reads on strict catch-up (`indexed >= target`), so a read
// issued shortly after a write burst legitimately arrives while the index is a
// few revisions behind and gets a transient. semstreams#592 settled that
// retrying that transient IS the contract for exact consumers — serving a
// bounded-stale answer instead would risk reporting a just-written entity as an
// authoritative miss. Readiness is sticky, so a retry converges quickly.
//
// This exists because widening a deadline does not fix an error-shaped failure:
// a loop that returns on the first error never reaches its deadline at all.
func retryUntilReady[T any](t *testing.T, label string, read func() (T, error)) T {
	t.Helper()
	deadline := time.Now().Add(readReadyTimeout)
	for {
		v, err := read()
		if err == nil {
			return v
		}
		if !isIndexNotReady(err) {
			t.Fatalf("%s: %v", label, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: index still catching up after %s", label, readReadyTimeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// fuseErrIsRetryable reports whether a Fuse error is the readiness transient
// rather than a real failure, and fails the test if it is anything else. Used by
// poll loops that already have their own deadline and result condition.
//
// As of beta.156 Fuse degrades to an empty-honest Ready=false envelope on this
// transient instead of propagating it (semstreams#593), so these loops should
// rarely reach here — the check stays because it is the honest classification
// either way, and a degraded envelope simply means "keep polling".
func fuseErrIsRetryable(t *testing.T, err error) bool {
	t.Helper()
	if err == nil {
		return false
	}
	if isIndexNotReady(err) {
		return true
	}
	t.Fatalf("Fuse: %v", err)
	return false
}
