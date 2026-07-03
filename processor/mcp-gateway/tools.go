package mcpgateway

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourceallow"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
)

// AddSourceInput is the add_source tool's typed argument set. The SDK derives
// the tool's JSON Schema (with these descriptions) from the struct tags.
type AddSourceInput struct {
	Type     string `json:"type" jsonschema:"source type: repo (fans out to git+ast+docs+config), git, docs, config, or url"`
	Path     string `json:"path,omitempty" jsonschema:"local filesystem path to index in place (a mounted repo/worktree); must be under an allowlisted root"`
	URL      string `json:"url,omitempty" jsonschema:"remote URL (git remote or http url) — an alternative to path"`
	Branch   string `json:"branch,omitempty" jsonschema:"git branch or ref to track"`
	Language string `json:"language,omitempty" jsonschema:"primary code language (go, typescript, java, python, svelte)"`
	Watch    bool   `json:"watch,omitempty" jsonschema:"watch for live changes; omit for a one-shot snapshot (the default)"`
	Actor    string `json:"actor,omitempty" jsonschema:"caller identity, recorded as provenance"`
}

// RemoveSourceInput is the remove_source tool's argument set.
type RemoveSourceInput struct {
	InstanceName string `json:"instance_name" jsonschema:"the source handle (component instance name) returned by add_source"`
	Actor        string `json:"actor,omitempty" jsonschema:"caller identity, recorded as provenance"`
}

// SourceStatusInput is empty: source_status takes no arguments.
type SourceStatusInput struct{}

// addSource registers a source: it enforces the path allowlist at this external
// boundary, then forwards an AddRequest to source-manifest over NATS. Returns
// the AddReply JSON (handles + status_subject + ready_when) as tool text.
func (c *Component) addSource(ctx context.Context, _ *mcp.CallToolRequest, in AddSourceInput) (*mcp.CallToolResult, any, error) {
	if in.Type == "" {
		return nil, nil, fmt.Errorf("type is required")
	}
	src := config.SourceEntry{
		Type:     in.Type,
		Path:     in.Path,
		URL:      in.URL,
		Branch:   in.Branch,
		Language: in.Language,
		Watch:    in.Watch,
	}
	// Boundary guard: a path-based source must resolve under an allowlisted root.
	if err := sourceallow.Enforce(src, c.config.AllowedRoots); err != nil {
		return nil, nil, err
	}

	req := sourcemanifest.AddRequest{Source: src, Provenance: sourcemanifest.Provenance{Actor: in.Actor}}
	data, err := json.Marshal(&req)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal add request: %w", err)
	}
	resp, err := c.request(ctx, "graph.ingest.add."+c.config.Namespace, data)
	if err != nil {
		return nil, nil, fmt.Errorf("registration request failed: %w", err)
	}

	// Surface a registration-level failure (e.g. INSTANCE_EXISTS) as a tool error
	// so the agent sees it; otherwise return the reply verbatim.
	var reply sourcemanifest.AddReply
	if json.Unmarshal(resp, &reply) == nil && reply.Error != nil && len(reply.Components) == 0 {
		return nil, nil, fmt.Errorf("%s: %s", reply.Error.Code, reply.Error.Message)
	}
	return textResult(resp), nil, nil
}

// removeSource deregisters a source by its handle. Removal stops ingestion; it
// does NOT retract entities (ADR-0007 sequencing guardrail).
func (c *Component) removeSource(ctx context.Context, _ *mcp.CallToolRequest, in RemoveSourceInput) (*mcp.CallToolResult, any, error) {
	if in.InstanceName == "" {
		return nil, nil, fmt.Errorf("instance_name is required")
	}
	req := sourcemanifest.RemoveRequest{InstanceName: in.InstanceName, Provenance: sourcemanifest.Provenance{Actor: in.Actor}}
	data, err := json.Marshal(&req)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal remove request: %w", err)
	}
	resp, err := c.request(ctx, "graph.ingest.remove."+c.config.Namespace, data)
	if err != nil {
		return nil, nil, fmt.Errorf("remove request failed: %w", err)
	}
	var reply sourcemanifest.RemoveReply
	if json.Unmarshal(resp, &reply) == nil && reply.Error != nil {
		return nil, nil, fmt.Errorf("%s: %s", reply.Error.Code, reply.Error.Message)
	}
	return textResult(resp), nil, nil
}

// readinessNote warns callers that ingest phase precedes query-index
// completeness on a large one-shot ingest (semstreams#431).
const readinessNote = "phase can report 'ready' before the byName/search indexes finish " +
	"populating on a large one-shot ingest (semstreams#431). If code_context / code_impact / " +
	"code_search misses a symbol you expect, poll source_status until index.ready is true AND " +
	"total_entities has stopped climbing, then retry — a miss during that window may be the index " +
	"still building, not a genuine absence."

// sourceStatus reports graph readiness. It merges TWO distinct signals so a
// caller isn't misled by either alone: the source-manifest ingest phase
// (graph.query.status: phase + per-source counts + total_entities) and the
// graph-index NAME_INDEX readiness (graph.index.query.status: ready/state).
func (c *Component) sourceStatus(ctx context.Context, _ *mcp.CallToolRequest, _ SourceStatusInput) (*mcp.CallToolResult, any, error) {
	statusResp, err := c.request(ctx, "graph.query.status", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("status request failed: %w", err)
	}
	out := map[string]any{
		"status": json.RawMessage(statusResp),
		"note":   readinessNote,
	}
	// Best-effort: the graph-index's own NAME_INDEX readiness, distinct from the
	// ingest phase. Omitted (not fatal) if the index status is unavailable.
	if idxResp, idxErr := c.request(ctx, "graph.index.query.status", nil); idxErr == nil && len(idxResp) > 0 {
		out["index"] = json.RawMessage(idxResp)
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal status: %w", err)
	}
	return textResult(data), nil, nil
}

// textResult wraps raw JSON bytes as the tool's text content.
func textResult(raw []byte) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}
}
