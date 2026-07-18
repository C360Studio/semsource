package codecontext

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/fusion"
	"github.com/nats-io/nats.go"

	"github.com/c360studio/semsource/source/fusion/fusiontest"
)

type errorGraph struct {
	*fusiontest.MemGraph
	statusErr error
}

func (g *errorGraph) Status(context.Context) (fusion.IndexStatus, error) {
	if g.statusErr != nil {
		return fusion.IndexStatus{}, g.statusErr
	}
	return g.MemGraph.Status(context.Background())
}

type trackingResponseWriter struct {
	header http.Header
	writes int
}

func (w *trackingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *trackingResponseWriter) Write(p []byte) (int, error) {
	w.writes++
	return len(p), nil
}

func (w *trackingResponseWriter) WriteHeader(int) { w.writes++ }

type cancelingReader struct {
	cancel context.CancelFunc
}

func (r *cancelingReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, errors.New("request body read failed")
}

func TestFusionHTTPErrorContract_LocalRequestFailures(t *testing.T) {
	c := newTestComponent("code", fusiontest.NewMemGraph(), fusiontest.NewMemStore())
	tests := []struct {
		name      string
		method    string
		body      io.Reader
		running   bool
		status    int
		code      string
		class     string
		retryable bool
	}{
		{"method", http.MethodGet, nil, true, 405, "method_not_allowed", "invalid", false},
		{"oversized", http.MethodPost, strings.NewReader(strings.Repeat("x", maxBodyBytes+1)), true, 413, "request_too_large", "invalid", false},
		{"invalid JSON", http.MethodPost, strings.NewReader(`{"query":`), true, 400, "invalid_json", "invalid", false},
		{"blank query", http.MethodPost, strings.NewReader(`{"query":"  "}`), true, 400, "invalid_request", "invalid", false},
		{"not started", http.MethodPost, strings.NewReader(`{"query":"Dispatch"}`), false, 503, "component_not_ready", "transient", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.running = tt.running
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, "/code-context/context", tt.body)
			c.handleHTTP(rec, req, "context")
			assertHTTPError(t, rec, tt.status, tt.code, tt.class, tt.retryable)
			if tt.status == http.StatusMethodNotAllowed && rec.Header().Get("Allow") != http.MethodPost {
				t.Fatalf("Allow = %q, want POST", rec.Header().Get("Allow"))
			}
		})
	}
}

func TestFusionHTTPErrorContract_DependencyFailures(t *testing.T) {
	secret := "private.subject.entity.storage-key"
	tests := []struct {
		name      string
		err       error
		status    int
		code      string
		retryable bool
	}{
		{"no responders", nats.ErrNoResponders, 503, "dependency_unavailable", true},
		{"not connected", natsclient.ErrNotConnected, 503, "dependency_unavailable", true},
		{"circuit open", natsclient.ErrCircuitOpen, 503, "dependency_unavailable", true},
		{"transient", errs.Classified(errs.ErrorTransient, errors.New(secret)), 503, "dependency_unavailable", true},
		{"deadline", context.DeadlineExceeded, 504, "upstream_timeout", true},
		{"nats timeout", nats.ErrTimeout, 504, "upstream_timeout", true},
		{"invalid upstream", errs.Classified(errs.ErrorInvalid, errors.New(secret)), 502, "upstream_contract_error", false},
		{"upstream JSON syntax", fmt.Errorf("decode status: %w", &json.SyntaxError{Offset: 1}), 502, "upstream_contract_error", false},
		{"upstream JSON type", fmt.Errorf("decode status: %w", &json.UnmarshalTypeError{Value: "number", Type: reflect.TypeFor[string]()}), 502, "upstream_contract_error", false},
		{"fatal upstream", errs.Classified(errs.ErrorFatal, errors.New(secret)), 502, "upstream_failure", false},
		{"unclassified upstream", errors.New(secret), 502, "upstream_failure", false},
		{"unclassified timeout words", errors.New("temporary network timeout unavailable"), 502, "upstream_failure", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			g := &errorGraph{MemGraph: fusiontest.NewMemGraph(), statusErr: tt.err}
			c := newTestComponent("code", g, fusiontest.NewMemStore())
			c.logger = slog.New(slog.NewTextHandler(&logs, nil))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/code-context/context", strings.NewReader(`{"query":"Dispatch"}`))
			c.handleHTTP(rec, req, "context")
			assertHTTPError(t, rec, tt.status, tt.code, map[bool]string{true: "transient", false: "fatal"}[tt.retryable], tt.retryable)
			if strings.Contains(rec.Body.String(), secret) || strings.Contains(logs.String(), secret) {
				t.Fatalf("private cause leaked: body=%q logs=%q", rec.Body.String(), logs.String())
			}
			for _, field := range []string{
				"route=/code-context/context",
				"verb=context",
				"code=" + tt.code,
				"class=" + map[bool]string{true: "transient", false: "fatal"}[tt.retryable],
				"origin=dependency",
			} {
				if !strings.Contains(logs.String(), field) {
					t.Errorf("safe log context missing %q: %s", field, logs.String())
				}
			}
		})
	}
}

