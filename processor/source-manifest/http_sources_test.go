package sourcemanifest

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
)

func httpTestLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// newHTTPComponent builds a running source-manifest with the HTTP façade routes
// registered on a fresh mux. A nil client is fine — publishManifest skips the
// NATS broadcast and only prepares the HTTP responseData.
func newHTTPComponent(t *testing.T, cfg *IngestHandlerConfig, manifest []ManifestSource) *http.ServeMux {
	t.Helper()
	c := &Component{
		name:            "source-manifest",
		config:          Config{Namespace: "acme"},
		logger:          httpTestLogger(),
		running:         true,
		ingestCfg:       cfg,
		manifestSources: manifest,
	}
	mux := http.NewServeMux()
	// ServiceManager registers under the leading-slash instance prefix.
	c.RegisterHTTPHandlers("/source-manifest", mux)
	return mux
}

func stubIngestCfg() *IngestHandlerConfig {
	return &IngestHandlerConfig{
		Namespace: "acme",
		Store:     stubStore{},
		Spawn:     sourcespawn.Options{Org: "acme"},
	}
}

func doJSON(t *testing.T, mux *http.ServeMux, method, path string, body any, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// --- POST /sources ---------------------------------------------------------

func TestHandleAddHTTP_URLSourceSucceeds(t *testing.T) {
	mux := newHTTPComponent(t, stubIngestCfg(), nil)
	// A branch-pinned git url avoids remote default-branch resolution; no path,
	// so the allowlist does not apply.
	req := AddRequest{Source: config.SourceEntry{Type: "git", URL: "https://example.com/x.git", Branch: "main"}}

	rec := doJSON(t, mux, http.MethodPost, "/source-manifest/sources", req, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var reply AddReply
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if reply.Error != nil {
		t.Fatalf("unexpected error: %+v", reply.Error)
	}
	if len(reply.Components) == 0 {
		t.Fatal("expected at least one spawned component")
	}
}

func TestHandleAddHTTP_PathSourceAllowedRoot(t *testing.T) {
	cfg := stubIngestCfg()
	cfg.AllowedRoots = []string{"/mnt/workspace"}
	mux := newHTTPComponent(t, cfg, nil)

	req := AddRequest{Source: config.SourceEntry{Type: "docs", Paths: []string{"/mnt/workspace/docs"}}}
	rec := doJSON(t, mux, http.MethodPost, "/source-manifest/sources", req, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleAddHTTP_PathOutsideAllowlistRejected(t *testing.T) {
	cfg := stubIngestCfg()
	cfg.AllowedRoots = []string{"/mnt/workspace"}
	mux := newHTTPComponent(t, cfg, nil)

	req := AddRequest{Source: config.SourceEntry{Type: "docs", Paths: []string{"/etc/secrets"}}}
	rec := doJSON(t, mux, http.MethodPost, "/source-manifest/sources", req, nil)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleAddHTTP_PathNoAllowlistRejected(t *testing.T) {
	// No AllowedRoots configured → any path-based HTTP add is refused.
	mux := newHTTPComponent(t, stubIngestCfg(), nil)

	req := AddRequest{Source: config.SourceEntry{Type: "docs", Paths: []string{"/mnt/workspace/docs"}}}
	rec := doJSON(t, mux, http.MethodPost, "/source-manifest/sources", req, nil)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleAddHTTP_Unauthorized(t *testing.T) {
	cfg := stubIngestCfg()
	cfg.APIToken = "s3cret"
	mux := newHTTPComponent(t, cfg, nil)

	req := AddRequest{Source: config.SourceEntry{Type: "git", URL: "https://example.com/x.git", Branch: "main"}}

	// Missing token.
	if rec := doJSON(t, mux, http.MethodPost, "/source-manifest/sources", req, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: status = %d, want 401", rec.Code)
	}
	// Wrong token.
	if rec := doJSON(t, mux, http.MethodPost, "/source-manifest/sources", req, map[string]string{"Authorization": "Bearer nope"}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d, want 401", rec.Code)
	}
	// Correct token passes auth (200).
	if rec := doJSON(t, mux, http.MethodPost, "/source-manifest/sources", req, map[string]string{"Authorization": "Bearer s3cret"}); rec.Code != http.StatusOK {
		t.Fatalf("correct token: status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleAddHTTP_NotReady(t *testing.T) {
	// ingestCfg nil = host has not wired RegisterIngestHandlers yet.
	mux := newHTTPComponent(t, nil, nil)

	req := AddRequest{Source: config.SourceEntry{Type: "git", URL: "https://example.com/x.git", Branch: "main"}}
	rec := doJSON(t, mux, http.MethodPost, "/source-manifest/sources", req, nil)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

// --- DELETE /sources/{id} --------------------------------------------------

func TestHandleRemoveHTTP(t *testing.T) {
	cfg := stubIngestCfg()
	cfg.Store = stubStore{components: []string{"doc-source-abc"}}
	mux := newHTTPComponent(t, cfg, nil)

	rec := doJSON(t, mux, http.MethodDelete, "/source-manifest/sources/doc-source-abc", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var reply RemoveReply
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if !reply.Removed || reply.InstanceName != "doc-source-abc" {
		t.Fatalf("unexpected reply: %+v", reply)
	}
}

// TestHandleRemoveHTTP_UnknownHandle pins honest removal over HTTP: an
// unregistered handle is 404 NOT_FOUND, never removed:true (audit 2026-07-19).
func TestHandleRemoveHTTP_UnknownHandle(t *testing.T) {
	mux := newHTTPComponent(t, stubIngestCfg(), nil)

	rec := doJSON(t, mux, http.MethodDelete, "/source-manifest/sources/nope-source", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
	var reply RemoveReply
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if reply.Removed || reply.Error == nil || reply.Error.Code != CodeNotFound {
		t.Fatalf("unexpected reply: %+v", reply)
	}
}

// --- GET /sources/{id} -----------------------------------------------------

func TestHandleSourceHTTP_NotFound(t *testing.T) {
	mux := newHTTPComponent(t, stubIngestCfg(), nil)
	rec := doJSON(t, mux, http.MethodGet, "/source-manifest/sources/nope", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleSourceHTTP_ReturnsHandle(t *testing.T) {
	// Register a docs source in the manifest, then resolve its instance name via
	// the same sourcespawn.Build the handler uses, and GET that handle.
	src := config.SourceEntry{Type: "docs", Paths: []string{"/mnt/workspace/docs"}, Watch: false}
	built, err := sourcespawn.Build(src, sourcespawn.Options{Org: "acme"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var instance string
	for name := range built {
		instance = name
	}
	if instance == "" {
		t.Fatal("no instance built for docs source")
	}

	mux := newHTTPComponent(t, stubIngestCfg(), []ManifestSource{sourceEntryToManifestSource(src)})
	rec := doJSON(t, mux, http.MethodGet, "/source-manifest/sources/"+instance, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var h SourceHandle
	if err := json.Unmarshal(rec.Body.Bytes(), &h); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	if h.InstanceName != instance || h.SourceType != "docs" || h.Mode != "snapshot" {
		t.Fatalf("unexpected handle: %+v", h)
	}
}

// --- unit: allowlist + auth ------------------------------------------------

func TestAuthorized(t *testing.T) {
	req := func(auth string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/x", nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		return r
	}
	cases := []struct {
		name  string
		token string
		auth  string
		want  bool
	}{
		{"no token configured allows all", "", "", true},
		{"no token allows even with header", "", "Bearer whatever", true},
		{"correct bearer", "s3cret", "Bearer s3cret", true},
		{"wrong bearer", "s3cret", "Bearer nope", false},
		{"missing header", "s3cret", "", false},
		{"no bearer prefix", "s3cret", "s3cret", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := authorized(req(tc.auth), tc.token); got != tc.want {
				t.Fatalf("authorized = %v, want %v", got, tc.want)
			}
		})
	}
}
