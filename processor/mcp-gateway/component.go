package mcpgateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
)

const serverVersion = "0.1.0"

// Component is the semsource MCP gateway. It serves a Streamable-HTTP MCP
// endpoint whose tools translate into NATS request/reply against source-manifest.
type Component struct {
	name     string
	config   Config
	client   *natsclient.Client
	apiToken string
	server   *mcp.Server
	logger   *slog.Logger

	mu        sync.RWMutex
	running   bool
	startTime time.Time
}

var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// NewComponent builds the MCP gateway, reading the bearer token from
// SEMSOURCE_API_TOKEN (kept out of the config KV).
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	cfg := DefaultConfig()
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &cfg); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w", err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	c := &Component{
		name:     "mcp-gateway",
		config:   cfg,
		client:   deps.NATSClient,
		apiToken: os.Getenv("SEMSOURCE_API_TOKEN"),
		logger:   deps.GetLogger(),
	}
	c.server = c.buildServer()
	return c, nil
}

// buildServer constructs the MCP server and registers the registration tools.
// The SDK derives each tool's input schema from its typed argument struct.
func (c *Component) buildServer() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "semsource", Version: serverVersion}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_source",
		Description: "Register a source (repo/git/docs/config/url) for semsource to index. Path-based sources must be under an allowlisted root. Returns handles + a readiness condition to poll.",
	}, c.addSource)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "remove_source",
		Description: "Deregister a source by its handle (instance name). Stops ingestion; existing graph data is not retracted.",
	}, c.removeSource)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "source_status",
		Description: "Report semsource graph readiness: the source-manifest ingest phase + per-source counts, the graph-index structural readiness (index), and the graph-embedding semantic readiness (embedding), plus a note. Readiness is honest (semstreams ADR-066): status.phase 'ready' means every configured source finished its initial seed, and index/embedding 'ready' mean caught up to the latest committed write. Gate structural queries (byName / code_context / code_impact) on phase ready AND index.ready; code_search reliability tracks embedding.ready. A miss is a genuine absence only once both gates hold.",
	}, c.sourceStatus)

	// Fused query tools (ADR-0004 read side over MCP). Readiness is honest
	// (ADR-066 + the seeded-phase gate): a miss is a genuine absence only once
	// status.phase is ready AND the relevant source_status signal is ready.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "code_context",
		Description: "Fused code answer for a symbol: the resolved definition, its verbatim source body, and its callers/callees. Use to understand a symbol and how it connects. If a symbol misses while index.ready is false (check source_status), the structural index is still catching up — retry once ready.",
	}, c.codeContext)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "code_impact",
		Description: "Reverse-dependency closure of a symbol — what depends on it, i.e. what would break if you change it. Answers questions grep cannot. If it misses while index.ready is false (check source_status), the structural index is still catching up — retry once ready.",
	}, c.codeImpact)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "code_search",
		Description: "Semantic / natural-language search over indexed code (e.g. 'where is the retry-with-backoff logic'). Returns matching symbols with bodies. Quality depends on the embedder tier; results are reliable once embedding.ready is true (check source_status).",
	}, c.codeSearch)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "doc_context",
		Description: "Fused documentation context (READMEs/ADRs/prose) for a query — the intended design, not just the code.",
	}, c.docContext)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "code_changes",
		Description: "What changed between two versions of a source: added / removed / changed / unchanged symbols (counts), with verbatim before/after bodies for changed symbols. Args: project, from, to (versions). A renamed symbol shows as a removed + an added, not a single change.",
	}, c.codeChanges)
	return s
}

// Initialize is a no-op (setup happens in NewComponent).
func (c *Component) Initialize() error { return nil }

// Start marks the gateway running. The Streamable-HTTP handler serves per-request
// sessions, so there is no long-lived loop to launch.
func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return fmt.Errorf("component already running")
	}
	c.running = true
	c.startTime = time.Now()
	c.logger.Info("mcp-gateway started")
	return nil
}

// Stop marks the gateway stopped.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		c.running = false
		c.logger.Info("mcp-gateway stopped")
	}
	return nil
}

// RegisterHTTPHandlers mounts the Streamable-HTTP MCP endpoint (behind the bearer
// auth seam) on the ServiceManager's shared mux. The endpoint path is the
// component prefix + MCPPath (e.g. /mcp-gateway/mcp).
func (c *Component) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return c.server }, nil)
	path := joinPath(prefix, c.config.MCPPath)
	mux.Handle(path, c.authMiddleware(handler))
	c.logger.Info("registered MCP endpoint", "path", path, "auth", c.apiToken != "")
}

// authMiddleware enforces the optional bearer token (ADR-0007 §6). Permissive
// when no token is configured.
func (c *Component) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r, c.apiToken) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// request performs a NATS request/reply with the configured timeout.
// RequestClassified (not plain Request) so ADR-060 handler-error replies
// (X-Error-Class/X-Error-Code + {message, detail} body) surface as Go errors
// and therefore as MCP isError results — with plain Request an infrastructure
// failure arrived as a SUCCESSFUL tool answer, detectable only by the missing
// contract_version (audit 2026-07-19, honest-readiness-and-errors).
func (c *Component) request(ctx context.Context, subject string, data []byte) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("no NATS client")
	}
	return c.client.RequestClassified(ctx, subject, data, c.timeout())
}

func (c *Component) timeout() time.Duration {
	if c.config.RequestTimeoutMs > 0 {
		return time.Duration(c.config.RequestTimeoutMs) * time.Millisecond
	}
	return 10 * time.Second
}

// authorized enforces the optional bearer-token seam; empty token = permissive.
// Constant-time compare.
func authorized(r *http.Request, token string) bool {
	if token == "" {
		return true
	}
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return false
	}
	got := strings.TrimPrefix(h, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

// joinPath combines the instance prefix and the MCP path into a clean route.
func joinPath(prefix, mcpPath string) string {
	p := "/" + strings.Trim(prefix, "/") + "/" + strings.Trim(mcpPath, "/")
	return strings.ReplaceAll(p, "//", "/")
}

// --- Discoverable ---------------------------------------------------------

// Meta implements component.Discoverable.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        c.name,
		Type:        "processor",
		Description: "MCP gateway: source-registration tools over Streamable HTTP for AI agents",
		Version:     serverVersion,
	}
}

// InputPorts implements component.Discoverable (none — request/reply + HTTP).
func (c *Component) InputPorts() []component.Port { return []component.Port{} }

// OutputPorts implements component.Discoverable (none — request/reply + HTTP).
func (c *Component) OutputPorts() []component.Port { return []component.Port{} }

// ConfigSchema implements component.Discoverable.
func (c *Component) ConfigSchema() component.ConfigSchema { return mcpGatewaySchema }

// Health implements component.Discoverable.
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	running := c.running
	startTime := c.startTime
	c.mu.RUnlock()
	status := "stopped"
	if running {
		status = "running"
	}
	return component.HealthStatus{
		Healthy:   running,
		LastCheck: time.Now(),
		Uptime:    time.Since(startTime),
		Status:    status,
	}
}

// DataFlow implements component.Discoverable.
func (c *Component) DataFlow() component.FlowMetrics { return component.FlowMetrics{} }
