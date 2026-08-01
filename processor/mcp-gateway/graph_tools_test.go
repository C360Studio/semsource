package mcpgateway

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestGraphSearchRequiresAQuery: argument validation happens before the NATS
// request, so the caller gets a usable message instead of a transport failure.
func TestGraphSearchRequiresAQuery(t *testing.T) {
	cs := connect(t, newTestComponent(nil))

	res := callTool(t, cs, "graph_search", map[string]any{"query": ""})
	if !res.IsError {
		t.Fatalf("empty query was accepted: %+v", res)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "query is required") {
		t.Errorf("error does not name the missing argument: %s", text)
	}
}

// TestGraphToolDescriptionsStayHonest pins the description contract. These tools
// are reachable on stacks that provide neither clustering nor an LLM, so a
// description that promises community reasoning or query understanding
// unconditionally is a claim the running configuration may not be able to keep.
func TestGraphToolDescriptionsStayHonest(t *testing.T) {
	cs := connect(t, newTestComponent(nil))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	descriptions := make(map[string]string, len(res.Tools))
	for _, tool := range res.Tools {
		descriptions[tool.Name] = tool.Description
	}

	search, ok := descriptions["graph_search"]
	if !ok {
		t.Fatal("graph_search is not registered")
	}
	// It must advertise the disclosure, or an agent has no reason to read it.
	for _, want := range []string{"retrieval", "not guaranteed"} {
		if !strings.Contains(search, want) {
			t.Errorf("graph_search description missing %q: %s", want, search)
		}
	}
	// It must draw the line against code_search, which is the tool an agent
	// would otherwise reach for with the same query.
	if !strings.Contains(search, "code_search") {
		t.Errorf("graph_search description does not distinguish itself from code_search: %s", search)
	}
}
