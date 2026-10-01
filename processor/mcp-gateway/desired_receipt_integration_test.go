//go:build integration

package mcpgateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semsource/processor/source-manifest"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestIntegrationRemovePreservesPartialDesiredReceipt(t *testing.T) {
	tc := natsclient.NewTestClient(t)
	sub, err := tc.Client.SubscribeForRequests(t.Context(), "graph.ingest.remove.acme", func(_ context.Context, _ []byte) ([]byte, error) {
		return json.Marshal(sourcemanifest.RemoveReply{InstanceName: "docs", Removed: true, DesiredChanged: true, RestartRequired: true, ProjectionStatus: "unavailable", Error: &sourcemanifest.IngestError{Code: sourcemanifest.CodeKVWriteFailed, Message: "manifest write failed"}})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sub.Drain(context.Background()); err != nil {
			t.Error(err)
		}
	})
	c := newTestComponent(nil)
	c.client = tc.Client
	res := callTool(t, connect(t, c), "remove_source", map[string]any{"instance_name": "docs"})
	if res.IsError || len(res.Content) != 1 {
		t.Fatalf("lost committed receipt: %+v", res)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	var reply sourcemanifest.RemoveReply
	if err := json.Unmarshal([]byte(text), &reply); err != nil {
		t.Fatal(err)
	}
	if !reply.DesiredChanged || !reply.RestartRequired || reply.RuntimeChanged || reply.ProjectionStatus != "unavailable" || reply.Error == nil || !strings.Contains(reply.Error.Message, "manifest") {
		t.Fatalf("reply=%s", text)
	}
}
