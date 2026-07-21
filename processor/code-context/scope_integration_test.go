//go:build integration

package codecontext

import (
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/graph/readiness"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/pkg/fusion/fusionnats"
)

// TestIntegration_DomainScopedRetrieval_OnTheWire proves ask #16 end to end over
// REAL NATS: a code-context / doc-context component driving the REAL fusion
// engine and REAL fusionnats client emits the per-lens default Scope onto the
// actual graph.query.semantic request — and honors a caller-provided scope
// verbatim.
//
// The scope FILTER itself (candidate prefix-matching via graph.MatchesAnyIDPrefix
// in graph-embedding) is framework code, validated in semstreams and in the live
// httpx dogfood (docs/upstream/semstreams-asks.md #16). This test owns the
// semsource seam — that the correct scope is SELECTED per lens and reaches the
// wire — deterministically, without standing up the embedding subsystem (the
// governance harness deliberately keeps the NL path out; validated separately).
func TestIntegration_DomainScopedRetrieval_OnTheWire(t *testing.T) {
	ctx := context.Background()
	// WithKV: readiness moved from a request subject to GRAPH_STATUS KV state
	// (ADR-083), so this test needs JetStream where it previously did not.
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	// Readiness is KV STATE, not a request subject (ADR-083, beta.157): the client
	// watches GRAPH_STATUS/graph-index rather than asking graph.index.query.status,
	// so stubbing that subject is now dead code. Seed the bucket instead.
	//
	// Two fields are load-bearing beyond Ready. State is allow-listed, and
	// BootstrapComplete fails CLOSED (ADR-084) — omit either and the gate defers,
	// resolve never fires, and this test reads an empty scope for a reason that has
	// nothing to do with scope selection.
	var mu sync.Mutex
	var lastScope []string
	statusBucket, err := readiness.EnsureBucket(ctx, tc.Client)
	if err != nil {
		t.Fatalf("ensure %s: %v", readiness.BucketGraphStatus, err)
	}
	publisher := readiness.NewPublisher(statusBucket, readiness.KeyGraphIndex)
	publishReady := func() {
		if err := publisher.Publish(ctx, graph.IndexStatusResponse{
			Ready: true, State: string(fusion.StateReady), BootstrapComplete: true,
		}); err != nil {
			t.Fatalf("publish readiness: %v", err)
		}
	}
	publishReady()
	// The watcher requires FRESHNESS, not merely presence: a value that stops
	// arriving reads as unknown and fails closed, exactly so a dead producer cannot
	// serve its final Ready=true forever. Keep the heartbeat alive for the run.
	heartbeat, stopHeartbeat := context.WithCancel(ctx)
	t.Cleanup(stopHeartbeat)
	go func() {
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-heartbeat.Done():
				return
			case <-tick.C:
				_ = publisher.Publish(heartbeat, graph.IndexStatusResponse{
					Ready: true, State: string(fusion.StateReady), BootstrapComplete: true,
				})
			}
		}
	}()
	semanticSub, err := tc.Client.SubscribeForRequests(ctx, "graph.query.semantic",
		func(_ context.Context, body []byte) ([]byte, error) {
			var req struct {
				Scope []string `json:"scope"`
			}
			_ = json.Unmarshal(body, &req)
			mu.Lock()
			lastScope = req.Scope
			mu.Unlock()
			return []byte(`{"results":[]}`), nil
		})
	if err != nil {
		t.Fatalf("subscribe semantic: %v", err)
	}
	t.Cleanup(func() { _ = semanticSub.Unsubscribe() })

	// A running component over the real fusionnats client + real engine. The
	// engine is injected and running set, skipping Start's ObjectStore attach —
	// bodies never hydrate here (the stub returns no results).
	newComp := func(lens string) *Component {
		g := fusionnats.New(tc.Client, 0)
		return &Component{
			name:        "code-context",
			lensKind:    lens,
			subjectRoot: lens + ".v1.",
			org:         "acme",
			graph:       g,
			engine:      fusion.NewEngine(g, fusion.NewBodyResolver(fusion.MapStoreResolver{})),
			logger:      slog.Default(),
			running:     true,
			startTime:   time.Now(),
		}
	}

	fetchScope := func(t *testing.T, c *Component, body string) []string {
		t.Helper()
		mu.Lock()
		lastScope = nil
		mu.Unlock()
		reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if _, err := c.serve(reqCtx, "context", []byte(body)); err != nil {
			t.Fatalf("serve: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		return lastScope
	}

	t.Run("docs lens defaults scope to the web and config domains on the wire", func(t *testing.T) {
		got := fetchScope(t, newComp("docs"), `{"query":"how does retry work"}`)
		// config joins web (search-ranking-and-reach D4): dependency/manifest
		// questions answer through doc_context too, matching docScopeDomains.
		want := []string{"acme.semsource.web", "acme.semsource.config"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("semantic scope = %v, want %v", got, want)
		}
	})

	t.Run("code lens defaults scope to the code-language domains on the wire", func(t *testing.T) {
		got := fetchScope(t, newComp("code"), `{"query":"where is the retry handler"}`)
		want := []string{
			"acme.semsource.golang",
			"acme.semsource.python",
			"acme.semsource.typescript",
			"acme.semsource.javascript",
			"acme.semsource.java",
			"acme.semsource.svelte",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("semantic scope = %v, want %v", got, want)
		}
	})

	t.Run("a caller-provided scope is passed through verbatim", func(t *testing.T) {
		got := fetchScope(t, newComp("docs"),
			`{"query":"how does retry work","scope":["other.semsource.golang"]}`)
		want := []string{"other.semsource.golang"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("semantic scope = %v, want caller scope %v", got, want)
		}
	})
}
