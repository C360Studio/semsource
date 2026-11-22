//go:build integration

package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	semgraph "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semsource/internal/graphstatus"
	"github.com/c360studio/semsource/internal/natstest"

	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
)

// TestIntegration_AddSourceTranslatesToNATS proves the gateway's core job: an MCP
// add_source tool call becomes a NATS request on graph.ingest.add.{namespace},
// and the reply is returned to the caller. A stub responder stands in for
// source-manifest so the test isolates the translation.
// spec: openspec/specs/mcp-gateway-contract/spec.md#Requirement: add_source is a namespaced NATS ingest request
func TestIntegration_AddSourceTranslatesToNATS(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t)

	sub, err := tc.Client.SubscribeForRequests(ctx, "graph.ingest.add.acme",
		func(_ context.Context, data []byte) ([]byte, error) {
			var req sourcemanifest.AddRequest
			if err := json.Unmarshal(data, &req); err != nil {
				return nil, err
			}
			reply := sourcemanifest.AddReply{
				Components: []sourcemanifest.AddedComponent{
					{InstanceName: "git-source-x", SourceType: req.Source.Type, Created: true},
				},
				StatusSubject: "graph.ingest.status",
				ReadyWhen:     "phase in ['watching','idle']",
				Timestamp:     time.Now(),
			}
			return json.Marshal(&reply)
		})
	if err != nil {
		t.Fatalf("subscribe stub: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	c := &Component{
		name:   "mcp-gateway",
		config: Config{Namespace: "acme", RequestTimeoutMs: 3000},
		client: tc.Client,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	c.server = c.buildServer()
	cs := connect(t, c)

	res := callTool(t, cs, "add_source", map[string]any{
		"type":   "git",
		"url":    "https://example.com/x.git",
		"branch": "main",
	})
	if res.IsError {
		t.Fatalf("add_source returned a tool error: %+v", res)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "git-source-x") {
		t.Fatalf("reply missing the spawned component handle: %s", text)
	}
}

// TestIntegration_SourceStatusMergesSignals proves source_status merges the three
// honest readiness signals (ADR-066) — the source-manifest ingest status
// (graph.query.status request/reply), plus the graph-index structural and
// graph-embedding semantic readiness, which since ADR-083 are KV envelopes under
// GRAPH_STATUS rather than request/reply subjects — plus the note.
// spec: openspec/specs/ingestion-readiness/spec.md#Requirement: Readiness sub-signals are present and honest on every surface
func TestIntegration_SourceStatusMergesSignals(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t, natstest.WithJetStream())

	sub1, err := tc.Client.SubscribeForRequests(ctx, "graph.query.status",
		func(_ context.Context, _ []byte) ([]byte, error) {
			return []byte(`{"namespace":"ss","phase":"ready","total_entities":42}`), nil
		})
	if err != nil {
		t.Fatalf("subscribe status: %v", err)
	}
	t.Cleanup(func() { _ = sub1.Unsubscribe() })

	// Seed the readiness producers' envelopes the way the producers would, so the
	// two signals are deliberately DIFFERENT: a still-building structural index
	// alongside a ready semantic one is exactly the state an agent must be able to
	// tell apart.
	statusKV, err := kvcatalog.EnsureCatalogBucket(ctx, tc.Client, semgraph.BucketGraphStatus)
	if err != nil {
		t.Fatalf("ensure %s: %v", semgraph.BucketGraphStatus, err)
	}
	if _, err := statusKV.PutString(ctx, "graph-index", `{"ready":false,"state":"building"}`); err != nil {
		t.Fatalf("seed graph-index readiness: %v", err)
	}
	if _, err := statusKV.PutString(ctx, "graph-lexical", `{"ready":true,"state":"ready"}`); err != nil {
		t.Fatalf("seed graph-embedding readiness: %v", err)
	}

	c := &Component{
		name:        "mcp-gateway",
		config:      Config{Namespace: "ss", RequestTimeoutMs: 3000},
		client:      tc.Client,
		graphStatus: graphstatus.New(tc.Client),
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	c.server = c.buildServer()
	cs := connect(t, c)

	res := callTool(t, cs, "source_status", map[string]any{})
	if res.IsError {
		t.Fatalf("source_status errored: %+v", res)
	}
	body := res.Content[0].(*mcp.TextContent).Text

	var merged struct {
		Status    json.RawMessage `json:"status"`
		Index     json.RawMessage `json:"index"`
		Embedding json.RawMessage `json:"embedding"`
		Note      string          `json:"note"`
	}
	if err := json.Unmarshal([]byte(body), &merged); err != nil {
		t.Fatalf("decode merged status: %v; body=%s", err, body)
	}
	if !strings.Contains(string(merged.Status), `"phase":"ready"`) {
		t.Errorf("status signal missing: %s", merged.Status)
	}
	if !strings.Contains(string(merged.Index), `"state":"building"`) {
		t.Errorf("index signal missing: %s", merged.Index)
	}
	if !strings.Contains(string(merged.Embedding), `"state":"ready"`) {
		t.Errorf("embedding signal missing: %s", merged.Embedding)
	}
	if merged.Note == "" {
		t.Error("readiness note missing")
	}
}

// TestIntegration_SourceStatusReportsDisabledSearchIndex: with semantic search
// off in the composition, source_status must report the search index as
// disabled, as the HTTP surface does, even when a graph-lexical readiness
// envelope says ready. Reporting it ready tells an agent code_search is
// reliable, which it then refuses (#270).
// spec: openspec/specs/ingestion-readiness/spec.md#Requirement: Readiness sub-signals are present and honest on every surface
func TestIntegration_SourceStatusReportsDisabledSearchIndex(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t, natstest.WithJetStream())

	sub, err := tc.Client.SubscribeForRequests(ctx, "graph.query.status",
		func(_ context.Context, _ []byte) ([]byte, error) {
			return []byte(`{"namespace":"ss","phase":"ready","total_entities":42}`), nil
		})
	if err != nil {
		t.Fatalf("subscribe status: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	statusKV, err := kvcatalog.EnsureCatalogBucket(ctx, tc.Client, semgraph.BucketGraphStatus)
	if err != nil {
		t.Fatalf("ensure %s: %v", semgraph.BucketGraphStatus, err)
	}
	if _, err := statusKV.PutString(ctx, "graph-lexical", `{"ready":true,"state":"ready"}`); err != nil {
		t.Fatalf("seed graph-lexical readiness: %v", err)
	}

	disabled := false
	c := &Component{
		name:        "mcp-gateway",
		config:      Config{Namespace: "ss", RequestTimeoutMs: 3000, SemanticEnabled: &disabled},
		client:      tc.Client,
		graphStatus: graphstatus.New(tc.Client),
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	c.server = c.buildServer()
	cs := connect(t, c)

	res := callTool(t, cs, "source_status", map[string]any{})
	if res.IsError {
		t.Fatalf("source_status errored: %+v", res)
	}
	var merged struct {
		Embedding struct {
			Available bool   `json:"available"`
			Ready     bool   `json:"ready"`
			State     string `json:"state"`
			Reason    *struct {
				Code string `json:"code"`
			} `json:"reason"`
		} `json:"embedding"`
	}
	body := res.Content[0].(*mcp.TextContent).Text
	if err := json.Unmarshal([]byte(body), &merged); err != nil {
		t.Fatalf("decode merged status: %v; body=%s", err, body)
	}
	emb := merged.Embedding
	if emb.Available || emb.Ready || emb.State != "disabled" || emb.Reason == nil || emb.Reason.Code != "not_enabled" {
		t.Fatalf("embedding = %+v, want available=false ready=false state=disabled reason=not_enabled; body=%s", emb, body)
	}
}

// TestIntegration_SourceStatusWaitsOnGraphEmbedding: at tier 1 source_status's
// embedding block is ready only while graph-lexical and graph-embedding both
// are. A provider that does not answer leaves graph-embedding degraded, and the
// block must say so even though the word index is ready.
// spec: openspec/changes/qualify-semengine-tier1/specs/tier1-consumer-qualification/spec.md#Requirement: Tier 1 acceptance needs a reachable, identified provider
func TestIntegration_SourceStatusWaitsOnGraphEmbedding(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t, natstest.WithJetStream())

	sub, err := tc.Client.SubscribeForRequests(ctx, "graph.query.status",
		func(_ context.Context, _ []byte) ([]byte, error) {
			return []byte(`{"namespace":"ss","phase":"ready","total_entities":42}`), nil
		})
	if err != nil {
		t.Fatalf("subscribe status: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	statusKV, err := kvcatalog.EnsureCatalogBucket(ctx, tc.Client, semgraph.BucketGraphStatus)
	if err != nil {
		t.Fatalf("ensure %s: %v", semgraph.BucketGraphStatus, err)
	}
	if _, err := statusKV.PutString(ctx, "graph-lexical", `{"ready":true,"state":"ready"}`); err != nil {
		t.Fatalf("seed graph-lexical readiness: %v", err)
	}

	c := &Component{
		name:        "mcp-gateway",
		config:      Config{Namespace: "ss", RequestTimeoutMs: 3000, EmbeddingEnabled: true},
		client:      tc.Client,
		graphStatus: graphstatus.New(tc.Client),
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	c.server = c.buildServer()
	cs := connect(t, c)

	embedding := func() (ready, available bool, state string) {
		t.Helper()
		res := callTool(t, cs, "source_status", map[string]any{})
		if res.IsError {
			t.Fatalf("source_status errored: %+v", res)
		}
		var merged struct {
			Embedding struct {
				Available bool   `json:"available"`
				Ready     bool   `json:"ready"`
				State     string `json:"state"`
			} `json:"embedding"`
		}
		body := res.Content[0].(*mcp.TextContent).Text
		if err := json.Unmarshal([]byte(body), &merged); err != nil {
			t.Fatalf("decode merged status: %v; body=%s", err, body)
		}
		return merged.Embedding.Ready, merged.Embedding.Available, merged.Embedding.State
	}

	if ready, available, state := embedding(); ready || available {
		t.Fatalf("before graph-embedding published: ready=%v available=%v state=%q, want unavailable", ready, available, state)
	}
	if _, err := statusKV.PutString(ctx, "graph-embedding", `{"ready":false,"state":"degraded","failed_count":42}`); err != nil {
		t.Fatalf("seed graph-embedding readiness: %v", err)
	}
	if ready, _, state := embedding(); ready || state != "degraded" {
		t.Fatalf("provider unreachable: ready=%v state=%q, want not ready, degraded", ready, state)
	}
	if _, err := statusKV.PutString(ctx, "graph-embedding", `{"ready":true,"state":"ready"}`); err != nil {
		t.Fatalf("seed graph-embedding readiness: %v", err)
	}
	if ready, _, state := embedding(); !ready || state != "ready" {
		t.Fatalf("both halves ready: ready=%v state=%q, want ready", ready, state)
	}
}

// TestIntegration_QueryToolsTranslateToNATS proves the read-side MCP tools are
// real NATS-backed facades, not just advertised names. Each tool call is routed
// to its owned request/reply subject and returns the responder payload verbatim.
// spec: openspec/specs/advertised-surface-coverage/spec.md#Requirement: Agent-facing MCP tools have happy-path coverage
func TestIntegration_QueryToolsTranslateToNATS(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t)

	tests := []struct {
		name        string
		tool        string
		subject     string
		args        map[string]any
		wantRequest map[string]string
		response    string
		wantText    []string
	}{
		{
			name:        "code context",
			tool:        "code_context",
			subject:     "code.v1.context",
			args:        map[string]any{"query": "Widget"},
			wantRequest: map[string]string{"query": "Widget"},
			response:    `{"answer":"context answer","definition":{"name":"Widget"}}`,
			wantText:    []string{"context answer", "Widget"},
		},
		{
			name:        "code search",
			tool:        "code_search",
			subject:     "code.v1.search",
			args:        map[string]any{"query": "retry backoff"},
			wantRequest: map[string]string{"query": "retry backoff"},
			response:    `{"results":[{"name":"retryWithBackoff"}]}`,
			wantText:    []string{"retryWithBackoff"},
		},
		{
			name:        "code impact",
			tool:        "code_impact",
			subject:     "code.v1.impact",
			args:        map[string]any{"query": "Processor.Run"},
			wantRequest: map[string]string{"query": "Processor.Run"},
			response:    `{"impacted":[{"name":"Controller.Start"}]}`,
			wantText:    []string{"Controller.Start"},
		},
		{
			name:        "doc context",
			tool:        "doc_context",
			subject:     "docs.v1.context",
			args:        map[string]any{"query": "operator guide"},
			wantRequest: map[string]string{"query": "operator guide"},
			response:    `{"documents":[{"title":"Operator Guide"}]}`,
			wantText:    []string{"Operator Guide"},
		},
		{
			name:    "code changes",
			tool:    "code_changes",
			subject: "graph.query.versionDiff",
			args: map[string]any{
				"project": "github.com/acme/app",
				"from":    "v1.0.0",
				"to":      "v1.1.0",
			},
			wantRequest: map[string]string{
				"project": "github.com/acme/app",
				"from":    "v1.0.0",
				"to":      "v1.1.0",
			},
			response: `{"project":"github.com/acme/app","changed":[{"name":"Run"}]}`,
			wantText: []string{"github.com/acme/app", "Run"},
		},
	}

	for _, tt := range tests {
		tt := tt
		sub, err := tc.Client.SubscribeForRequests(ctx, tt.subject,
			func(_ context.Context, data []byte) ([]byte, error) {
				if err := assertRequestFields(data, tt.wantRequest); err != nil {
					return nil, err
				}
				return []byte(tt.response), nil
			})
		if err != nil {
			t.Fatalf("subscribe %s: %v", tt.subject, err)
		}
		t.Cleanup(func() { _ = sub.Unsubscribe() })
	}

	c := &Component{
		name:   "mcp-gateway",
		config: Config{Namespace: "acme", RequestTimeoutMs: 3000},
		client: tc.Client,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	c.server = c.buildServer()
	cs := connect(t, c)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := callTool(t, cs, tt.tool, tt.args)
			if res.IsError {
				t.Fatalf("%s returned a tool error: %+v", tt.tool, res)
			}
			if len(res.Content) == 0 {
				t.Fatalf("%s returned no content", tt.tool)
			}
			text, ok := res.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("%s returned %T, want TextContent", tt.tool, res.Content[0])
			}
			for _, want := range tt.wantText {
				if !strings.Contains(text.Text, want) {
					t.Fatalf("%s response missing %q: %s", tt.tool, want, text.Text)
				}
			}
		})
	}
}

func assertRequestFields(data []byte, want map[string]string) error {
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			return fmt.Errorf("request[%s] = %q, want %q; body=%s", key, got[key], wantValue, string(data))
		}
	}
	return nil
}

