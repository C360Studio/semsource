// Package codecontext is the fusion gateway component: it answers fused
// code_context / doc_context queries over NATS request/reply and HTTP/JSON by
// driving the deterministic fusion engine (source/fusion) against the graph
// (graph.query.* via natsgraph). It is lens-parameterized — one factory, run as
// a "code" instance and a "docs" instance — so the same component serves both
// domains. See docs/adr/0004 (fusion gateway).
package codecontext

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/c360studio/semstreams/pkg/fusion/fusionnats"
	"github.com/c360studio/semstreams/pkg/fusion/fusionvocab"
	"github.com/c360studio/semstreams/storage"
	"github.com/c360studio/semstreams/storage/objectstore"
	"github.com/c360studio/semstreams/storage/storeregistry"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semsource/source/fusion/lens/code"
	"github.com/c360studio/semsource/source/fusion/lens/docs"
)

// codeScopeDomains are the entity-ID domain segments the "code" lens covers —
// the languages the AST parsers stamp on code entity IDs. KEEP IN SYNC with the
// registered AST parsers (source/ast/*/): a language present there but missing
// here is silently excluded from code_context NL retrieval scope. Note the ID
// domain is NOT the parser registration name (Go registers as "go" but stamps
// "golang"), so this list is maintained by hand, not derived from the registry.
var codeScopeDomains = []string{"golang", "python", "typescript", "javascript", "java", "svelte"}

// docScopeDomains are the entity-ID domain segments the "docs" lens covers.
// Doc/prose entities (handler/doc type "doc", handler/url type "page") all live
// under the "web" domain.
var docScopeDomains = []string{"web"}

// maxBodyBytes bounds an HTTP request body (queries are small JSON).
const maxBodyBytes = 1 << 20

// requestTimeout caps the whole fuse (many bounded graph round-trips), so a
// slow or wedged query cannot run unbounded and a caller disconnect cancels it.
const requestTimeout = 30 * time.Second

// The verbatim-body ObjectStore the producers offload to and this gateway
// dereferences (ADR-0006 §5 / semstreams#399). The instance + bucket are shared
// with the producers in graph.BodyStore{Instance,Bucket} so the addressing
// cannot silently drift between producer and resolver.

// verbs are the query verbs exposed over both NATS and HTTP. "context" is the
// primary fused query; the rest preset a default facet set.
var verbs = []string{"context", "callers", "callees", "impact", "file", "search"}

// Component implements the code-context fusion gateway. It holds a fusion engine
// over the graph and a lens kind ("code" or "docs"); a fresh lens is built per
// request (the code lens is worktree-scoped for source hydration).
type Component struct {
	name        string
	lensKind    string // "code" | "docs"
	subjectRoot string // NATS subject root, e.g. "code.v1."
	// org is the deployment's single global org (= the required top-level
	// namespace), sourced from platform identity. It forms the first segment of
	// the per-lens default retrieval scope; empty means no default scope (ask
	// #16 / ADR-071).
	org   string
	graph fusion.RetrievalClient
	// engine is written once in Start (buildEngine) BEFORE running is set, and
	// read lock-free thereafter — the running-gate ordering (running is set under
	// mu after buildEngine returns; serve only runs once running/subscribed)
	// publishes it safely. Guarded by that ordering, not by mu; keep it that way.
	engine *fusion.Engine
	client *natsclient.Client
	// storeRegistry is the shared {StorageInstance → store} resolver (ADR-063),
	// injected via deps. When set, body hydration resolves offloaded bodies
	// through it (lazily, per query); nil in standalone/tests, where buildEngine
	// falls back to its own objectstore attach.
	storeRegistry *storeregistry.Registry
	logger        *slog.Logger

	mu        sync.RWMutex
	running   bool
	startTime time.Time
	subs      []*natsclient.Subscription
}

// NewComponent creates a new code-context component. It builds the fusion
// retrieval client (graph.query.* over NATS) now; the ObjectStore-backed body
// resolver and engine are built in Start, where a context exists to attach the
// store.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	name := "code-context"
	if config.Lens == "docs" {
		name = "doc-context"
	}
	var graph fusion.RetrievalClient
	if deps.NATSClient != nil {
		graph = fusionnats.New(deps.NATSClient, 0)
	}
	return &Component{
		name:          name,
		lensKind:      config.Lens,
		subjectRoot:   config.Lens + ".v1.",
		org:           deps.Platform.Org,
		graph:         graph,
		client:        deps.NATSClient,
		storeRegistry: deps.StoreRegistry,
		logger:        deps.GetLogger(),
	}, nil
}

