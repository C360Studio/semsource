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

	"github.com/c360studio/semstreams/natsclient"

	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
)

// TestIntegration_AddSourceTranslatesToNATS proves the gateway's core job: an MCP
// add_source tool call becomes a NATS request on graph.ingest.add.{namespace},
// and the reply is returned to the caller. A stub responder stands in for
// source-manifest so the test isolates the translation.
func TestIntegration_AddSourceTranslatesToNATS(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t)

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
// (graph.query.status), the graph-index structural readiness
// (graph.index.query.status), and the graph-embedding semantic readiness
// (graph.embedding.query.status) — plus the note.
func TestIntegration_SourceStatusMergesSignals(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t)

	sub1, err := tc.Client.SubscribeForRequests(ctx, "graph.query.status",
		func(_ context.Context, _ []byte) ([]byte, error) {
			return []byte(`{"namespace":"ss","phase":"ready","total_entities":42}`), nil
		})
	if err != nil {
		t.Fatalf("subscribe status: %v", err)
	}
	t.Cleanup(func() { _ = sub1.Unsubscribe() })

	sub2, err := tc.Client.SubscribeForRequests(ctx, "graph.index.query.status",
		func(_ context.Context, _ []byte) ([]byte, error) {
			return []byte(`{"ready":false,"state":"building"}`), nil
		})
	if err != nil {
		t.Fatalf("subscribe index status: %v", err)
	}
	t.Cleanup(func() { _ = sub2.Unsubscribe() })

	sub3, err := tc.Client.SubscribeForRequests(ctx, "graph.embedding.query.status",
		func(_ context.Context, _ []byte) ([]byte, error) {
			return []byte(`{"ready":true,"state":"ready"}`), nil
		})
	if err != nil {
		t.Fatalf("subscribe embedding status: %v", err)
	}
	t.Cleanup(func() { _ = sub3.Unsubscribe() })

	c := &Component{
		name:   "mcp-gateway",
		config: Config{Namespace: "ss", RequestTimeoutMs: 3000},
		client: tc.Client,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
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