func TestFusionHTTPErrorContract_LocalFailures(t *testing.T) {
	secret := "private local assembly failure"
	var logs bytes.Buffer
	c := newTestComponent("code", fusiontest.NewMemGraph(), fusiontest.NewMemStore())
	c.logger = slog.New(slog.NewTextHandler(&logs, nil))
	c.marshalHTTPResponse = func(fusion.Response) ([]byte, error) {
		return nil, errors.New(secret)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/code-context/context", strings.NewReader(`{"query":"Dispatch"}`))
	c.handleHTTP(rec, req, "context")
	assertHTTPError(t, rec, 500, "internal_error", "fatal", false)
	if strings.Contains(rec.Body.String(), secret) || strings.Contains(logs.String(), secret) {
		t.Fatalf("private cause leaked: body=%q logs=%q", rec.Body.String(), logs.String())
	}
	for _, field := range []string{"route=/code-context/context", "verb=context", "code=internal_error", "class=fatal", "origin=local"} {
		if !strings.Contains(logs.String(), field) {
			t.Errorf("safe log context missing %q: %s", field, logs.String())
		}
	}
}

func TestFusionHTTPErrorContract_SuccessHonestyStates(t *testing.T) {
	t.Run("not ready", func(t *testing.T) {
		g := fusiontest.NewMemGraph()
		g.SetStatus(fusion.IndexStatus{Ready: false, State: fusion.StateBuilding})
		c := newTestComponent("code", g, fusiontest.NewMemStore())
		rec := httptest.NewRecorder()
		c.handleHTTP(rec, httptest.NewRequest(http.MethodPost, "/code-context/context", strings.NewReader(`{"query":"Dispatch"}`)), "context")
		if rec.Code != 200 {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		if resp := decodeResp(t, rec.Body.Bytes()); resp.Index.Ready || resp.ContractVersion == "" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("ready miss", func(t *testing.T) {
		g := fusiontest.NewMemGraph()
		g.SetStatus(fusion.IndexStatus{Ready: true, State: fusion.StateReady})
		c := newTestComponent("code", g, fusiontest.NewMemStore())
		rec := httptest.NewRecorder()
		c.handleHTTP(rec, httptest.NewRequest(http.MethodPost, "/code-context/context", strings.NewReader(`{"query":"Missing"}`)), "context")
		if rec.Code != 200 {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		if resp := decodeResp(t, rec.Body.Bytes()); len(resp.Misses) != 1 {
			t.Fatalf("misses = %+v, want one", resp.Misses)
		}
	})

	t.Run("unknown fields remain compatible", func(t *testing.T) {
		g := fusiontest.NewMemGraph()
		g.SetStatus(fusion.IndexStatus{Ready: true, State: fusion.StateReady})
		c := newTestComponent("code", g, fusiontest.NewMemStore())
		rec := httptest.NewRecorder()
		body := strings.NewReader(`{"query":"Missing","future_option":{"enabled":true}}`)
		c.handleHTTP(rec, httptest.NewRequest(http.MethodPost, "/code-context/context", body), "context")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestFusionHTTPErrorContract_BlankQueryStopsBeforeDependency(t *testing.T) {
	g := &errorGraph{MemGraph: fusiontest.NewMemGraph(), statusErr: errors.New("dependency must not run")}
	c := newTestComponent("code", g, fusiontest.NewMemStore())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/code-context/context", strings.NewReader(`{"query":" "}`))
	c.handleHTTP(rec, req, "context")
	assertHTTPError(t, rec, 400, "invalid_request", "invalid", false)
}

func TestFusionHTTPErrorContract_CallerCancellationDoesNotWrite(t *testing.T) {
	t.Run("during dependency", func(t *testing.T) {
		g := &errorGraph{MemGraph: fusiontest.NewMemGraph(), statusErr: context.Canceled}
		c := newTestComponent("code", g, fusiontest.NewMemStore())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodPost, "/code-context/context", strings.NewReader(`{"query":"Dispatch"}`)).WithContext(ctx)
		rec := &trackingResponseWriter{}
		c.handleHTTP(rec, req, "context")
		if rec.writes != 0 {
			t.Fatalf("writes after cancellation = %d, want 0", rec.writes)
		}
	})

	t.Run("during body read", func(t *testing.T) {
		c := newTestComponent("code", fusiontest.NewMemGraph(), fusiontest.NewMemStore())
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest(http.MethodPost, "/code-context/context", &cancelingReader{cancel: cancel}).WithContext(ctx)
		rec := &trackingResponseWriter{}
		c.handleHTTP(rec, req, "context")
		if rec.writes != 0 {
			t.Fatalf("writes after canceled body read = %d, want 0", rec.writes)
		}
	})
}

func TestFusionHTTPErrorContract_AllRegisteredRoutes(t *testing.T) {
	for _, lensKind := range []string{"code", "docs"} {
		t.Run(lensKind, func(t *testing.T) {
			c := newTestComponent(lensKind, fusiontest.NewMemGraph(), fusiontest.NewMemStore())
			mux := http.NewServeMux()
			prefix := "/" + map[string]string{"code": "code-context", "docs": "doc-context"}[lensKind]
			c.RegisterHTTPHandlers(prefix, mux)
			for _, verb := range verbs {
				t.Run(verb, func(t *testing.T) {
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodPost, prefix+"/"+verb, strings.NewReader(`{"query":`))
					mux.ServeHTTP(rec, req)
					assertHTTPError(t, rec, 400, "invalid_json", "invalid", false)

					rec = httptest.NewRecorder()
					req = httptest.NewRequest(http.MethodPost, prefix+"/"+verb, strings.NewReader(`{"query":"status"}`))
					mux.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Fatalf("success status = %d, want 200: %s", rec.Code, rec.Body.String())
					}
					if resp := decodeResp(t, rec.Body.Bytes()); resp.ContractVersion == "" {
						t.Fatalf("unexpected success response: %+v", resp)
					}
				})
			}
		})
	}
}

func TestFusionHTTPErrorContract_PublicHandlers(t *testing.T) {
	for _, lensKind := range []string{"code", "docs"} {
		t.Run(lensKind, func(t *testing.T) {
			g := &errorGraph{MemGraph: fusiontest.NewMemGraph()}
			g.SetStatus(fusion.IndexStatus{Ready: false, State: fusion.StateBuilding})
			c := newTestComponent(lensKind, g, fusiontest.NewMemStore())
			mux := http.NewServeMux()
			prefix := "/" + map[string]string{"code": "code-context", "docs": "doc-context"}[lensKind]
			c.RegisterHTTPHandlers(prefix, mux)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, prefix+"/context", strings.NewReader(`{"query":"status"}`))
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
			}
			if resp := decodeResp(t, rec.Body.Bytes()); resp.Index.Ready || resp.ContractVersion == "" {
				t.Fatalf("unexpected response: %+v", resp)
			}

			rec = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, prefix+"/context", strings.NewReader(`{"query":`))
			mux.ServeHTTP(rec, req)
			assertHTTPError(t, rec, 400, "invalid_json", "invalid", false)

			secret := "private public dependency failure"
			g.statusErr = errors.New(secret)
			rec = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, prefix+"/context", strings.NewReader(`{"query":"status"}`))
			mux.ServeHTTP(rec, req)
			assertHTTPError(t, rec, 502, "upstream_failure", "fatal", false)
			if strings.Contains(rec.Body.String(), secret) {
				t.Fatalf("private dependency cause leaked: %s", rec.Body.String())
			}
		})
	}
}

func assertHTTPError(t *testing.T, rec *httptest.ResponseRecorder, status int, code, class string, retryable bool) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d: %s", rec.Code, status, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var body struct {
		Error struct {
			ContractVersion string `json:"contract_version"`
			Code            string `json:"code"`
			Class           string `json:"class"`
			Message         string `json:"message"`
			Retryable       bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error envelope: %v: %s", err, rec.Body.String())
	}
	if body.Error.ContractVersion != "1" || body.Error.Code != code || body.Error.Class != class ||
		body.Error.Message == "" || body.Error.Retryable != retryable {
		t.Fatalf("error envelope = %+v", body.Error)
	}
}
