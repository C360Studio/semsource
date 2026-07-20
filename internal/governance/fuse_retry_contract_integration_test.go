//go:build integration

package governance

import (
	"errors"
	"fmt"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/pkg/errs"
)

// The readiness-retry helpers are the reason this package's integration tests
// survive a cold runner, and their correctness rests entirely on classifying one
// specific error apart from every other transient. That distinction cannot be
// exercised by the suite itself — the gate only fires under timing this test
// cannot force — so it is pinned directly here.

func TestIsIndexNotReady_ClassifiesTheReadinessGateOnly(t *testing.T) {
	readiness := errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady,
		errors.New("index not ready: still catching up to ENTITY_STATES"))

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"the readiness gate", readiness, true},
		{"wrapped readiness gate", fmt.Errorf("query relationships failed: %w", readiness), true},
		{"nil", nil, false},
		{"plain error", errors.New("boom"), false},
		{
			// The distinction that matters. A connection timeout is ALSO
			// classified transient, so a helper keyed on errs.IsTransient would
			// retry a genuine outage until the deadline and report a mute
			// timeout instead of the real failure (semstreams#593).
			name: "a different transient must NOT be retried",
			err:  errs.ClassifiedCode(errs.ErrorTransient, "", errors.New("connection timeout")),
			want: false,
		},
		{
			name: "a transient with a different code must NOT be retried",
			err:  errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeRevisionMismatch, errors.New("revision mismatch")),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isIndexNotReady(tt.err); got != tt.want {
				t.Errorf("isIndexNotReady(%v) = %v, want %v", tt.err, got, tt.want)
			}
			// Guard against a regression back to the broad check: for the two
			// non-readiness transients, errs.IsTransient is TRUE while ours is
			// false — that gap is the whole point of the helper.
			if !tt.want && tt.err != nil && errs.IsTransient(tt.err) && isIndexNotReady(tt.err) {
				t.Error("classified on transience rather than on the readiness code")
			}
		})
	}
}

func TestRetryUntilReady_RetriesThenSucceeds(t *testing.T) {
	readiness := errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady,
		errors.New("index not ready: still catching up to ENTITY_STATES"))

	calls := 0
	got := retryUntilReady(t, "fake read", func() (string, error) {
		calls++
		if calls < 3 {
			return "", readiness
		}
		return "ready", nil
	})

	if got != "ready" {
		t.Errorf("value = %q, want %q", got, "ready")
	}
	if calls != 3 {
		t.Errorf("read called %d times, want 3 — the helper must retry the gate, not fail on it", calls)
	}
}
