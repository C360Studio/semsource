//go:build integration

package governance

import (
	"testing"

	"github.com/c360studio/semstreams/pkg/errs"
)

// fuseErrIsRetryable reports whether a Fuse error is the graph-index readiness
// gate rather than a real failure, and fails the test if it is anything else.
//
// Every fusion poll loop in this package waits for an async pipeline to catch
// up, and the readiness gate is strict catch-up — `ready := target > 0 &&
// indexed >= target` (semstreams graph/index_status.go). A query issued shortly
// after a write burst can therefore legitimately arrive while indexed is a
// handful of revisions behind target, and graph-index answers with a properly
// classified TRANSIENT carrying ErrorCodeIndexNotReady
// (processor/graph-index/query.go:183).
//
// Treating that as fatal turns a benign, self-resolving lag into a red build:
// the loops were written to poll for exactly this condition and then failed on
// it. The bootstrap flag is sticky, so once the gate opens it stays open and a
// retry always converges — which is why these failures appear only under load
// and vanish when a test is run alone.
//
// Detection is by classification, never by message text — the error carries the
// classification precisely so consumers do not have to string-match it. See
// semstreams#590 for the readiness-semantics discussion this sits on top of.
func fuseErrIsRetryable(t *testing.T, err error) bool {
	t.Helper()
	if err == nil {
		return false
	}
	if errs.IsTransient(err) {
		return true
	}
	t.Fatalf("Fuse: %v", err)
	return false
}
