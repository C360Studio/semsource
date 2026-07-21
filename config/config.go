// Package config loads and validates the semsource.json configuration file.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semstreams/model"
	semtypes "github.com/c360studio/semstreams/pkg/types"
)

// EntityStoreConfig configures persistent graph storage via NATS KV.
// When present, entities are persisted to the shared ENTITY_STATES bucket.
// When absent, entities are stored in-memory only.
type EntityStoreConfig struct {
	// NATSUrl is the NATS server URL (e.g., "nats://localhost:4222").
	NATSUrl string `json:"nats_url"`
}

// GraphConfig configures graph subsystem components.
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

	// IndexWorkers is the number of graph-index worker goroutines. 0 uses the
	// semstreams default (1), which is a throughput bottleneck for large one-shot
	// ingests (indexing a whole dependency) — raise it to parallelize index build.
	IndexWorkers int `json:"index_workers,omitempty"`

	// EnableClustering wires the graph-clustering component (LPA community
	// detection over ENTITY_STATES → COMMUNITY_INDEX), lighting up the
	// already-declared local/global/summary query routes (tier 2). Off by
	// default: without it those routes return empty. Pure-Go LPA needs no
	// external service; LLM community summaries additionally require
	// ClusteringLLM + a model_registry "community_summary" capability.
	EnableClustering bool `json:"enable_clustering,omitempty"`

	// ClusteringLLM turns on LLM-based community summarization in
	// graph-clustering (GraphRAG summaries). Requires EnableClustering and a
	// model_registry with a "community_summary" capability (→ seminstruct).
	// Ignored when EnableClustering is false.
	ClusteringLLM bool `json:"clustering_llm,omitempty"`
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

	// SourceRoots is the allowlist of filesystem roots under which a path-based
	// source may be registered over the HTTP façade (ADR-0007 sidecar). A
	// path-only git/repo/docs/config add over HTTP must resolve to a path under
	// one of these roots (with a traversal guard) — arbitrary host paths are
	// rejected. Empty means path-based HTTP registration is refused. Does NOT
	// constrain the in-mesh NATS path or config-file sources (operator-trusted).
	SourceRoots []string `json:"source_roots,omitempty"`

	// WebSocketBind is the host:port for the WebSocket output server.
	// Can also be set via the SEMSOURCE_WS_BIND environment variable.
	// Defaults to "0.0.0.0:7890".
	WebSocketBind string `json:"websocket_bind,omitempty"`

	// WebSocketPath is the URL path for the WebSocket endpoint.
	// Can also be set via the SEMSOURCE_WS_PATH environment variable.
	// Defaults to "/graph".
	WebSocketPath string `json:"websocket_path,omitempty"`

	// Graph configures graph subsystem components.
	Graph *GraphConfig `json:"graph,omitempty"`

	// ModelRegistry is the semstreams unified model-endpoint registry, passed
	// straight through to the ServiceManager (ssCfg.ModelRegistry) so
	// graph-embedding's HTTP embedder (tier 1, semembed) and graph-clustering's
	// LLM summarizer (tier 2, seminstruct) can resolve their endpoints by
	// capability ("embedding" / "community_summary"). Reuses the semstreams
	// model.Registry type directly — no bespoke shape. Nil = tier 0 (BM25), no
	// external model services. See configs/tiers/ and docs.
	ModelRegistry *model.Registry `json:"model_registry,omitempty"`

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
}

// Validate checks that all required fields are present and each source is valid.
func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("config: namespace is required")
	}
	if err := validateNamespaceSegment(c.Namespace); err != nil {
		return err
	}
	if len(c.Sources) == 0 {
		return fmt.Errorf("config: sources must contain at least one source")
	}
	for i, src := range c.Sources {
		if err := src.Validate(); err != nil {
			return fmt.Errorf("config: sources[%d]: %w", i, err)
		}
	}

	return c.validateModelRegistry()
}

// validateNamespaceSegment rejects a namespace that cannot be an entity-ID
// org segment, using the substrate's own validator on a composed probe ID —
// so a config that passes validate can never be rejected later purely for
// ID shape. Values are rejected with guidance, never silently rewritten
// (audit 2026-07-19, init-config-validation: a dotted org passed
// `semsource validate` and produced a permanently empty graph at runtime).
func validateNamespaceSegment(namespace string) error {
	return ValidateNamespace(namespace)
}

