//go:build integration

package codecontext

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	semgraph "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/fusion"
	"github.com/c360studio/semengine/pkg/fusion/fusionnats"
	"github.com/c360studio/semsource/internal/natstest"
)

// TestIntegration_FusionRepliesCarryMergedSearchProvenance: code_search and
// doc_context seed natural-language questions only through graph-query's
// mergedSearch, and their replies carry the fusion response's provenance as
// the real fusionnats client derives it: embedding when a returned seed came
// from the embedding list (tier 1), lexical for the word list alone (tier 0,
// or a merge that was not honoured).
// spec: openspec/changes/qualify-semengine-tier1/specs/tier1-consumer-qualification/spec.md#Requirement: Fusion tools disclose where Tier 1 seeds came from
func TestIntegration_FusionRepliesCarryMergedSearchProvenance(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t)
	const seedID = "acme.test-a1b2c3.fixture.golang.function.retry"

	// mergedReply is graph.query.mergedSearch's reply, picked per question so
	// one stub serves every case. The question names the case.
	mergedReply := map[string]string{
		"tier one meaning": `{"results":[{"entity_id":"` + seedID + `","score":0.0163,"vector_rank":1}],"bodies_not_indexed":0,"merge":{"hint":"semantic","detail":"merged","honoured":true}}`,
		"tier zero words":  `{"results":[{"entity_id":"` + seedID + `","score":3.2}],"bodies_not_indexed":0}`,
		"provider is down": `{"results":[{"entity_id":"` + seedID + `","score":3.2}],"bodies_not_indexed":0,"merge":{"hint":"semantic","detail":"merged","honoured":false,"reason":"embed the query: provider answered 503","code":"internal"}}`,
	}
	asked := make(chan string, 16)
	mergedSub, err := tc.Client.SubscribeForRequests(ctx, "graph.query.mergedSearch",
		func(_ context.Context, body []byte) ([]byte, error) {
			var req struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				return nil, err
			}
			asked <- req.Query
			data, ok := mergedReply[req.Query]
			if !ok {
				return nil, fmt.Errorf("unexpected question %q", req.Query)
			}
			return []byte(`{"producer":"graph-query","data":` + data + `}`), nil
		})
	if err != nil {
		t.Fatalf("subscribe mergedSearch: %v", err)
	}
	t.Cleanup(func() { _ = mergedSub.Unsubscribe() })
	entity, err := json.Marshal(semgraph.EntityState{ID: seedID, Triples: []message.Triple{
		{Subject: seedID, Predicate: "dc.terms.title", Object: "retry", Source: "test", Timestamp: time.Now().UTC()},
	}})
	if err != nil {
		t.Fatalf("marshal entity: %v", err)
	}
	batchSub, err := tc.Client.SubscribeForRequests(ctx, "graph.query.batch",
		func(context.Context, []byte) ([]byte, error) {
			// batch is not enveloped: the reply is the EntityBatchResponse itself.
			return []byte(`{"entities":[` + string(entity) + `]}`), nil
		})
	if err != nil {
		t.Fatalf("subscribe batch: %v", err)
	}
	t.Cleanup(func() { _ = batchSub.Unsubscribe() })

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
			name: "code-context", lensKind: lens, subjectRoot: lens + ".v1.",
			org: "acme", platform: "test-a1b2c3", sourceSystems: []string{"fixture"},
			graph: g, engine: eng, logger: slog.Default(), running: true, startTime: time.Now(),
		}
	}

	for _, tc := range []struct {
		lens, verb, question string
		want                 fusion.Provenance
	}{
		{lens: "code", verb: "search", question: "tier one meaning", want: fusion.ProvenanceEmbedding},
		{lens: "docs", verb: "context", question: "tier one meaning", want: fusion.ProvenanceEmbedding},
		{lens: "code", verb: "search", question: "tier zero words", want: fusion.ProvenanceLexical},
		{lens: "code", verb: "search", question: "provider is down", want: fusion.ProvenanceLexical},
	} {
		t.Run(tc.lens+" "+tc.verb+" "+tc.question, func(t *testing.T) {
			reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			raw, err := newComp(tc.lens).serve(reqCtx, tc.verb, []byte(`{"query":"`+tc.question+`"}`))
			if err != nil {
				t.Fatalf("serve: %v", err)
			}
			select {
			case q := <-asked:
				if q != tc.question {
					t.Fatalf("mergedSearch asked %q, want the question unchanged", q)
				}
			default:
				t.Fatal("the question did not go to graph.query.mergedSearch")
			}
			if !strings.Contains(string(raw), `"provenance":"`+string(tc.want)+`"`) {
				t.Fatalf("reply provenance is not %q: %s", tc.want, raw)
			}
			var resp fusion.Response
			if err := json.Unmarshal(raw, &resp); err != nil {
				t.Fatal(err)
			}
			if len(resp.Nodes) == 0 || resp.Nodes[0].Name != "retry" {
				t.Fatalf("nodes = %+v, want the merged seed hydrated: %s", resp.Nodes, raw)
			}
		})
	}
}
