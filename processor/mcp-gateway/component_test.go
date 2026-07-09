package mcpgateway

import (
	"context"
	"io"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var wantToolNames = []string{
	"add_source", "code_changes", "code_context", "code_impact", "code_search",
	"doc_context", "remove_source", "source_status",
}

func newTestComponent(roots []string) *Component {
	c := &Component{
		name:   "mcp-gateway",
		config: Config{Namespace: "acme", AllowedRoots: roots, RequestTimeoutMs: 1000},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	c.server = c.buildServer()
	return c
}

// connect wires an in-memory MCP client to the component's server.
func connect(t *testing.T, c *Component) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	clientT, serverT := mcp.NewInMemoryTransports()
	if _, err := c.server.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestListTools(t *testing.T) {
	cs := connect(t, newTestComponent(nil))
	got := listToolNames(t, cs)
	if !sameStrings(got, wantToolNames) {
		t.Fatalf("tools = %v, want %v", got, wantToolNames)
	}
}

func TestREADMEAdvertisedMCPToolsAreRegistered(t *testing.T) {
	cs := connect(t, newTestComponent(nil))
	registered := make(map[string]bool)
	for _, name := range listToolNames(t, cs) {
		registered[name] = true
	}

	readme := readRepoFile(t, "README.md")
	section := between(t, readme, "### Connect an agent (MCP)", "## Config File")
	advertised := backtickIdentifiers(section)
	if len(advertised) == 0 {
		t.Fatal("README Connect an agent section did not advertise any MCP tools")
	}
	for _, name := range advertised {
		if !registered[name] {
			t.Fatalf("README advertises MCP tool %q, but registered tools are %v", name, listToolNames(t, cs))
		}
	}
}

func TestMCPQuickstartToolTableMatchesRegisteredTools(t *testing.T) {
	cs := connect(t, newTestComponent(nil))
	quickstart := readRepoFile(t, "docs/integration/mcp-quickstart.md")
	got := mcpQuickstartToolNames(quickstart)
	if !sameStrings(got, listToolNames(t, cs)) {
		t.Fatalf("quickstart tools = %v, registered tools = %v", got, listToolNames(t, cs))
	}
}

func listToolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		got = append(got, tool.Name)
		if tool.Description == "" {
			t.Errorf("tool %q has no description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has no input schema", tool.Name)
		}
	}
	sort.Strings(got)
	return got
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile("../../" + path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func between(t *testing.T, body, start, end string) string {
	t.Helper()
	startIdx := strings.Index(body, start)
	if startIdx < 0 {
		t.Fatalf("missing section start %q", start)
	}
	body = body[startIdx+len(start):]
	endIdx := strings.Index(body, end)
	if endIdx < 0 {
		t.Fatalf("missing section end %q", end)
	}
	return body[:endIdx]
}

func backtickIdentifiers(body string) []string {
	matches := regexp.MustCompile("`([a-z]+_[a-z]+)`").FindAllStringSubmatch(body, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if !seen[match[1]] {
			seen[match[1]] = true
			out = append(out, match[1])
		}
	}
	sort.Strings(out)
	return out
}

func mcpQuickstartToolNames(body string) []string {
	matches := regexp.MustCompile(`(?m)^\| `+"`"+`([a-z]+_[a-z]+)`+"`").FindAllStringSubmatch(body, -1)
	tools := make([]string, 0, len(matches))
	for _, match := range matches {
		tools = append(tools, match[1])
	}
	sort.Strings(tools)
	return tools
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestQueryToolRequiresQuery(t *testing.T) {
	cs := connect(t, newTestComponent(nil))
	for _, tool := range []string{"code_context", "code_impact", "code_search", "doc_context"} {
		res := callTool(t, cs, tool, map[string]any{})
		if !res.IsError {
			t.Errorf("%s: expected tool error when query is missing", tool)
		}
	}
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s (protocol error): %v", name, err)
	}
	return res
}

// The boundary guards (allowlist, required fields) short-circuit before any NATS
// call, so they exercise the tools with a nil client and surface as tool errors.
func TestAddSource_AllowlistRejectsPath(t *testing.T) {
	cs := connect(t, newTestComponent([]string{"/mnt/workspace"}))
	res := callTool(t, cs, "add_source", map[string]any{"type": "git", "path": "/etc/secrets"})
	if !res.IsError {
		t.Fatalf("expected tool error for path outside allowlist; got %+v", res)
	}
}

func TestAddSource_RequiresType(t *testing.T) {
	cs := connect(t, newTestComponent([]string{"/mnt/workspace"}))
	res := callTool(t, cs, "add_source", map[string]any{"path": "/mnt/workspace/repo"})
	if !res.IsError {
		t.Fatal("expected tool error when type is missing")
	}
}

func TestRemoveSource_RequiresInstance(t *testing.T) {
	cs := connect(t, newTestComponent(nil))
	res := callTool(t, cs, "remove_source", map[string]any{})
	if !res.IsError {
		t.Fatal("expected tool error when instance_name is missing")
	}
}
