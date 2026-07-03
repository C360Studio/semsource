// Package mcpgateway is the semsource MCP gateway: a Streamable-HTTP Model
// Context Protocol server (official github.com/modelcontextprotocol/go-sdk) that
// exposes semsource's source-registration tools to agents (Claude Code and
// others). External consumers reach semsource over HTTP/MCP, never NATS
// (ADR-0007) — the gateway is an in-mesh translator: MCP tool calls become NATS
// request/reply to source-manifest.
package mcpgateway

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// Config configures the MCP gateway. The API token is NOT here — it is read from
// the SEMSOURCE_API_TOKEN environment variable at construction so the secret
// never lands in the config KV store.
type Config struct {
	Ports *component.PortConfig `json:"ports,omitempty" schema:"type:ports,description:Port configuration,category:basic"`

	// Namespace routes registration tool calls to graph.ingest.add/remove.{namespace}.
	Namespace string `json:"namespace" schema:"type:string,description:Organization namespace for registration subjects,category:basic,required:true"`

	// MCPPath is the HTTP path the Streamable-HTTP MCP endpoint mounts at,
	// relative to the component's instance prefix (e.g. /mcp-gateway + /mcp).
	MCPPath string `json:"mcp_path,omitempty" schema:"type:string,description:MCP endpoint path,category:basic,default:/mcp"`

	// AllowedRoots is the filesystem-root allowlist for path-based add_source
	// (ADR-0007 §3). Empty rejects path-based registration over MCP.
	AllowedRoots []string `json:"allowed_roots,omitempty" schema:"type:array,description:Allowlisted filesystem roots for path-based source registration,category:advanced"`

	// RequestTimeoutMs bounds each NATS request/reply a tool makes.
	RequestTimeoutMs int `json:"request_timeout_ms,omitempty" schema:"type:int,description:NATS request timeout in ms,category:advanced,default:10000"`

	// InstanceName is the unique component instance name.
	InstanceName string `json:"instance_name,omitempty" schema:"type:string,description:Unique component instance name,category:internal"`
}

// Validate checks required fields.
func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	return nil
}

// DefaultConfig returns the default MCP gateway configuration. The timeout is
// generous because a fused query traverses the graph (many bounded round-trips);
// registration replies are fast, so the ceiling is harmless there.
func DefaultConfig() Config {
	return Config{
		MCPPath:          "/mcp",
		RequestTimeoutMs: 30000,
	}
}
