package mcpgateway

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// QueryInput is the shared argument for the fused query tools. The per-tool
// Description tells the agent what kind of query to pass (symbol name vs. natural
// language); the field itself is a free-form query string.
type QueryInput struct {
	Query string `json:"query" jsonschema:"the query — a symbol name (e.g. registerProvidedStores) for context/impact, or a natural-language phrase for search"`
}

// ChangesInput is the argument for the code_changes tool: a project (source
// identity) and two version identifiers to compare.
type ChangesInput struct {
	Project string `json:"project" jsonschema:"the project / source identity (code.artifact.project, e.g. a module path or repo slug)"`
	From    string `json:"from" jsonschema:"the older version to compare from (code.artifact.version, e.g. 1.9.0)"`
	To      string `json:"to" jsonschema:"the newer version to compare to (code.artifact.version, e.g. 1.10.0)"`
}

// codeChanges returns the symbol-level changeset between two versions of a
// project: added / removed / changed / unchanged (counted) symbols, with verbatim
// before/after bodies for changed ones. Forwards to graph.query.versionDiff.
func (c *Component) codeChanges(ctx context.Context, _ *mcp.CallToolRequest, in ChangesInput) (*mcp.CallToolResult, any, error) {
	if in.Project == "" || in.From == "" || in.To == "" {
		return nil, nil, fmt.Errorf("project, from, and to are required")
	}
	data, err := json.Marshal(map[string]string{"project": in.Project, "from": in.From, "to": in.To})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}
	resp, err := c.request(ctx, "graph.query.versionDiff", data)
	if err != nil {
		return nil, nil, fmt.Errorf("version diff failed: %w", err)
	}
	return textResult(resp), nil, nil
}

// codeContext returns the fused code answer: the resolved symbol, its verbatim
// body, and its callers/callees — the primary "show me this code and how it
// connects" query.
func (c *Component) codeContext(ctx context.Context, _ *mcp.CallToolRequest, in QueryInput) (*mcp.CallToolResult, any, error) {
	return c.fusionQuery(ctx, "code.v1.context", in.Query)
}

// codeImpact returns the reverse-dependency closure of a symbol: what depends on
// it, i.e. what would break if you change it. The query grep cannot answer.
func (c *Component) codeImpact(ctx context.Context, _ *mcp.CallToolRequest, in QueryInput) (*mcp.CallToolResult, any, error) {
	return c.fusionQuery(ctx, "code.v1.impact", in.Query)
}

// codeSearch does semantic/natural-language discovery over the indexed code —
// "where is the retry-with-backoff logic" — returning matching symbols + bodies.
func (c *Component) codeSearch(ctx context.Context, _ *mcp.CallToolRequest, in QueryInput) (*mcp.CallToolResult, any, error) {
	return c.fusionQuery(ctx, "code.v1.search", in.Query)
}

// docContext returns fused documentation context (READMEs/ADRs/prose) for a
// query — the intended design, not just the code.
func (c *Component) docContext(ctx context.Context, _ *mcp.CallToolRequest, in QueryInput) (*mcp.CallToolResult, any, error) {
	return c.fusionQuery(ctx, "docs.v1.context", in.Query)
}

// fusionQuery sends a fusion.Request ({"query": ...}) to a code-context /
// doc-context verb subject and returns the fusion.Response JSON verbatim. Each
// verb applies its own default facet set server-side when Want is omitted.
func (c *Component) fusionQuery(ctx context.Context, subject, query string) (*mcp.CallToolResult, any, error) {
	if query == "" {
		return nil, nil, fmt.Errorf("query is required")
	}
	data, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal query: %w", err)
	}
	resp, err := c.request(ctx, subject, data)
	if err != nil {
		return nil, nil, fmt.Errorf("fusion query failed: %w", err)
	}
	return textResult(resp), nil, nil
}
