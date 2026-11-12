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

	"github.com/c360studio/semengine/pkg/fusion"
	"github.com/c360studio/semengine/pkg/fusion/fusionnats"
	"github.com/c360studio/semsource/internal/natstest"
)

// TestIntegration_DomainScopedRetrieval_OnTheWire proves ask #16 end to end over
// REAL NATS: a code-context / doc-context component driving the REAL fusion
// engine and REAL fusionnats client emits the per-lens default Scope onto the
// actual graph.query.semantic request — and honors a caller-provided scope
// verbatim.
//
// The scope FILTER itself (candidate prefix-matching via graph.MatchesAnyIDPrefix
// in graph-embedding) is framework code, validated in semstreams and in the live
// httpx dogfood (semstreams#463, adopted in beta.141). This test owns the
// semsource seam — that the correct scope is SELECTED per lens and reaches the
// wire — deterministically, without standing up the embedding subsystem (the
// governance harness deliberately keeps the NL path out; validated separately).
// spec: openspec/specs/fusion-gateway/spec.md#Requirement: Domain-scoped NL retrieval per lens
func TestIntegration_DomainScopedRetrieval_OnTheWire(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t)

	var mu sync.Mutex
	var lastScope []string
	semanticSub, err := tc.Client.SubscribeForRequests(ctx, "graph.query.semantic",
		func(_ context.Context, body []byte) ([]byte, error) {
			var req struct {
				Scope []string `json:"scope"`
			}
			_ = json.Unmarshal(body, &req)
			mu.Lock()
			lastScope = req.Scope
			mu.Unlock()
			return []byte(`{"producer":"graph-embedding","data":{"results":[]}}`), nil
		})
	if err != nil {
		t.Fatalf("subscribe semantic: %v", err)
	}
	t.Cleanup(func() { _ = semanticSub.Unsubscribe() })

	// A running component over the real fusionnats client + real engine. The
	// engine is injected and running set, skipping Start's ObjectStore attach —
	// bodies never hydrate here (the stub returns no results).
	newComp := func(lens string) *Component {
		g, err := fusionnats.New(tc.Client, 0)
		if err != nil {
			t.Fatalf("fusionnats.New: %v", err)
		}
		eng, err := fusion.NewEngine(g, fusion.NewBodyResolver(fusion.MapStoreResolver{}), slog.Default())
		if err != nil {
			t.Fatalf("fusion.NewEngine: %v", err)
		}
		return &Component{
			name:          "code-context",
			lensKind:      lens,
			subjectRoot:   lens + ".v1.",
			org:           "acme",
			platform:      "test-a1b2c3",
			sourceSystems: []string{"fixture"},
			graph:         g,
			engine:        eng,
			logger:        slog.Default(),
			running:       true,
			startTime:     time.Now(),
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
		want := []string{"acme.test-a1b2c3.fixture.web", "acme.test-a1b2c3.fixture.config"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("semantic scope = %v, want %v", got, want)
		}
	})

	t.Run("code lens defaults scope to the code-language domains on the wire", func(t *testing.T) {
		got := fetchScope(t, newComp("code"), `{"query":"where is the retry handler"}`)
		want := []string{
			"acme.test-a1b2c3.fixture.golang",
			"acme.test-a1b2c3.fixture.python",
			"acme.test-a1b2c3.fixture.typescript",
			"acme.test-a1b2c3.fixture.javascript",
			"acme.test-a1b2c3.fixture.java",
			"acme.test-a1b2c3.fixture.svelte",
			"acme.test-a1b2c3.fixture.c",
			"acme.test-a1b2c3.fixture.cpp",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("semantic scope = %v, want %v", got, want)
		}
	})

	t.Run("a caller-provided scope is passed through verbatim", func(t *testing.T) {
		got := fetchScope(t, newComp("docs"),
			`{"query":"how does retry work","scope":["other.platform.foreign.golang"]}`)
		want := []string{"other.platform.foreign.golang"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("semantic scope = %v, want caller scope %v", got, want)
		}
	})
}
