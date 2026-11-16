//go:build integration

package mcpgateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
	graphlexical "github.com/c360studio/semengine/processor/graph-lexical"
	graphquery "github.com/c360studio/semengine/processor/graph-query"
	"github.com/c360studio/semengine/types"

	"github.com/c360studio/semsource/internal/natstest"
	"github.com/c360studio/semsource/internal/testutil"
	source "github.com/c360studio/semsource/source/vocabulary"
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

// TestIntegration_GraphSearchAgainstRealGraphQuery drives graph_search through
// a real graph-query and graph-lexical with no clustering (the Tier 0 shape),
// so the request is decoded strictly and the reply is the pinned substrate's
// own. It proves the tool answers rather than refusing, ranks the word hits
// with their IDs, labels, and allowlisted values, and says the answer is NOT
// community-backed.
// spec: openspec/specs/graphrag-access/spec.md#Requirement: The surface is available at every tier
func TestIntegration_GraphSearchAgainstRealGraphQuery(t *testing.T) {
	ctx := t.Context()
	// graph-ingest serves graph-query's batch read-back of the matched
	// entities; its default entity stream must exist for it to start.
	tc := natstest.New(t, natstest.WithJetStream(),
		natstest.WithStreams(natstest.Stream{Name: "ENTITY", Subjects: []string{"entity.>"}}))

	states, err := tc.Client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: gtypes.BucketEntityStates})
	if err != nil {
		t.Fatalf("create ENTITY_STATES: %v", err)
	}
	const (
		depID   = "acme.semsource.repo.config.dependency.guava"
		otherID = "acme.semsource.docs.web.doc.unrelated"
	)
	put := func(e gtypes.EntityState) {
		raw, err := gtypes.MarshalEntityState(&e)
		if err != nil {
			t.Fatalf("marshal %s: %v", e.ID, err)
		}
		if _, err := states.Put(ctx, e.ID, raw); err != nil {
			t.Fatalf("put %s: %v", e.ID, err)
		}
	}
	now := time.Now().UTC()
	fact := func(subject, predicate string, object any) message.Triple {
		return message.Triple{Subject: subject, Predicate: predicate, Object: object, Source: "test", Timestamp: now}
	}
	put(gtypes.EntityState{ID: depID, Triples: []message.Triple{
		fact(depID, source.DcTitle, "cobalt lantern routing library"),
		fact(depID, source.ConfigDepVersion, "33.0.0-jre"),
	}})
	put(gtypes.EntityState{ID: otherID, Triples: []message.Triple{
		fact(otherID, source.DcTitle, "an unrelated page"),
	}})

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
	startLifecycle(t, "graph-query", func() (component.Discoverable, error) {
		return graphquery.CreateGraphQuery(json.RawMessage(`{}`), deps)
	})

	cs := connect(t, graphToolComponent(t, tc.Client))
	var out graphSearchResult
	var last string
	answered := testutil.WaitFor(t, 15*time.Second, func() bool {
		res := callTool(t, cs, "graph_search", map[string]any{"query": "cobalt lantern routing"})
		last = res.Content[0].(*mcp.TextContent).Text
		if res.IsError {
			return false // the word index refuses index_not_ready until it has caught up
		}
		out = graphSearchResult{}
		return json.Unmarshal([]byte(last), &out) == nil && len(out.Matches) > 0
	})
	if !answered {
		t.Fatalf("graph_search never answered with matches; last result: %s", last)
	}

	if len(out.Matches) != 1 || out.TotalMatches != 1 || out.Truncated {
		t.Fatalf("matches = %+v total=%d truncated=%v, want exactly the guava dependency",
			out.Matches, out.TotalMatches, out.Truncated)
	}
	m := out.Matches[0]
	if m.ID != depID || m.Type != "dependency" || m.Label == "" || m.Relevance <= 0 {
		t.Errorf("match lost the substrate's ID, label, or score: %+v", m)
	}
	if m.Properties[source.ConfigDepVersion] != "33.0.0-jre" {
		t.Errorf("properties = %v, want the dependency version from the matched entity", m.Properties)
	}
	if out.Retrieval.CommunityBacked || out.Retrieval.Rung != rungEntitiesOnly {
		t.Errorf("a clustering-off answer was reported as community-backed: %+v", out.Retrieval)
	}
	if out.Answer == "" || out.Retrieval.AnswerSource != answerSourceTemplate {
		t.Errorf("answer=%q source=%q, want the substrate's template answer disclosed as such",
			out.Answer, out.Retrieval.AnswerSource)
	}
	if strings.Contains(last, source.DcTitle) {
		t.Errorf("graph_search leaked entity triples into the result: %s", last)
	}
}

