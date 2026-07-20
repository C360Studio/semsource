//go:build integration

package governance

import (
	"errors"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/pkg/errs"
)

// fuseErrIsRetryable reports whether a Fuse error is the graph-index readiness
// transient rather than a real failure, and fails the test if it is anything
// else.
//
// Every fusion poll loop in this package waits for an async pipeline to catch
// up. graph-index gates queries on strict catch-up (`indexed >= target`), so a
// query issued shortly after a write burst can legitimately arrive while the
// index is a few revisions behind and get a classified transient carrying
// graph.ErrorCodeIndexNotReady. Treating that as fatal turns a benign,
// self-resolving lag into a red build: these loops were written to poll for
// exactly that condition and then failed on it. Readiness is sticky, so a retry
// always converges — which is why such failures appear only under load and
// vanish when a test runs alone.
//
// Matched on the STABLE CODE, not errs.IsTransient. A real connection timeout is
// also classified transient, and retrying one until the deadline would convert a
// genuine outage into a mute timeout rather than a reported failure. This mirrors
// pkg/fusion's own isIndexNotReady (semstreams#593), which made the same
// correction for the same reason.
//
// As of beta.156 Fuse degrades to an empty-honest Ready=false envelope on this
// transient instead of propagating it, so the loops should rarely reach here —
// the check stays because it is the honest classification either way, and a
// degraded envelope simply means "keep polling".
func fuseErrIsRetryable(t *testing.T, err error) bool {
	t.Helper()
	if err == nil {
		return false
	}
	var ce *errs.ClassifiedError
	if errors.As(err, &ce) && ce.Code == graph.ErrorCodeIndexNotReady {
		return true
	}
	t.Fatalf("Fuse: %v", err)
	return false
}
