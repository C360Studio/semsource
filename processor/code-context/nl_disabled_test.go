package codecontext

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	semgraph "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/fusion"

	"github.com/c360studio/semsource/source/fusion/fusiontest"
)

// modeRecorder records every resolve mode that reaches the substrate.
type modeRecorder struct {
	*fusiontest.MemGraph
	modes []fusion.ResolveMode
}

func (g *modeRecorder) Resolve(ctx context.Context, q fusion.ResolveQuery) (fusion.Resolution, error) {
	g.modes = append(g.modes, q.Mode)
	return g.MemGraph.Resolve(ctx, q)
}

func configWith(t *testing.T, raw string) Config {
	t.Helper()
	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func assertNotEnabled(t *testing.T, err error) {
	t.Helper()
	var ce *errs.ClassifiedError
	if err == nil || !errs.IsInvalid(err) || !errors.As(err, &ce) || ce.Code != semgraph.ErrorCodeNotEnabled {
		t.Fatalf("err = %v, want an invalid not_enabled refusal", err)
	}
	if errs.IsTransient(err) {
		t.Fatalf("not_enabled must not be transient: %v", err)
	}
}

// TestRetrievalClientRefusesNLWithoutSearchIndex: with graph.lexical_search
// false the composition sets semantic_enabled false, and an nl resolve must be
// refused permanently before it reaches the substrate (where no responder
// reads as a transient outage). Symbol and prefix resolves are untouched, and
// with search on nl passes through.
//
// spec: openspec/specs/tier0-consumer-qualification/spec.md#Requirement: Tier 0 qualifies an exclusive no-embedder composition
func TestRetrievalClientRefusesNLWithoutSearchIndex(t *testing.T) {
	for _, lens := range []string{"code", "docs"} {
		t.Run(lens, func(t *testing.T) {
			rec := &modeRecorder{MemGraph: fusiontest.NewMemGraph()}
			off := retrievalClient(rec, configWith(t, `{"lens":"`+lens+`","semantic_enabled":false}`), component.Dependencies{})

			_, err := off.Resolve(context.Background(), fusion.ResolveQuery{Query: "where are bodies kept", Mode: fusion.ResolveModeNL})
			assertNotEnabled(t, err)
			if len(rec.modes) != 0 {
				t.Fatalf("refused nl resolve still reached the substrate: %v", rec.modes)
			}
			for _, mode := range []fusion.ResolveMode{fusion.ResolveModeSymbol, fusion.ResolveModePrefix} {
				if _, err := off.Resolve(context.Background(), fusion.ResolveQuery{Query: "Dispatch", Mode: mode}); err != nil {
					t.Fatalf("%s resolve refused with search off: %v", mode, err)
				}
			}

			rec.modes = nil
			on := retrievalClient(rec, configWith(t, `{"lens":"`+lens+`"}`), component.Dependencies{})
			if _, err := on.Resolve(context.Background(), fusion.ResolveQuery{Query: "where are bodies kept", Mode: fusion.ResolveModeNL}); err != nil {
				t.Fatalf("nl resolve refused with search on: %v", err)
			}
			if len(rec.modes) != 1 || rec.modes[0] != fusion.ResolveModeNL {
				t.Fatalf("nl resolve with search on did not reach the substrate: %v", rec.modes)
			}
		})
	}
}

// TestDocContextRefusesNLWithoutSearchIndex: doc_context sends a phrase to the
// docs lens's context verb, which resolves it in nl mode, not through the
// search verb. Over HTTP it must answer 501 not_enabled, non-retryable (#291).
//
// spec: openspec/specs/tier0-consumer-qualification/spec.md#Requirement: Tier 0 qualifies an exclusive no-embedder composition
func TestDocContextRefusesNLWithoutSearchIndex(t *testing.T) {
	g := retrievalClient(fusiontest.NewMemGraph(), configWith(t, `{"lens":"docs","semantic_enabled":false}`), component.Dependencies{})
	c := newTestComponent("docs", g, fusiontest.NewMemStore())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/doc-context/context", strings.NewReader(`{"query":"how do I add a source"}`))
	c.handleHTTP(rec, req, "context")
	assertHTTPError(t, rec, http.StatusNotImplemented, semgraph.ErrorCodeNotEnabled, "invalid", false)
}