// Initialize prepares the component (no-op; setup happens in Start).
func (c *Component) Initialize() error { return nil }

// Start builds the ObjectStore-backed fusion engine and subscribes the NATS
// request handlers. Body hydration derefs the lens's StorageReference handle
// through a BodyResolver over the "objectstore" store (ADR-062 increment 4);
// a store attach failure is fatal to Start — a gateway that cannot return
// bodies is misconfigured, not degraded.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	c.mu.Unlock()

	if err := c.buildEngine(ctx); err != nil {
		return err
	}

	if c.client != nil {
		for _, verb := range verbs {
			if err := c.subscribeVerb(ctx, verb); err != nil {
				return err
			}
		}
	}

	c.mu.Lock()
	c.running = true
	c.startTime = time.Now()
	c.mu.Unlock()
	return nil
}

// buildEngine constructs the lens-driven fusion engine over the verbatim-body
// resolver. A pre-set engine (unit tests inject one) is left as-is. A nil NATS
// client is a hard error: a gateway with no graph client cannot answer, and
// letting Start set running=true without an engine would let a request nil-deref
// Fuse — so Start fails fast rather than run half-built.
func (c *Component) buildEngine(ctx context.Context) error {
	if c.engine != nil {
		return nil
	}
	if c.client == nil {
		return fmt.Errorf("code-context requires a NATS client")
	}
	resolver, err := c.bodyResolver(ctx)
	if err != nil {
		return err
	}
	// Attach the framework ranking signals (ADR-062 increment 5): ontology
	// specificity (BFO/CCO subclass depth on our stamped entity.ontology.class)
	// + predicate salience (vocabulary WithWeight). Without this the engine
	// ranks on resolve-order + lexical only, ignoring the ontology class we emit
	// on every entity. fusionvocab resolves both against the framework registries.
	c.engine = fusion.NewEngine(c.graph, resolver).WithSignals(fusionvocab.New())
	return nil
}

// bodyResolver builds the verbatim-body resolver for hydration. It prefers the
// shared StoreRegistry (ADR-063), resolved lazily per query, so it dereferences
// bodies offloaded to ANY registered instance (e.g. "objectstore") without this
// gateway opening its own store connection. When no registry is wired
// (standalone/tests), it falls back to attaching its own objectstore over the
// shared CONTENT bucket. Either way a missing store is fatal to Start — a gateway
// that cannot return bodies is misconfigured, not degraded.
func (c *Component) bodyResolver(ctx context.Context) (*fusion.BodyResolver, error) {
	if c.storeRegistry != nil {
		return fusion.NewBodyResolver(c.storeRegistry), nil
	}
	store, err := objectstore.NewStoreWithConfig(ctx, c.client, objectstore.Config{
		BucketName:   graph.BodyStoreBucket,
		InstanceName: graph.BodyStoreInstance,
	})
	if err != nil {
		return nil, fmt.Errorf("attach body store %q: %w", graph.BodyStoreInstance, err)
	}
	return fusion.NewBodyResolver(fusion.MapStoreResolver{graph.BodyStoreInstance: storage.Store(store)}), nil
}

// subscribeVerb wires one NATS request/reply subject to the shared query path.
func (c *Component) subscribeVerb(ctx context.Context, verb string) error {
	subject := c.subjectRoot + verb
	sub, err := c.client.SubscribeForRequests(ctx, subject, func(reqCtx context.Context, body []byte) ([]byte, error) {
		return c.serve(reqCtx, verb, body)
	})
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", subject, err)
	}
	c.mu.Lock()
	c.subs = append(c.subs, sub)
	c.mu.Unlock()
	c.logger.Info("listening for code-context queries", "subject", subject)
	return nil
}

// serve decodes a request, fuses it through the configured lens, and returns the
// marshaled response. The response always carries the readiness envelope, so a
// "not ready" answer (graph still seeding) is distinct from "not found". The
// engine populates the Paths/Impact facets on the response itself when the
// request Wants them (beta.123, semstreams#409) — no local extension.
func (c *Component) serve(ctx context.Context, verb string, body []byte) ([]byte, error) {
	var req fusion.Request
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("decode request: %w", err)
		}
	}
	lens := c.lensFor()
	if len(req.Want) == 0 {
		req.Want = defaultWants(verb)
	}
	// Domain-scope NL seed resolution to this lens's own domain so a smaller
	// domain (e.g. docs) is not diluted by a larger co-resident one (e.g. code)
	// over the shared embedding index (ask #16 / ADR-071). Respect a
	// caller-provided scope; default only when none was set.
	if len(req.Scope) == 0 {
		req.Scope = c.defaultScope()
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	resp, err := c.engine.Fuse(ctx, req, lens)
	if err != nil {
		return nil, err
	}
	return json.Marshal(resp)
}

