// Package config loads and validates the semsource.json configuration file.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/c360studio/semengine/model"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semsource/entityid"
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
	// LexicalSearch runs graph-lexical, the tier 0 word index behind code_search
	// and doc_context. Nil means on; false composes a structural-only graph.
	LexicalSearch *bool `json:"lexical_search,omitempty"`

	// SemanticSearch composes tier 1: graph-embedding over the model_registry
	// embedding endpoint, merged with the word index by graph-query. Nil and
	// false mean off. It is an explicit switch rather than inferred from the
	// registry, because tier 0 configs carry registries for other capabilities.
	SemanticSearch *bool `json:"semantic_search,omitempty"`

	// CoalesceMs is the debounce window in ms for graph-index.
	// Defaults to 200.
	CoalesceMs int `json:"coalesce_ms,omitempty"`

	// IndexWorkers is the number of graph-index worker goroutines. 0 uses the
	// semengine default (1), which is a throughput bottleneck for large one-shot
	// ingests (indexing a whole dependency) — raise it to parallelize index build.
	IndexWorkers int `json:"index_workers,omitempty"`

	// EnableClustering wires the graph-clustering component (entity-ID prefix
	// community grouping over ENTITY_STATES → COMMUNITY_INDEX, ADR-099), lighting
	// up the declared local/global/summary query routes (tier 2). Off by default.
	EnableClustering bool `json:"enable_clustering,omitempty"`

	// Retired SemStreams fields decoded only so Validate() can reject them with
	// an actionable migration message instead of silently ignoring them.
	GatewayBind        string          `json:"gateway_bind,omitempty"`
	EnablePlayground   *bool           `json:"enable_playground,omitempty"`
	EmbeddingBatchSize *int            `json:"embedding_batch_size,omitempty"`
	ClusteringLLM      *bool           `json:"clustering_llm,omitempty"`
	EmbedderType       json.RawMessage `json:"embedder_type,omitempty"`
	EntityIDEdges      json.RawMessage `json:"entity_id_edges,omitempty"`
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
	Subjects []string `json:"subjects,omitempty"`
	Storage  string   `json:"storage,omitempty"`
	MaxBytes *int64   `json:"max_bytes,omitempty"`
	MaxAge   string   `json:"max_age,omitempty"`
	Replicas *int     `json:"replicas,omitempty"`

	// Discard is what happens when the stream reaches MaxBytes or MaxAge:
	// "old" evicts the oldest messages (the write succeeds and the loss is
	// silent), "new" refuses the write (producers see NATS 503
	// err_code=10077). semengine requires ordinary streams to
	// declare it; see graphStreamConfig for why the GRAPH default is "new".
	Discard string `json:"discard,omitempty"`
}

// Config is the top-level semsource configuration.
type Config struct {
	// Namespace is the org identifier used in entity ID construction (e.g., "acme").
	Namespace string `json:"namespace"`

	// PlatformID is the declared deployment stem. The framework persists its effective unique authority at first boot.
	PlatformID string `json:"platform_id,omitempty"`

	// ConfigRevision orders edits of this file against the configuration a
	// deployment retains from its last boot: the retained configuration stays
	// in effect until the file carries a higher revision. Bump it after every
	// edit; `semsource add` and `semsource remove` bump it for you. Defaults to 0.
	ConfigRevision int `json:"config_revision,omitempty"`

	// Sources lists all ingestion sources.
	Sources []SourceEntry `json:"sources"`

	// EntityStore configures persistent graph storage.
	// When set, entities are persisted to the NATS KV ENTITY_STATES bucket.
	EntityStore *EntityStoreConfig `json:"entity_store,omitempty"`

	// WorkspaceDir is the base directory where remote git repositories are
	// cloned. Defaults to ~/.semsource/repos when empty.
	WorkspaceDir string `json:"workspace_dir,omitempty"`

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
	//
	// Configurable again since semstreams beta.161 restored the component's
	// path surface (semsource#147, semstreams#945); beta.160 had pinned it
	// to "/ws". "/ws" stays the default and the documented contract path.
	WebSocketPath string `json:"websocket_path,omitempty"`

	// Graph configures graph subsystem components.
	Graph *GraphConfig `json:"graph,omitempty"`

	// ModelRegistry is the semengine unified model-endpoint registry, passed
	// straight through to the ServiceManager (ssCfg.ModelRegistry). Reuses the
	// semengine model.Registry type directly — no bespoke shape. Nil = tier 0,
	// no external model services. Nothing at tier 0 resolves from it; the
	// tier 1 embedder that will is #234.
	ModelRegistry *model.Registry `json:"model_registry,omitempty"`

	// Metrics configures the Prometheus metrics endpoint.
	Metrics *MetricsConfig `json:"metrics,omitempty"`

	// Streams allows overriding JetStream stream configurations.
	// Keys are stream names (e.g., "GRAPH").
	Streams map[string]StreamOverride `json:"streams,omitempty"`
}

