//go:build integration

package graphstatus_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/internal/graphstatus"
	semgraph "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
)

// TestReader_UnprovisionedBucketIsClassifiedNotReady is the beta.159
// acquisition-seam contract: a READER binds must-exist and never creates.
//
// Before beta.159 a reader could conjure the bucket on first use, so a
// deployment missing its owning component read happily from a bucket nothing
// ever wrote — an empty result that looked exactly like "no data". Now the read
// fails with a classified not-ready error that NAMES the owner.
//
// The assertions are on the error CLASS and on the owner being named, not on a
// code string: ADR-060 is explicit that adopters branch on the class, because
// the code string is not the stable surface.
func TestReader_UnprovisionedBucketIsClassifiedNotReady(t *testing.T) {
	// JetStream/KV is UP, but no SemStreams component has run, so GRAPH_STATUS
	// is genuinely absent rather than unreachable. Without WithKV the lookup
	// fails at the JetStream layer ("no responders") and never reaches the
	// catalog seam this test exists to exercise.
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reader := graphstatus.New(tc.Client)

	_, err := reader.Raw(ctx, graphstatus.KeyGraphIndex)
	if err == nil {
		t.Fatal("reading an unprovisioned GRAPH_STATUS returned no error; the reader seam created or bound a bucket nothing owns")
	}
	t.Logf("error: %v", err)

	// Class first: a missing owner is transient, not a permanent misconfiguration.
	// Classifying it invalid would tell a caller to give up on a deployment that
	// is merely still starting.
	if errs.IsInvalid(err) {
		t.Errorf("unprovisioned bucket classified as invalid (do-not-retry); want a retryable not-ready class: %v", err)
	}

	// Naming the owner is what makes the error actionable — it tells an operator
	// which component to deploy rather than just that a bucket is absent.
	msg := err.Error()
	if !strings.Contains(msg, "GRAPH_STATUS") {
		t.Errorf("not-ready error does not name the bucket: %v", err)
	}
	// The owner string is the actionable part: it tells an operator which
	// component to deploy, rather than only that a bucket is absent.
	if !strings.Contains(msg, "owner") && !strings.Contains(msg, "graph-index") {
		t.Errorf("not-ready error does not name the bucket's owner, so it cannot tell an operator what to deploy: %v", err)
	}
}

// TestReader_OffCatalogKeyIsInvalidNotNotReady guards the distinction the
// previous test depends on: a bucket outside the framework catalog is a
// PERMANENT misconfiguration, not a transient missing owner. Conflating the two
// would make an operator typo look like something worth retrying forever.
func TestReader_OffCatalogKeyIsInvalidNotNotReady(t *testing.T) {
	if _, ok := semgraph.SpecFor("GRAPH_STATUS_TYPO"); ok {
		t.Fatal("GRAPH_STATUS_TYPO unexpectedly resolves in the framework catalog")
	}

	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := semgraph.OpenCatalogBucket(ctx, tc.Client, "GRAPH_STATUS_TYPO")
	if err == nil {
		t.Fatal("opening an off-catalog bucket succeeded; a typo must not silently create a stray unguarded bucket")
	}
	if !errs.IsInvalid(err) {
		t.Errorf("off-catalog bucket is not classified invalid, so a typo would be retried forever: %v", err)
	}
	t.Logf("off-catalog error: %v", err)
}
