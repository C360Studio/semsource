package sourcemanifest

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/c360studio/semsource/internal/sourceallow"
	"github.com/c360studio/semsource/internal/sourcespawn"
)

// maxAddBodyBytes bounds a source-registration request body (small JSON).
const maxAddBodyBytes = 1 << 20

// SourceHandle is the auditable descriptor returned by GET /sources/{id}. It
// resolves a registration handle (a component instance name) to the stable
// metadata a long-running caller needs to prove which graph view it used
// (ADR-0007 §5). ref/commit generation and indexed-at timestamps are follow-on,
// gated on the fact-provenance model.
type SourceHandle struct {
	InstanceName  string `json:"instance_name"`
	SourceType    string `json:"source_type"`
	Path          string `json:"path,omitempty"`
	URL           string `json:"url,omitempty"`
	Branch        string `json:"branch,omitempty"`
	Mode          string `json:"mode"` // "watch" | "snapshot"
	StatusSubject string `json:"status_subject,omitempty"`
	ReadyWhen     string `json:"ready_when,omitempty"`
}

// ingestConfig returns the wired ingest config, or nil if the host has not yet
// called RegisterIngestHandlers (startup race — handlers 503 while nil).
func (c *Component) ingestConfig() *IngestHandlerConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ingestCfg
}

// handleAddHTTP serves POST /sources — the HTTP face of the source-add path
// (ADR-0007). It applies the transport-level guards (auth, path allowlist) then
// delegates to the same addSource code path the NATS handler uses.
func (c *Component) handleAddHTTP(w http.ResponseWriter, r *http.Request) {
	cfg, ok := c.authorizedIngest(w, r)
	if !ok {
		return
	}

	var req AddRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAddBodyBytes))
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, &AddReply{
			Error:     &IngestError{Code: CodeValidationFailed, Message: "decode request: " + err.Error()},
			Timestamp: time.Now(),
		})
		return
	}

	// Path allowlist (ADR-0007 §3): a path-based source over HTTP must resolve
	// under an allowlisted root; arbitrary host paths are rejected.
	if err := sourceallow.Enforce(req.Source, cfg.AllowedRoots); err != nil {
		writeJSON(w, http.StatusForbidden, &AddReply{
			Error:     &IngestError{Code: CodeValidationFailed, Message: err.Error()},
			Timestamp: time.Now(),
		})
		return
	}

	reply := c.addSource(r.Context(), req, *cfg)
	status := http.StatusOK
	if reply.Error != nil && len(reply.Components) == 0 {
		status = httpStatusForIngestError(reply.Error.Code)
	}
	writeJSON(w, status, reply)
}

// handleRemoveHTTP serves DELETE /sources/{id}. Removal stops ingestion but does
// not retract entities (ADR-0007 sequencing guardrail).
func (c *Component) handleRemoveHTTP(w http.ResponseWriter, r *http.Request) {
	cfg, ok := c.authorizedIngest(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing source id", http.StatusBadRequest)
		return
	}
	reply := c.removeSource(r.Context(), id, r.Header.Get("X-Actor"), *cfg)
	status := http.StatusOK
	if reply.Error != nil {
		status = httpStatusForIngestError(reply.Error.Code)
	}
	writeJSON(w, status, reply)
}

// handleSourceHTTP serves GET /sources/{id} — resolves a registration handle to
// its auditable SourceHandle (ADR-0007 §5/§6, HTTP-pollable).
func (c *Component) handleSourceHTTP(w http.ResponseWriter, r *http.Request) {
	cfg, ok := c.authorizedIngest(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	handle, found := c.sourceHandle(id, cfg.Spawn)
	if !found {
		http.Error(w, "source not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, handle)
}

// authorizedIngest runs the shared preconditions for the write/handle endpoints:
// the component is running, the ingest config is wired, and the bearer token (if
// configured) matches. On failure it writes the response and returns ok=false.
func (c *Component) authorizedIngest(w http.ResponseWriter, r *http.Request) (*IngestHandlerConfig, bool) {
	if !c.requireRunning(w) {
		return nil, false
	}
	cfg := c.ingestConfig()
	if cfg == nil {
		http.Error(w, "source registration not ready", http.StatusServiceUnavailable)
		return nil, false
	}
	if !authorized(r, cfg.APIToken) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	return cfg, true
}

// authorized enforces the optional bearer-token seam (ADR-0007 §6). An empty
// token means permissive (trusted-network) — all callers allowed. The compare is
// constant-time.
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

// sourceHandle resolves an instance name back to a SourceHandle by finding the
// registered manifest source whose sourcespawn expansion produces it.
func (c *Component) sourceHandle(instanceName string, opts sourcespawn.Options) (*SourceHandle, bool) {
	c.manifestMu.RLock()
	defer c.manifestMu.RUnlock()
	for _, existing := range c.manifestSources {
		src := manifestSourceToSourceEntry(existing)
		built, err := sourcespawn.Build(src, opts)
		if err != nil {
			continue
		}
		if _, ok := built[instanceName]; ok {
			return &SourceHandle{
				InstanceName:  instanceName,
				SourceType:    src.Type,
				Path:          src.Path,
				URL:           src.URL,
				Branch:        src.Branch,
				Mode:          watchMode(src.Watch),
				StatusSubject: statusSubject,
				ReadyWhen:     ingestReadyWhen,
			}, true
		}
	}
	return nil, false
}

func watchMode(watch bool) string {
	if watch {
		return "watch"
	}
	return "snapshot"
}

// httpStatusForIngestError maps a wire error code to an HTTP status.
func httpStatusForIngestError(code IngestErrorCode) int {
	switch code {
	case CodeValidationFailed, CodeUnsupportedType:
		return http.StatusBadRequest
	case CodeInstanceExists:
		return http.StatusConflict
	case CodeNotFound:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
