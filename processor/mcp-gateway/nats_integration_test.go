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
