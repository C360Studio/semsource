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

	"github.com/c360studio/semsource/source/fusion"
	"github.com/c360studio/semsource/source/fusion/lens/code"
	"github.com/c360studio/semsource/source/fusion/lens/docs"
	"github.com/c360studio/semsource/source/fusion/natsgraph"
)

// maxBodyBytes bounds an HTTP request body (queries are small JSON).
const maxBodyBytes = 1 << 20

// requestTimeout caps the whole fuse (many bounded graph round-trips), so a
// slow or wedged query cannot run unbounded and a caller disconnect cancels it.
const requestTimeout = 30 * time.Second

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
	engine      *fusion.Engine
	client      *natsclient.Client
	logger      *slog.Logger

	mu        sync.RWMutex
	running   bool
	startTime time.Time
	subs      []*natsclient.Subscription
}

// NewComponent creates a new code-context component over the NATS graph client.
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
	return &Component{
		name:        name,
		lensKind:    config.Lens,
		subjectRoot: config.Lens + ".v1.",
		engine:      fusion.NewEngine(natsgraph.New(deps.NATSClient)),
		client:      deps.NATSClient,
		logger:      deps.GetLogger(),
	}, nil
}

// Initialize prepares the component (no-op; setup happens in Start).
func (c *Component) Initialize() error { return nil }

// Start subscribes the NATS request handlers.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("component already running")
	}
	c.mu.Unlock()

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
// "not ready" answer (graph still seeding) is distinct from "not found".
func (c *Component) serve(ctx context.Context, verb string, body []byte) ([]byte, error) {
	var req fusion.Request
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("decode request: %w", err)
		}
	}
	lens, err := c.lensFor(req)
	if err != nil {
		return nil, err
	}
	if len(req.Want) == 0 {
		req.Want = defaultWants(verb)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	resp, err := c.engine.Fuse(ctx, req, lens)
	if err != nil {
		return nil, err
	}
	return json.Marshal(resp)
}

// lensFor builds a fresh lens for one request. The code lens is worktree-scoped
// (it hydrates source from req.Repo); the docs lens hydrates from the graph.
func (c *Component) lensFor(req fusion.Request) (fusion.Lens, error) {
	if c.lensKind == "docs" {
		return docs.New(), nil
	}
	if req.Repo == "" {
		return nil, fmt.Errorf("repo is required for code queries")
	}
	return code.New(req.Repo), nil
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
