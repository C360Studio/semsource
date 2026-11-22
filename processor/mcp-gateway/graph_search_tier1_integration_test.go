//go:build integration

package mcpgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/embedding/httpembedder"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/payloadregistry"
	graphembedding "github.com/c360studio/semengine/processor/graph-embedding"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	graphlexical "github.com/c360studio/semengine/processor/graph-lexical"
	graphquery "github.com/c360studio/semengine/processor/graph-query"
	"github.com/c360studio/semengine/types"

	"github.com/c360studio/semsource/internal/natstest"
	"github.com/c360studio/semsource/internal/testutil"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// fakeProvider is an OpenAI-shaped embeddings server. Texts mentioning a
// lantern or a lamp embed to one axis and everything else to another, so the
// question "glowing lamp" meets the lantern entity by meaning alone. While
// failQueries is set it refuses every query embedding, as a provider that went
// away after the documents were embedded.
type fakeProvider struct {
	failQueries atomic.Bool
}

func (p *fakeProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model     string   `json:"model"`
		Input     []string `json:"input"`
		InputType string   `json:"input_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.InputType == "query" && p.failQueries.Load() {
		http.Error(w, "provider unavailable", http.StatusServiceUnavailable)
		return
	}
	type datum struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	}
	out := struct {
		Model string  `json:"model"`
		Data  []datum `json:"data"`
	}{Model: req.Model}
	for i, text := range req.Input {
		vec := []float32{0, 1, 0}
		if lower := strings.ToLower(text); strings.Contains(lower, "lantern") || strings.Contains(lower, "lamp") {
			vec = []float32{1, 0, 0}
		}
		out.Data = append(out.Data, datum{Index: i, Embedding: vec})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// TestIntegration_GraphSearchDisclosesTheTierOneMerge drives graph_search
// through a real graph-query with embedding_enabled, graph-lexical, and
// graph-embedding over an HTTP provider, so the merged reply is the pinned
// substrate's own. A meaning-only question reaches its entity through the
// vector list, with the merged score and the ranks shown and the merge
// honoured; once the provider stops answering queries, the reply is the word
// list and says the merge was not honoured.
// spec: openspec/changes/qualify-semengine-tier1/specs/tier1-consumer-qualification/spec.md#Requirement: graph_search discloses the merged answer at Tier 1
func TestIntegration_GraphSearchDisclosesTheTierOneMerge(t *testing.T) {
	ctx := t.Context()
	tc := natstest.New(t, natstest.WithJetStream(),
		natstest.WithStreams(natstest.Stream{Name: "ENTITY", Subjects: []string{"entity.>"}}))
	states, err := tc.Client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: gtypes.BucketEntityStates})
	if err != nil {
		t.Fatalf("create ENTITY_STATES: %v", err)
	}
	const (
		lanternID = "acme.semsource.repo.config.dependency.lantern"
		otherID   = "acme.semsource.docs.web.doc.unrelated"
	)
	now := time.Now().UTC()
	for id, title := range map[string]string{lanternID: "cobalt lantern routing library", otherID: "an unrelated page"} {
		raw, err := gtypes.MarshalEntityState(&gtypes.EntityState{ID: id, Triples: []message.Triple{
			{Subject: id, Predicate: source.DcTitle, Object: title, Source: "test", Timestamp: now},
		}})
		if err != nil {
			t.Fatalf("marshal %s: %v", id, err)
		}
		if _, err := states.Put(ctx, id, raw); err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
	}

	provider := &fakeProvider{}
	srv := httptest.NewServer(provider)
	t.Cleanup(srv.Close)

	deps := component.Dependencies{NATSClient: tc.Client, MetricsRegistry: metric.NewMetricsRegistry()}
	startLifecycle(t, "graph-ingest", func() (component.Discoverable, error) {
		withRegistry := deps
		withRegistry.PayloadRegistry = payloadregistry.New()
		withRegistry.Platform = types.PlatformMeta{Org: "acme", Platform: "semsource"}
		return graphingest.CreateGraphIngest(json.RawMessage(`{}`), withRegistry)
	})
	startLifecycle(t, "graph-lexical", func() (component.Discoverable, error) {
		return graphlexical.CreateGraphLexical(json.RawMessage(`{}`), deps)
	})
	registry := component.NewRegistry()
	if err := graphembedding.RegisterWithEmbedder(registry, "http", func() (embedding.Embedder, error) {
		return httpembedder.New(httpembedder.Config{BaseURL: srv.URL + "/v1", Model: "fake-embed"})
	}); err != nil {
		t.Fatalf("register graph-embedding: %v", err)
	}
	startLifecycle(t, "graph-embedding", func() (component.Discoverable, error) {
		return registry.CreateComponent("graph-embedding", types.ComponentConfig{
			Name: "graph-embedding", Type: types.ComponentTypeProcessor, Enabled: true, Config: json.RawMessage(`{}`),
		}, deps, nil)
	})
	startLifecycle(t, "graph-query", func() (component.Discoverable, error) {
		return graphquery.CreateGraphQuery(json.RawMessage(`{"embedding_enabled":true}`), deps)
	})

	cs := connect(t, graphToolComponent(t, tc.Client))
	search := func(query string) (graphSearchResult, string, bool) {
		t.Helper()
		res := callTool(t, cs, "graph_search", map[string]any{"query": query})
		raw := res.Content[0].(*mcp.TextContent).Text
		var out graphSearchResult
		if res.IsError || json.Unmarshal([]byte(raw), &out) != nil {
			return out, raw, false
		}
		return out, raw, true
	}

	// The indexes refuse index_not_ready until they have caught up, and the
	// merge is not honoured until the embedding search answers.
	var out graphSearchResult
	var last string
	merged := testutil.WaitFor(t, 30*time.Second, func() bool {
		var ok bool
		out, last, ok = search("glowing lamp")
		return ok && out.Merge != nil && out.Merge.Honoured && len(out.Matches) > 0
	})
	if !merged {
		t.Fatalf("graph_search never answered a merged reply; last result: %s", last)
	}
	m := out.Matches[0]
	if m.ID != lanternID || m.VectorRank != 1 || m.WordRank != 0 {
		t.Fatalf("top match = %+v, want the lantern entity from the vector list alone (vector_rank 1, no word_rank)", m)
	}
	if want := 1.0 / 61; m.Relevance <= 0 || m.Relevance > want+1e-9 {
		t.Errorf("relevance = %v, want the merged score 1/61 for a first place in one list", m.Relevance)
	}
	if !strings.Contains(out.Retrieval.Note, "merged by rank") {
		t.Errorf("note = %q, want it to say the hits are merged word and embedding hits", out.Retrieval.Note)
	}

	// The provider stops answering queries: the reply is the word list, and
	// says the merge was not honoured rather than passing as merged.
	provider.failQueries.Store(true)
	out, last, ok := search("cobalt lantern routing")
	if !ok {
		t.Fatalf("graph_search failed with the provider down; it must answer from words: %s", last)
	}
	if out.Merge == nil || out.Merge.Honoured || out.Merge.Reason == "" {
		t.Fatalf("merge = %+v, want honoured false with its reason surfaced; body=%s", out.Merge, last)
	}
	if len(out.Matches) != 1 || out.Matches[0].ID != lanternID || out.Matches[0].VectorRank != 0 {
		t.Fatalf("matches = %+v, want the word hit alone with no vector rank", out.Matches)
	}
	if !strings.Contains(out.Retrieval.Note, "merge was not honoured") {
		t.Errorf("note = %q, want it to say the merge was not honoured", out.Retrieval.Note)
	}
}