// EnvGitToken is the environment variable holding the access token for
// authenticated HTTPS clones of private repositories.
const EnvGitToken = "SEMSOURCE_GIT_TOKEN"

// GitToken returns the git access token from the environment. It is read
// where it is used and is never part of a configuration: component
// configurations are replicated through the config bucket, and the file is
// rewritten by the CLI, so a token held in either would be persisted in
// plaintext (#272).
func GitToken() string {
	return os.Getenv(EnvGitToken)
}

// applyDefaults fills in omitted fields with their documented defaults.
func (c *Config) applyDefaults() {
	if c.PlatformID == "" {
		c.PlatformID = "semsource"
	}
	if c.WorkspaceDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			c.WorkspaceDir = filepath.Join(home, ".semsource", "repos")
		}
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
		c.WebSocketPath = "/ws"
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
	stem := c.PlatformStem()
	if err := (entityid.Authority{Org: c.Namespace, Platform: stem}).Validate(); err != nil {
		return fmt.Errorf("config: platform_id: %w", err)
	}
	if len(c.Namespace)+len(stem)+7 > entityid.MaxAuthorityPairLen {
		return fmt.Errorf("config: namespace and platform_id must leave seven bytes for the persisted authority suffix (maximum combined length %d)", entityid.MaxAuthorityPairLen-7)
	}
	if c.ConfigRevision < 0 {
		return fmt.Errorf("config: config_revision must be zero or greater, got %d", c.ConfigRevision)
	}
	if len(c.Sources) == 0 {
		return fmt.Errorf("config: sources must contain at least one source")
	}
	for i, src := range c.Sources {
		if err := src.Validate(); err != nil {
			return fmt.Errorf("config: sources[%d]: %w", i, err)
		}
	}

	if err := c.validateGraph(); err != nil {
		return err
	}

	if err := c.validateStreams(); err != nil {
		return err
	}

	// The path is honored again since semstreams beta.161 restored the
	// websocket output's path surface (semsource#147, semstreams#945), so
	// validation checks shape, not the beta.160-era "/ws" pin. Failing here
	// keeps `semsource validate` load-time-loud; the component's own
	// Validate stays authoritative for full ServeMux pattern rules.
	if c.WebSocketPath != "" {
		if err := validateWebSocketPath(c.WebSocketPath); err != nil {
			return fmt.Errorf("config: websocket_path %q: %w", c.WebSocketPath, err)
		}
	}

	if err := c.validateModelRegistry(); err != nil {
		return err
	}
	return c.validateSemanticSearch()
}