// lensFor builds a fresh lens for the configured domain. Both lenses are
// stateless: bodies are hydrated from ObjectStore handles (ADR-062), not a
// worktree, so the query no longer carries a repo.
func (c *Component) lensFor() fusion.Lens {
	if c.lensKind == "docs" {
		return docs.New()
	}
	return code.New()
}

// defaultScope returns the per-lens NL retrieval scope: the entity-ID prefixes
// {org}.{platform}.{domain} for each domain this lens covers. It returns nil
// when the org is unknown (standalone/test contexts with no platform identity),
// which leaves NL resolution unfiltered — the pre-ask-#16 behavior — rather than
// emitting a malformed prefix. The platform segment is entityid.PlatformSemsource
// (the literal every entity ID is built with), not deps.Platform.Platform.
func (c *Component) defaultScope() []string {
	if c.org == "" {
		return nil
	}
	domains := codeScopeDomains
	if c.lensKind == "docs" {
		domains = docScopeDomains
	}
	scope := make([]string, len(domains))
	for i, domain := range domains {
		scope[i] = c.org + "." + entityid.PlatformSemsource + "." + domain
	}
	return scope
}

// defaultWants returns the facet set for a verb when the caller did not specify
// Want. "context" returns nil so the engine applies its own default.
func defaultWants(verb string) []fusion.Want {
	switch verb {
	case "callers", "callees":
		return []fusion.Want{fusion.WantBody, fusion.WantRelations}
	case "impact":
		return []fusion.Want{fusion.WantBody, fusion.WantImpact}
	case "file", "search":
		return []fusion.Want{fusion.WantBody}
	default:
		return nil
	}
}

// RegisterHTTPHandlers mounts POST /<prefix>/{verb} on the ServiceManager's
// shared mux. The instance name yields the prefix (code-context → /code-context/,
// doc-context → /doc-context/).
func (c *Component) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	for _, verb := range verbs {
		v := verb
		path := prefix + v
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			c.handleHTTP(w, r, v)
		})
		c.logger.Info("registered HTTP handler", "path", path)
	}
}

// handleHTTP serves one verb over HTTP/JSON. The body is bounded; internal error
// detail is logged, not echoed across the external boundary.
func (c *Component) handleHTTP(w http.ResponseWriter, r *http.Request, verb string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.requireRunning(w) {
		return
	}
	body, err := readBody(w, r)
	if err != nil {
		http.Error(w, "request body too large or unreadable", http.StatusBadRequest)
		return
	}
	data, err := c.serve(r.Context(), verb, body)
	if err != nil {
		c.logger.Warn("code-context request failed", "verb", verb, "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// readBody reads the request body, rejecting (not truncating) anything over the
// limit via MaxBytesReader.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer func() { _ = r.Body.Close() }()
	return io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
}

// requireRunning writes a 503 and returns false if Start has not completed.
func (c *Component) requireRunning(w http.ResponseWriter) bool {
	c.mu.RLock()
	started := c.running
	c.mu.RUnlock()
	if !started {
		http.Error(w, "component not started", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// Stop unsubscribes the NATS handlers. The subscription slice is cleaned up
// unconditionally so a partial Start failure does not leak, and it is safe to
// call more than once.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, sub := range c.subs {
		if sub != nil {
			_ = sub.Unsubscribe()
		}
	}
	c.subs = nil
	if c.running {
		c.running = false
		c.logger.Info("code-context stopped")
	}
	return nil
}

// Meta implements component.Discoverable.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        c.name,
		Type:        "processor",
		Description: "Fused code_context/doc_context queries: verbatim body plus structure, over NATS and HTTP",
		Version:     "0.2.0",
	}
}

// InputPorts implements component.Discoverable (none — driven by request/reply).
func (c *Component) InputPorts() []component.Port { return []component.Port{} }

// OutputPorts implements component.Discoverable (none — request/reply + HTTP).
func (c *Component) OutputPorts() []component.Port { return []component.Port{} }

// ConfigSchema implements component.Discoverable.
func (c *Component) ConfigSchema() component.ConfigSchema { return codeContextSchema }

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