// ValidateNamespace reports whether a namespace can serve as an entity-ID org
// segment. Exported for the CLI wizard so bad values are rejected at the
// earliest surface with the same rule the config loader enforces.
func ValidateNamespace(namespace string) error {
	probe := namespace + ".semsource.golang.probe.function.probe"
	if err := semtypes.ValidateEntityID(probe); err != nil {
		return fmt.Errorf("config: namespace %q cannot be used as an entity-ID org segment "+
			"(allowed: ^[a-zA-Z0-9][a-zA-Z0-9_-]*$ — no dots, spaces, or leading symbols); "+
			"every published entity would be rejected by the graph: %w", namespace, err)
	}
	// The charset probe above uses short stand-ins for the other five segments,
	// so it cannot catch an org that only overflows once real repo slugs and
	// symbol names are in play. Bound it here instead: an over-long org is not
	// a per-entity problem but a deployment-wide one, and failing startup beats
	// discovering it as rejected entities during ingest.
	if len(namespace) > entityid.MaxOrgLen {
		return fmt.Errorf("config: namespace %q is %d bytes, over the %d-byte limit for an "+
			"entity-ID org segment; it would leave too little of the %d-byte entity-ID budget "+
			"for repo and symbol names, and entities would be rejected by the graph at ingest",
			namespace, len(namespace), entityid.MaxOrgLen, semtypes.MaxEntityIDBytes)
	}
	return nil
}

// validateModelRegistry checks the optional model registry and the tier wiring
// that depends on it, failing fast at load time (before any component starts)
// rather than letting graph-embedding/graph-clustering degrade silently at
// runtime. The registry itself is validated by semstreams; here we additionally
// enforce that a selected tier has the capability it needs to resolve.
func (c *Config) validateModelRegistry() error {
	if c.ModelRegistry != nil {
		if err := c.ModelRegistry.Validate(); err != nil {
			return fmt.Errorf("config: model_registry: %w", err)
		}
	}

	var embedderType string
	var clusteringLLM bool
	if c.Graph != nil {
		embedderType = c.Graph.EmbedderType
		clusteringLLM = c.Graph.EnableClustering && c.Graph.ClusteringLLM
	}

	// Tier 1: the HTTP embedder resolves the "embedding" capability at startup;
	// without a registry that provides it, graph-embedding cannot start.
	if embedderType == "http" {
		if err := requireCapability(c.ModelRegistry, model.CapabilityEmbedding,
			`graph.embedder_type "http" (tier 1) needs a model_registry with an "embedding" capability (→ semembed)`); err != nil {
			return err
		}
	}

	// Every capability that resolves at all must resolve to an endpoint that can
	// serve it — the complement of the tier checks below, which only cover the
	// capabilities a selected tier needs.
	if err := validateCapabilityRoles(c.ModelRegistry); err != nil {
		return err
	}

	// Tier 2: LLM community summaries resolve "community_summary" (→ seminstruct).
	if clusteringLLM {
		if err := requireCapability(c.ModelRegistry, model.CapabilityCommunitySummary,
			`graph.clustering_llm (tier 2) needs a model_registry with a "community_summary" capability (→ seminstruct)`); err != nil {
			return err
		}
	}

	return nil
}

// requireCapability fails unless the registry EXPLICITLY declares capability.
// We require an explicit capability rather than accepting reg.Resolve's
// fallback to defaults.model: that fallback would silently route, say,
// community_summary to the embedding endpoint (defaults.model is typically the
// embedder), which starts cleanly and then produces garbage. Demanding the
// operator declare the capability is the honest fail-fast. GetCapability!=nil
// already implies a non-empty Preferred list with existing endpoints (semstreams
// Registry.Validate ran first), so resolution is guaranteed to succeed.
func requireCapability(reg *model.Registry, capability, hint string) error {
	if reg == nil {
		return fmt.Errorf("config: %s, but none is configured", hint)
	}
	if reg.GetCapability(capability) == nil {
		return fmt.Errorf("config: %s — the registry declares no %q capability", hint, capability)
	}
	name := reg.Resolve(capability)
	if name == "" || reg.GetEndpoint(name) == nil {
		return fmt.Errorf("config: %s — %q resolves to no endpoint", hint, capability)
	}
	return nil
}

