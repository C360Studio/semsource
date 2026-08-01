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

// GraphSummaryInput is empty: graph_summary takes no arguments. The substrate
// applies its own sample-limit and predicate defaults on an empty request body.
type GraphSummaryInput struct{}

// GraphSearchInput is the graph_search argument: a natural-language question
// about the corpus as a whole, not a symbol name.
type GraphSearchInput struct {
	Query string `json:"query" jsonschema:"a natural-language question about the indexed corpus as a whole (e.g. 'how does readiness gating work'), not a symbol name"`
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

// graphSummary returns the substrate's graph overview: entity-type counts with
// examples plus predicate summary data. It depends on no optional capability —
// the subject is a discovery resolver over prefix + predicate-list queries — so
// it answers identically at every tier.
//
// Pure passthrough, deliberately: there is no retrieval path here to disclose,
// and attaching a disclosure would imply a choice of path that was never made.
//
// This routes to graph.query.summary, which SemSource's own source-manifest used
// to subscribe to as well. That collision is why this tool was withdrawn from
// expose-graphrag-on-mcp; source-manifest has since moved to
// graph.query.sourceSummary and the subject now has exactly one handler.
func (c *Component) graphSummary(ctx context.Context, _ *mcp.CallToolRequest, _ GraphSummaryInput) (*mcp.CallToolResult, any, error) {
	resp, err := c.request(ctx, "graph.query.summary", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("graph summary failed: %w", err)
	}
	return textResult(resp), nil, nil
}

// graphSearch answers a corpus-wide thematic question. It routes to
// graph.query.searchGraph, which delegates to globalSearch and returns that
// response unchanged when non-empty, adding a labelled semantic fallback only
// when it is empty — so this one subject covers both.
//
// The result is a RANKED, BOUNDED match list plus a disclosure of the rung the
// answer reached. It is deliberately not the substrate payload verbatim: on a
// live 119-entity corpus a 38-hit query returned 197 KB of entity triples whose
// only "content" was a body-by-reference key — 11x the cost of code_search for
// an answer an agent cannot read. A search verb's job is to rank so the agent
// can follow up on the IDs worth reading.
//
// summarize_threshold asks the substrate for the compact shape. It is honored on
// the graphrag strategy and ignored on the semantic one, which is why
// deriveMatches also reconstructs the list from whatever the response carried.
func (c *Component) graphSearch(ctx context.Context, _ *mcp.CallToolRequest, in GraphSearchInput) (*mcp.CallToolResult, any, error) {
	if in.Query == "" {
		return nil, nil, fmt.Errorf("query is required")
	}
	data, err := json.Marshal(map[string]any{"query": in.Query, "summarize_threshold": 1})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal query: %w", err)
	}
	resp, err := c.request(ctx, "graph.query.searchGraph", data)
	if err != nil {
		return nil, nil, fmt.Errorf("graph search failed: %w", err)
	}

	var body graphSearchBody
	if err := json.Unmarshal(resp, &body); err != nil {
		// Unreadable payload: report the disclosure's unknown rung rather than
		// failing the call, and say nothing about matches we cannot see.
		out, mErr := json.Marshal(graphSearchResult{Retrieval: deriveDisclosure(resp)})
		if mErr != nil {
			return nil, nil, fmt.Errorf("marshal graph search result: %w", mErr)
		}
		return textResult(out), nil, nil
	}

	matches, total, truncated := deriveMatches(&body)
	out, err := json.Marshal(graphSearchResult{
		Retrieval:          deriveDisclosure(resp),
		Answer:             body.Answer,
		Matches:            matches,
		TotalMatches:       total,
		Truncated:          truncated,
		CommunitySummaries: body.CommunitySummaries,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal graph search result: %w", err)
	}
	return textResult(out), nil, nil
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
