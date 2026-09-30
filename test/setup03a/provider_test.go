//go:build qualification

package setup03a

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestProviderFailureAndRecovery injects transport faults through an owned proxy.
// Successful calls still go to exactly the frozen, real embedding provider.
func TestProviderFailureAndRecovery(t *testing.T) {
	if os.Getenv("SETUP03A_PROFILE") != "neural" {
		t.Skip("provider fault qualification is only selected by the neural profile")
	}
	binary, out, endpoint := os.Getenv("SETUP03A_BINARY"), os.Getenv("SETUP03A_OUT"), os.Getenv("SETUP03A_PROVIDER")
	if binary == "" || out == "" || endpoint == "" {
		t.Fatal("neural provider qualification requires SETUP03A_BINARY, SETUP03A_OUT and SETUP03A_PROVIDER")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	h := &harness{t: t, ctx: ctx, out: filepath.Join(out, "provider-faults"), binary: binary, ids: map[string]string{}}
	if err := os.MkdirAll(h.out, 0755); err != nil {
		t.Fatal(err)
	}
	h.result = report{CorpusVersion: "1", Profile: "neural-provider-faults", SemEngineSlices: []int{2}, Started: time.Now().UTC()}
	defer h.finish()
	raw, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	h.result.BinarySHA256 = hex.EncodeToString(sum[:])
	h.fixture()
	h.broker()
	h.configure("neural")
	target, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	target.Path = ""
	reverse := httputil.NewSingleHostReverseProxy(target)
	var mode atomic.Int32
	var failed, delayed atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode.Load() {
		case 1:
			failed.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"qualification provider unavailable"}}`))
		case 2:
			delayed.Add(1)
			_, _ = io.Copy(io.Discard, r.Body)
			// This delay is the injected fault, not a readiness synchronizer.
			// Bound it independently even if a client does not propagate cancellation.
			timer := time.NewTimer(8 * time.Second)
			defer timer.Stop()
			select {
			case <-r.Context().Done():
			case <-timer.C:
			}

		default:
			reverse.ServeHTTP(w, r)
		}
	}))
	defer func() { proxy.CloseClientConnections(); proxy.Close() }()
	h.result.Configuration["model_registry"].(map[string]any)["endpoints"].(map[string]any)["semembed"].(map[string]any)["url"] = proxy.URL + "/v1"
	raw, err = json.MarshalIndent(h.result.Configuration, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	h.save("semsource.json", raw)
	h.providerMetrics("before")
	h.start()
	h.check("real_provider_initial_passage", 60*time.Second, func() (any, error) {
		f, queryErr := h.fuse("doc", "context", "cobalt lantern routing", 10)
		if queryErr != nil {
			return f, queryErr
		}
		for _, n := range f.Nodes {
			if strings.Contains(n.Body, "north depot") {
				return f, nil
			}
		}
		return f, errors.New("real provider has not returned expected passage")
	})
	mode.Store(1)
	h.check("provider_unavailable_is_not_absence", time.Second, func() (any, error) {
		f, queryErr := h.fuse("doc", "context", "blue parcel transfer unavailable-provider unique probe", 10)
		detail := map[string]any{"response": f, "injected_requests": failed.Load(), "transport_error": fmt.Sprint(queryErr)}
		if failed.Load() == 0 {
			return detail, errors.New("fault did not reach the provider; cached answers do not qualify outage")
		}
		if queryErr == nil || len(f.Misses) > 0 {
			return detail, errors.New("provider outage became a successful absence instead of classified failure/defer")
		}
		return detail, nil
	})
	mode.Store(2)
	h.check("provider_deadline_is_bounded_and_not_absence", time.Second, func() (any, error) {
		started := time.Now()
		f, queryErr := h.fuse("doc", "context", "northern dispatch deadline-provider distinct probe", 10)
		elapsed := time.Since(started)
		detail := map[string]any{"response": f, "injected_requests": delayed.Load(), "transport_error": fmt.Sprint(queryErr), "elapsed_ms": elapsed.Milliseconds()}
		if delayed.Load() == 0 {
			return detail, errors.New("deadline did not reach the provider")
		}
		if queryErr == nil || len(f.Misses) > 0 || elapsed > 7*time.Second {
			return detail, errors.New("provider deadline was unbounded or became successful absence")
		}
		return detail, nil
	})
	mode.Store(0)
	h.check("provider_recovery_returns_exact_passage", 30*time.Second, func() (any, error) {
		f, queryErr := h.fuse("doc", "context", "cobalt lantern blue parcels north depot recovery", 10)
		if queryErr != nil {
			return f, queryErr
		}
		expected, readErr := os.ReadFile(filepath.Join(h.source, "docs/routing.md"))
		if readErr != nil {
			return f, readErr
		}
		for _, n := range f.Nodes {
			if n.Body == string(expected) {
				return f, nil
			}
		}
		return f, errors.New("provider recovery did not return exact passage")
	})
	h.providerMetrics("after_recovery")
}