// TestIntegration_QueryToolsNameNotEnabledRefusal: with no search index the
// code-context and doc-context gateways refuse an nl query as not_enabled
// (#291). The MCP tools must carry that code into the tool error, so an agent
// can tell a permanent refusal from an outage it may retry.
//
// spec: openspec/specs/tier0-consumer-qualification/spec.md#Requirement: Tier 0 qualifies an exclusive no-embedder composition
func TestIntegration_QueryToolsNameNotEnabledRefusal(t *testing.T) {
	ctx := context.Background()
	tc := natstest.New(t)
	for _, subject := range []string{"code.v1.search", "docs.v1.context"} {
		sub, err := tc.Client.SubscribeForRequests(ctx, subject,
			func(context.Context, []byte) ([]byte, error) {
				return nil, errs.ClassifiedCode(errs.ErrorInvalid, semgraph.ErrorCodeNotEnabled,
					errors.New("natural-language search is not enabled in this composition"))
			})
		if err != nil {
			t.Fatalf("subscribe %s: %v", subject, err)
		}
		t.Cleanup(func() { _ = sub.Unsubscribe() })
	}

	c := &Component{
		name:   "mcp-gateway",
		config: Config{Namespace: "acme", RequestTimeoutMs: 3000},
		client: tc.Client,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	c.server = c.buildServer()
	cs := connect(t, c)

	for _, tool := range []string{"code_search", "doc_context"} {
		t.Run(tool, func(t *testing.T) {
			res := callTool(t, cs, tool, map[string]any{"query": "where are bodies kept"})
			if !res.IsError || len(res.Content) == 0 {
				t.Fatalf("%s: want a tool error, got %+v", tool, res)
			}
			text, ok := res.Content[0].(*mcp.TextContent)
			if !ok || !strings.Contains(text.Text, semgraph.ErrorCodeNotEnabled) {
				t.Fatalf("%s: tool error does not name %s: %+v", tool, semgraph.ErrorCodeNotEnabled, res.Content[0])
			}
		})
	}
}