// llmCapabilities are the model-registry capabilities that require a generative
// (chat-completions) endpoint. Routing any of them to an embeddings endpoint
// starts cleanly and then fails at call time — the failure mode requireCapability
// already names for the tier capabilities it guards.
//
// embedding is deliberately absent: it is the one capability that REQUIRES an
// embeddings endpoint, and it is checked in the opposite direction below.
var llmCapabilities = []string{
	model.CapabilityQueryClassification,
	model.CapabilityAnswerSynthesis,
	model.CapabilityCommunitySummary,
	model.CapabilityAnomalyReview,
	model.CapabilitySummarization,
	model.CapabilityIntentClassification,
}

// isEmbeddingEndpoint reports whether an endpoint serves embeddings rather than
// chat completions.
//
// INFERENCE, NOT A PROBE. Asking the endpoint (GET /v1/models) would be
// authoritative, but `semsource validate` must work offline: making config
// validation depend on a running service would fail a perfectly good config
// whenever a container happens to be down, which is a worse failure than the one
// being prevented.
//
// query_prefix is the signal because the substrate documents it as "used ONLY by
// the embedding capability" — the query instruction for asymmetric retrieval
// models (arctic-embed, BGE, E5). It is meaningless on a chat endpoint, so its
// presence is a deliberate statement by whoever wrote the config. The model-name
// check is the backstop for symmetric embedders, which need no prefix.
//
// Both signals are already present in every config; neither is invented for this
// check. If this ever misclassifies, it fails validation loudly rather than
// producing garbage at runtime — the safe direction to be wrong in.
func isEmbeddingEndpoint(ep *model.EndpointConfig) bool {
	if ep == nil {
		return false
	}
	if ep.QueryPrefix != "" {
		return true
	}
	name := strings.ToLower(ep.Model)
	for _, marker := range []string{"embed", "arctic", "bge-", "e5-", "gte-", "nomic-"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// validateCapabilityRoles rejects a registry in which a capability resolves to an
// endpoint that cannot serve it.
//
// This is the complement of requireCapability. That function enforces that a
// capability a SELECTED TIER needs is explicitly declared; this one enforces that
// a capability which resolves AT ALL resolves to something real. Both say the same
// thing — a capability's binding is never allowed to be fictional — and the gap
// between them is how every shipped config came to route query_classification and
// answer_synthesis to the embedder via defaults.model.
//
// A misroute is REJECTED, not silently treated as unbound. Quietly degrading would
// fix the runtime behaviour and leave the configuration asserting something untrue,
// which is the same defect one layer up. An operator who set defaults.model
// deliberately deserves to be told it cannot serve answer_synthesis.
func validateCapabilityRoles(reg *model.Registry) error {
	if reg == nil {
		return nil
	}
	for _, capability := range llmCapabilities {
		name := reg.Resolve(capability)
		if name == "" {
			continue // unbound: the consuming component uses its documented non-LLM path
		}
		ep := reg.GetEndpoint(name)
		if ep == nil {
			continue // semstreams Registry.Validate already covers dangling references
		}
		if isEmbeddingEndpoint(ep) {
			return fmt.Errorf("config: model_registry: capability %q resolves to endpoint %q "+
				"(model %q), which serves embeddings and cannot answer chat completions; "+
				"either bind %q to a generative endpoint, or leave it unbound (remove "+
				"defaults.model) so the component uses its documented non-LLM fallback",
				capability, name, ep.Model, capability)
		}
	}
	// Deliberately NOT checked in the opposite direction — that embedding resolves
	// to something recognisably an embedder.
	//
	// The inference is asymmetric and only one direction is sound. A positive
	// signal (query_prefix, or a known embedding family in the model name) is
	// strong evidence an endpoint serves embeddings. The ABSENCE of one is not
	// evidence of the reverse: it is equally consistent with an embedder nobody
	// taught this function to recognise. Enforcing that direction rejected
	// `model: "arctic-s"` — a real embedding model — in this package's own
	// fixtures, which is exactly the false positive it would inflict on an
	// operator running a model we have not enumerated.
	//
	// Left unchecked, a chat endpoint bound to `embedding` fails at first use with
	// a protocol error. That is a loud, immediate, self-explanatory failure. The
	// defect this function exists for is the opposite: silent, deferred, and
	// invisible until an unexercised path is called.
	return nil
}