// TestIntegration_GraphSearchUnreadableReplyDisclosesUnknown pins the request
// the tool sends (exactly the GlobalSearchRequest keys the substrate accepts)
// and that an unreadable reply lowers the disclosure rather than failing the
// call or claiming community backing.
func TestIntegration_GraphSearchUnreadableReplyDisclosesUnknown(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t)
	sub, err := tc.Client.SubscribeForRequests(ctx, "graph.query.searchGraph",
		func(_ context.Context, data []byte) ([]byte, error) {
			var req map[string]any
			if err := json.Unmarshal(data, &req); err != nil {
				return nil, err
			}
			want := map[string]any{"query": "how does readiness gating work", "limit": float64(maxGraphMatches)}
			if !jsonEqual(req, want) {
				t.Errorf("request = %v, want exactly %v", req, want)
			}
			return []byte(`not json at all`), nil
		})
	if err != nil {
		t.Fatalf("subscribe stub: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	cs := connect(t, graphToolComponent(t, tc.Client))
	res := callTool(t, cs, "graph_search", map[string]any{"query": "how does readiness gating work"})
	if res.IsError {
		t.Fatalf("an unreadable reply must not fail the call: %+v", res)
	}
	var out graphSearchResult
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &out); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}
	if out.Retrieval.Rung != rungUnknown || out.Retrieval.CommunityBacked || out.Retrieval.Note == "" {
		t.Errorf("disclosure = %+v, want the unknown rung, explained, never community-backed", out.Retrieval)
	}
}

// TestIntegration_GraphSearchCarriesHonouredEntityAnswer drives the tool end
// to end on a one-word reference the substrate answered beside zero word
// matches: the entity reaches the agent, honoured, with no triples leaked.
func TestIntegration_GraphSearchCarriesHonouredEntityAnswer(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t)
	const reply = `{"data":{"text":{"level":0,"groups":[],"matches":[],"matched":0,"missing":0,
		"bodies_not_indexed":0,"truncated":false,"unreadable_groups":0,"answer":"Found 0 matching entities in 0 groups."},
		"entity":{"id":"acme.semsource.repo.config.dependency.guava","triples":[
			{"subject":"acme.semsource.repo.config.dependency.guava","predicate":"dc.terms.title","object":"guava"}]},
		"hints":[{"hint":"entity","detail":"guava","honoured":true}]},
		"indexed_revision":3,"producer":"graph-query"}`
	sub, err := tc.Client.SubscribeForRequests(ctx, "graph.query.searchGraph",
		func(context.Context, []byte) ([]byte, error) { return []byte(reply), nil })
	if err != nil {
		t.Fatalf("subscribe stub: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	cs := connect(t, graphToolComponent(t, tc.Client))
	res := callTool(t, cs, "graph_search", map[string]any{"query": "guava"})
	raw := res.Content[0].(*mcp.TextContent).Text
	var out graphSearchResult
	if res.IsError || json.Unmarshal([]byte(raw), &out) != nil {
		t.Fatalf("graph_search failed: %s", raw)
	}
	if out.Entity == nil || out.Entity.ID != "acme.semsource.repo.config.dependency.guava" || out.Entity.Label != "guava" {
		t.Fatalf("honoured entity answer lost: %s", raw)
	}
	if len(out.Hints) != 1 || !out.Hints[0].Honoured || len(out.Matches) != 0 {
		t.Errorf("hints=%+v matches=%d, want the honoured hint beside zero matches", out.Hints, len(out.Matches))
	}
	if strings.Contains(raw, `"triples"`) {
		t.Errorf("the entity answer leaked raw triples: %s", raw)
	}
}

// startLifecycle creates, initializes, and starts a substrate component, and
// stops it at cleanup.
func startLifecycle(t *testing.T, name string, create func() (component.Discoverable, error)) {
	t.Helper()
	d, err := create()
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	lc, ok := d.(component.LifecycleComponent)
	if !ok {
		t.Fatalf("%s is not a lifecycle component", name)
	}
	if err := lc.Initialize(); err != nil {
		t.Fatalf("%s Initialize: %v", name, err)
	}
	if err := lc.Start(t.Context()); err != nil {
		t.Fatalf("%s Start: %v", name, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := lc.Stop(ctx); err != nil {
			t.Errorf("%s Stop: %v", name, err)
		}
	})
}