// validateGraph checks the graph block and rejects retired SemStreams keys
// with explicit migration instructions so operators are never misled into
// thinking a silently dropped key took effect.
func (c *Config) validateGraph() error {
	if c.Graph == nil {
		return nil
	}
	if c.Graph.GatewayBind != "" {
		return fmt.Errorf("config: graph.gateway_bind is retired in SemEngine; graph-gateway serves HTTP on the shared ServiceManager http_port (/graph-gateway/graphql) — remove graph.gateway_bind")
	}
	if c.Graph.EnablePlayground != nil {
		return fmt.Errorf("config: graph.enable_playground is retired in SemEngine; remove graph.enable_playground")
	}
	if c.Graph.EmbeddingBatchSize != nil {
		return fmt.Errorf("config: graph.embedding_batch_size is retired in SemEngine; remove graph.embedding_batch_size")
	}
	if c.Graph.ClusteringLLM != nil {
		return fmt.Errorf("config: graph.clustering_llm is retired in SemEngine (LLM community summaries are tracked in semengine#485); remove graph.clustering_llm")
	}
	if len(c.Graph.EntityIDEdges) > 0 && string(c.Graph.EntityIDEdges) != "null" {
		return fmt.Errorf("config: graph.entity_id_edges is retired in SemEngine (ADR-099 groups communities by entity-ID prefix); remove graph.entity_id_edges")
	}
	// Any presence is refused, null included: the key has no meaning on SemEngine.
	if len(c.Graph.EmbedderType) > 0 {
		return fmt.Errorf("config: graph.embedder_type is retired in SemEngine: tier 0 word search is graph-lexical, on by default " +
			"(set graph.lexical_search to false for a structural-only graph), and tier 1 is graph.semantic_search: true with a " +
			"model_registry embedding endpoint — remove graph.embedder_type")
	}
	return nil
}

// LexicalSearchEnabled reports whether the composition runs graph-lexical.
func (g *GraphConfig) LexicalSearchEnabled() bool {
	return g == nil || g.LexicalSearch == nil || *g.LexicalSearch
}

// SemanticSearchEnabled reports whether the composition runs tier 1:
// graph-embedding, merged with the word index by graph-query.
func (g *GraphConfig) SemanticSearchEnabled() bool {
	return g != nil && g.SemanticSearch != nil && *g.SemanticSearch
}

// validateSemanticSearch refuses a tier 1 config that would boot as something
// else. Without an embedding endpoint graph-embedding fails at every start
// while search quietly answers words only; without the word index there is no
// second list to merge with. The refusal names the missing piece.
//
// This is the reverse-direction check validateCapabilityRoles deliberately
// skips: here the operator has asked for tier 1, so an endpoint that is not
// recognisably an embedder is refused rather than trusted. Reachability is not
// probed (validate must work offline); an unreachable provider boots degraded.
func (c *Config) validateSemanticSearch() error {
	if !c.Graph.SemanticSearchEnabled() {
		return nil
	}
	if !c.Graph.LexicalSearchEnabled() {
		return fmt.Errorf("config: graph.semantic_search needs graph.lexical_search on: tier 1 merges the word index's list " +
			"with the embedding list — remove graph.lexical_search: false or graph.semantic_search")
	}
	if c.ModelRegistry == nil {
		return fmt.Errorf("config: graph.semantic_search needs a model_registry whose %q capability resolves to an embedding "+
			"endpoint (for example semembed at http://semembed:8081/v1); none is configured", model.CapabilityEmbedding)
	}
	name := c.ModelRegistry.Resolve(model.CapabilityEmbedding)
	ep := c.ModelRegistry.GetEndpoint(name)
	if ep == nil {
		return fmt.Errorf("config: graph.semantic_search needs model_registry capability %q to resolve to an endpoint; "+
			"it resolves to none", model.CapabilityEmbedding)
	}
	if !isEmbeddingEndpoint(ep) {
		return fmt.Errorf("config: graph.semantic_search: model_registry capability %q resolves to endpoint %q (model %q), "+
			"which is not recognisably an embedding endpoint; bind %q to an embedding model", model.CapabilityEmbedding,
			name, ep.Model, model.CapabilityEmbedding)
	}
	if err := checkEmbeddingURL(ep.URL); err != nil {
		return fmt.Errorf("config: graph.semantic_search: model_registry endpoint %q url %q %w", name, ep.URL, err)
	}
	return nil
}

// checkEmbeddingURL checks a tier 1 base URL offline: an absolute http or https
// URL with a host, which httpembedder itself requires but refuses only at
// Start, and a path ending in /v1, semembed's convention (the client adds
// /embeddings), which httpembedder does not check and a wrong path fails only on
// the first request. Validation names a typo instead of booting degraded.
// Nothing is dialled.
//
// shortcut: /v1 is required because semembed is the only provider SemSource
// ships for; drop it or make it per-provider when a provider with another API
// root is supported.
func checkEmbeddingURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("does not parse: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("is not an http or https URL")
	}
	if u.Hostname() == "" {
		return fmt.Errorf("has no host")
	}
	if !strings.HasSuffix(strings.TrimSuffix(u.Path, "/"), "/v1") {
		return fmt.Errorf("must end in /v1, the semembed convention (the embedder adds /embeddings), for example http://semembed:8081/v1")
	}
	return nil
}

