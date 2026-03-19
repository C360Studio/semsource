// Package config loads and validates the semsource.json configuration file.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Mode constants for semsource operation.
const (
	// ModeStandalone runs the full graph subsystem (ingest, index, embedding,
	// query, gateway) and WebSocket output. This is the default.
	ModeStandalone = "standalone"

	// ModeHeadless runs source components only. Entities are published to
	// graph.ingest.entity for consumption by a host app (SemSpec, SemDragon)
	// that manages its own graph infrastructure. No graph processing, no
	// WebSocket server, no GraphQL gateway.
	ModeHeadless = "headless"
)

// EntityStoreConfig configures persistent graph storage via NATS KV.
// When present, entities are persisted to the shared ENTITY_STATES bucket.
// When absent, entities are stored in-memory only.
type EntityStoreConfig struct {
	// NATSUrl is the NATS server URL (e.g., "nats://localhost:4222").
	NATSUrl string `json:"nats_url"`
}

// GraphConfig configures graph subsystem components (standalone mode only).
// In headless mode these settings are ignored.
type GraphConfig struct {
	// GatewayBind is the bind address for the GraphQL gateway. Defaults to "0.0.0.0:8082".
	GatewayBind string `json:"gateway_bind,omitempty"`

	// EnablePlayground enables the GraphQL playground UI. Defaults to true.
	EnablePlayground *bool `json:"enable_playground,omitempty"`

	// EmbedderType is the embedding algorithm ("bm25" or "http"). Defaults to "bm25".
	EmbedderType string `json:"embedder_type,omitempty"`

	// EmbeddingBatchSize is the batch size for embedding generation. Defaults to 50.
	EmbeddingBatchSize int `json:"embedding_batch_size,omitempty"`

	// CoalesceMs is the debounce window in ms for graph-index and graph-embedding.
	// Defaults to 200.
	CoalesceMs int `json:"coalesce_ms,omitempty"`
}

// MetricsConfig configures the Prometheus metrics endpoint.
type MetricsConfig struct {
	// Port is the Prometheus scrape port. Defaults to 9091.
	Port int `json:"port,omitempty"`

	// Path is the metrics endpoint path. Defaults to "/metrics".
	Path string `json:"path,omitempty"`
}

// StreamOverride allows overriding JetStream stream configuration.
type StreamOverride struct {
	Storage  string `json:"storage,omitempty"`
	MaxBytes *int64 `json:"max_bytes,omitempty"`
	MaxAge   string `json:"max_age,omitempty"`
	Replicas *int   `json:"replicas,omitempty"`
}

// Config is the top-level semsource configuration.
type Config struct {
	// Namespace is the org identifier used in entity ID construction (e.g., "acme").
	Namespace string `json:"namespace"`

	// Sources lists all ingestion sources.
	Sources []SourceEntry `json:"sources"`

	// EntityStore configures persistent graph storage.
	// When set, entities are persisted to the NATS KV ENTITY_STATES bucket.
	EntityStore *EntityStoreConfig `json:"entity_store,omitempty"`

	// WorkspaceDir is the base directory where remote git repositories are
	// cloned. Defaults to ~/.semsource/repos when empty.
	WorkspaceDir string `json:"workspace_dir,omitempty"`

	// GitToken is a personal access token or GitHub App installation token
	// for authenticating HTTPS clones of private repositories.
	// Can also be set via the SEMSOURCE_GIT_TOKEN environment variable.
	GitToken string `json:"git_token,omitempty"`

	// MediaStoreDir is the root directory used by media source components
	// (image, video, audio) to store binary content on the local filesystem.
	// When empty, media processors operate in metadata-only mode.
	MediaStoreDir string `json:"media_store_dir,omitempty"`

	// HTTPPort is the port for the ServiceManager HTTP API server.
	// Can also be set via the SEMSOURCE_HTTP_PORT environment variable.
	// Defaults to 8080.
	HTTPPort int `json:"http_port,omitempty"`

	// WebSocketBind is the host:port for the WebSocket output server.
	// Can also be set via the SEMSOURCE_WS_BIND environment variable.
	// Defaults to "0.0.0.0:7890".
	WebSocketBind string `json:"websocket_bind,omitempty"`

	// WebSocketPath is the URL path for the WebSocket endpoint.
	// Can also be set via the SEMSOURCE_WS_PATH environment variable.
	// Defaults to "/graph".
	WebSocketPath string `json:"websocket_path,omitempty"`

	// Mode controls semsource operation: "standalone" (default) runs the full
	// graph subsystem; "headless" publishes entities only for a host app.
	// Can also be set via the SEMSOURCE_MODE environment variable.
	Mode string `json:"mode,omitempty"`

	// Graph configures graph subsystem components (standalone mode only).
	Graph *GraphConfig `json:"graph,omitempty"`

	// Metrics configures the Prometheus metrics endpoint.
	Metrics *MetricsConfig `json:"metrics,omitempty"`

	// Streams allows overriding JetStream stream configurations.
	// Keys are stream names (e.g., "GRAPH").
	Streams map[string]StreamOverride `json:"streams,omitempty"`
}

// applyDefaults fills in omitted fields with their documented defaults.
func (c *Config) applyDefaults() {
	if c.WorkspaceDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			c.WorkspaceDir = filepath.Join(home, ".semsource", "repos")
		}
	}
	// Allow token to be set via environment variable (avoids putting secrets in config files).
	if c.GitToken == "" {
		c.GitToken = os.Getenv("SEMSOURCE_GIT_TOKEN")
	}
	// WebSocket bind: env var takes precedence, then config, then default.
	if v := os.Getenv("SEMSOURCE_WS_BIND"); v != "" {
		c.WebSocketBind = v
	}
	if c.WebSocketBind == "" {
		c.WebSocketBind = "0.0.0.0:7890"
	}
	if v := os.Getenv("SEMSOURCE_WS_PATH"); v != "" {
		c.WebSocketPath = v
	}
	if c.WebSocketPath == "" {
		c.WebSocketPath = "/graph"
	}
	// HTTP API port: env var takes precedence, then config, then default.
	if v := os.Getenv("SEMSOURCE_HTTP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.HTTPPort = p
		}
	}
	if c.HTTPPort == 0 {
		c.HTTPPort = 8080
	}
	// Mode: env var takes precedence, then config, then default.
	if v := os.Getenv("SEMSOURCE_MODE"); v != "" {
		c.Mode = v
	}
	if c.Mode == "" {
		c.Mode = ModeStandalone
	}
}

// IsHeadless returns true when semsource is running in headless mode.
func (c *Config) IsHeadless() bool {
	return c.Mode == ModeHeadless
}

// Validate checks that all required fields are present and each source is valid.
func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("config: namespace is required")
	}
	if len(c.Sources) == 0 {
		return fmt.Errorf("config: sources must contain at least one source")
	}
	if c.Mode != "" && c.Mode != ModeStandalone && c.Mode != ModeHeadless {
		return fmt.Errorf("config: mode must be %q or %q, got %q", ModeStandalone, ModeHeadless, c.Mode)
	}

	for i, src := range c.Sources {
		if err := src.Validate(); err != nil {
			return fmt.Errorf("config: sources[%d]: %w", i, err)
		}
	}

	return nil
}
