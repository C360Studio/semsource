//go:build integration

package mcpgateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/c360studio/semstreams/natsclient"
)

// graphToolComponent wires a gateway to a live test NATS client.
func graphToolComponent(t *testing.T, client *natsclient.Client) *Component {
	t.Helper()
	c := &Component{
		name:   "mcp-gateway",
		config: Config{Namespace: "acme", RequestTimeoutMs: 3000},
		client: client,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	c.server = c.buildServer()
	return c
}

// TestIntegration_GraphSearchDisclosesAndPassesThrough proves the two guarantees
// that make this tool safe to ship on a stack without clustering: the substrate
// payload arrives verbatim, and the answer is labelled as NOT community-backed
// so an agent cannot read similarity hits as thematic reasoning.
//
// It also pins that a graph-query success is not rejected for lacking
// contract_version — that field belongs to the fusion contract, and the
// substrate does not emit it.
func TestIntegration_GraphSearchDisclosesAndPassesThrough(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t)

	const substrateBody = `{"entity_digests":[{"id":"acme.semsource.golang.x.function.Foo","relevance":0.7}],"count":1,"duration_ms":9}`
	sub, err := tc.Client.SubscribeForRequests(ctx, "graph.query.searchGraph",
		func(_ context.Context, data []byte) ([]byte, error) {
			var req map[string]string
			if err := json.Unmarshal(data, &req); err != nil {
				return nil, err
			}
			if req["query"] != "how does readiness gating work" {
				t.Errorf("query not forwarded verbatim: %v", req)
			}
			return []byte(substrateBody), nil
		})
	if err != nil {
		t.Fatalf("subscribe stub: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	cs := connect(t, graphToolComponent(t, tc.Client))
	res := callTool(t, cs, "graph_search", map[string]any{"query": "how does readiness gating work"})
	if res.IsError {
		t.Fatalf("graph_search returned a tool error: %+v", res)
	}

	var out struct {
		Retrieval retrievalDisclosure `json:"retrieval"`
		Result    json.RawMessage     `json:"result"`
	}
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &out); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}

	if out.Retrieval.CommunityBacked {
		t.Errorf("a non-clustered answer was reported as community-backed: %+v", out.Retrieval)
	}
	if out.Retrieval.Rung != rungEntitiesOnly {
		t.Errorf("rung = %q, want %q", out.Retrieval.Rung, rungEntitiesOnly)
	}
	if !strings.Contains(out.Retrieval.Note, "NOT community-backed") {
		t.Errorf("disclosure note does not warn the agent: %q", out.Retrieval.Note)
	}

	var got, want any
	if err := json.Unmarshal(out.Result, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if err := json.Unmarshal([]byte(substrateBody), &want); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if !jsonEqual(got, want) {
		t.Errorf("substrate payload not verbatim:\n got %s\nwant %s", out.Result, substrateBody)
	}
}

// TestIntegration_GraphSummaryReturnsTheSubstrateShape asserts the SHAPE, not
// merely a non-error result — deliberately.
//
// graph.query.summary was contested: SemSource's source-manifest subscribed to
// it alongside the substrate's graph-query, and the two return entirely
// different payloads. A "did it error?" assertion passes under either handler,
// which is precisely why the collision survived a full green suite. Asserting
// the substrate's entity-type/predicate shape means a reintroduced competing
// handler fails here, because source-manifest's payload decodes to an empty
// SummaryData.
func TestIntegration_GraphSummaryReturnsTheSubstrateShape(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t)

	// The substrate's shape: entity-type aggregation + predicate counts.
	const substrateBody = `{"total_entities":6,"entity_types":[{"type":"function","count":2,"examples":["acme.semsource.golang.x.function.Foo"]}],"predicates":[{"predicate":"code.artifact.path","entity_count":4}]}`
	sub, err := tc.Client.SubscribeForRequests(ctx, "graph.query.summary",
		func(_ context.Context, data []byte) ([]byte, error) {
			// An empty body is the contract: the substrate applies its defaults.
			if len(data) != 0 {
				t.Errorf("expected an empty request body, got %s", data)
			}
			return []byte(substrateBody), nil
		})
	if err != nil {
		t.Fatalf("subscribe stub: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	cs := connect(t, graphToolComponent(t, tc.Client))
	res := callTool(t, cs, "graph_summary", map[string]any{})
	if res.IsError {
		t.Fatalf("graph_summary returned a tool error: %+v", res)
	}

	got := res.Content[0].(*mcp.TextContent).Text
	if got != substrateBody {
		t.Errorf("graph_summary is not a verbatim passthrough:\n got %s\nwant %s", got, substrateBody)
	}

	// Shape assertion: this is what a competing handler's payload would fail.
	var summary struct {
		EntityTypes []struct {
			Type  string `json:"type"`
			Count int    `json:"count"`
		} `json:"entity_types"`
		Predicates []struct {
			Predicate string `json:"predicate"`
		} `json:"predicates"`
	}
	if err := json.Unmarshal([]byte(got), &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if len(summary.EntityTypes) == 0 || summary.EntityTypes[0].Type == "" {
		t.Errorf("no substrate entity-type summary — did a different handler answer? %s", got)
	}
	if len(summary.Predicates) == 0 || summary.Predicates[0].Predicate == "" {
		t.Errorf("no substrate predicate summary — did a different handler answer? %s", got)
	}

	// A disclosure block here would imply a retrieval path that was never chosen.
	if strings.Contains(got, `"retrieval"`) {
		t.Errorf("graph_summary must not carry a retrieval disclosure: %s", got)
	}
}