// defaultGraphIngestSubjects lists the five subjects SemSource's GRAPH stream
// binds by default so entitypub, graph-ingest, and source-manifest stay connected.
var defaultGraphIngestSubjects = []string{
	"graph.ingest.entity",
	"graph.ingest.batch",
	"graph.ingest.manifest",
	"graph.ingest.status",
	"graph.ingest.predicates",
}

// validateStreams validates optional stream overrides. An explicit GRAPH
// subject list must name exactly the five default graph.ingest.* subjects
// (entitypub, graph-ingest, and source-manifest stay connected) and nothing
// else: a stream bound over a request/reply subject SemSource serves
// (graph.query.status, graph.ingest.add.<ns>, ...) lets the PubAck answer the
// request, and SemEngine's boot check protects only its own query verbs.
// Covering wildcards pass config: "graph.ingest.>", "graph.>", and ">" reach
// SemEngine's EnsureStreams reserved_subject_capture startup refusal, and
// "graph.ingest.*" covers only three-token subjects, none of them served.
func (c *Config) validateStreams() error {
	override, ok := c.Streams["GRAPH"]
	if !ok || len(override.Subjects) == 0 {
		return nil
	}
	present := make(map[string]bool, len(override.Subjects))
	wildcard := false
	for _, subj := range override.Subjects {
		if subj == "graph.ingest.*" || subj == "graph.ingest.>" || subj == "graph.>" || subj == ">" {
			wildcard = true
			continue
		}
		if !slices.Contains(defaultGraphIngestSubjects, subj) {
			return fmt.Errorf("config: streams.GRAPH.subjects entry %q is not one of the default graph.ingest subjects (a stream bound over a request/reply subject swallows its replies)", subj)
		}
		present[subj] = true
	}
	if wildcard {
		return nil
	}
	for _, req := range defaultGraphIngestSubjects {
		if !present[req] {
			return fmt.Errorf("config: streams.GRAPH.subjects is missing required subject %q (all default graph.ingest.* subjects must remain connected)", req)
		}
	}
	return nil
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

// validateWebSocketPath mirrors the semstreams websocket output's basic path
// rules (leading slash, no whitespace or control bytes) so an unservable path
// fails at config load rather than at component start. The component's own
// Validate remains authoritative for full ServeMux pattern rules.
func validateWebSocketPath(path string) error {
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("must begin with /")
	}
	for i := 0; i < len(path); i++ {
		if path[i] <= ' ' || path[i] == 0x7f {
			return fmt.Errorf("contains whitespace or a control character at byte %d", i)
		}
	}
	return nil
}

// validateModelRegistry checks the optional model registry at load time, before
// any component starts. The registry itself is validated by semengine; here we
// additionally enforce that every capability resolves to an endpoint that can
// serve it.
func (c *Config) validateModelRegistry() error {
	if c.ModelRegistry != nil {
		if err := c.ModelRegistry.Validate(); err != nil {
			return fmt.Errorf("config: model_registry: %w", err)
		}
	}

	// Every capability that resolves at all must resolve to an endpoint that can
	// serve it.
	return validateCapabilityRoles(c.ModelRegistry)
}

// llmCapabilities are the model-registry capabilities that require a generative
// (chat-completions) endpoint. Routing any of them to an embeddings endpoint
// starts cleanly and then fails at call time.
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
// A capability which resolves AT ALL must resolve to something real: a
// capability's binding is never allowed to be fictional. Without this check every
// shipped config came to route query_classification and answer_synthesis to the
// embedder via defaults.model.
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

// PlatformStem returns the declaration used to locate the persisted deployment identity.
func (c *Config) PlatformStem() string {
	if c.PlatformID != "" {
		return c.PlatformID
	}
	return "semsource"
}
