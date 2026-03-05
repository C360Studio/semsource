package urlhandler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	urlhandler "github.com/c360studio/semsource/handler/url"
)

func TestSafeFetcher_FetchReturnsBody(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"etag-v1"`)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	f := urlhandler.NewTestFetcher(srv.Client())
	result, err := f.Fetch(context.Background(), srv.URL, "")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(result.Body) != "hello world" {
		t.Errorf("Body = %q, want %q", string(result.Body), "hello world")
	}
	if result.ETag != `"etag-v1"` {
		t.Errorf("ETag = %q, want %q", result.ETag, `"etag-v1"`)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusOK)
	}
}

func TestSafeFetcher_FetchWithETag_NotModified(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"etag-v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"etag-v1"`)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	f := urlhandler.NewTestFetcher(srv.Client())
	result, err := f.Fetch(context.Background(), srv.URL, `"etag-v1"`)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if result.StatusCode != http.StatusNotModified {
		t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusNotModified)
	}
	if result.Body != nil {
		t.Error("Body should be nil for 304")
	}
}

func TestSafeFetcher_FetchRejectsHTTP(t *testing.T) {
	// HTTP (non-TLS) server — fetcher should reject the URL before dialing
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("should not reach here"))
	}))
	defer srv.Close()

	// Use nil client — we expect rejection at URL validation before making a connection
	f := urlhandler.NewSafeFetcher(nil)
	_, err := f.Fetch(context.Background(), srv.URL, "")
	if err == nil {
		t.Error("expected error for http:// URL (not https)")
	}
}

func TestSafeFetcher_FetchRejectsLocalhost(t *testing.T) {
	f := urlhandler.NewSafeFetcher(nil)
	_, err := f.Fetch(context.Background(), "https://localhost/path", "")
	if err == nil {
		t.Error("expected error for localhost URL")
	}
}

func TestSafeFetcher_FetchRejectsPrivateIP(t *testing.T) {
	f := urlhandler.NewSafeFetcher(nil)
	_, err := f.Fetch(context.Background(), "https://192.168.1.1/api", "")
	if err == nil {
		t.Error("expected error for private IP URL")
	}
}

func TestSafeFetcher_FetchHTTPError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := urlhandler.NewTestFetcher(srv.Client())
	_, err := f.Fetch(context.Background(), srv.URL, "")
	if err == nil {
		t.Error("expected error on HTTP 500")
	}
}

func TestSafeFetcher_FetchContextCancelled(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	f := urlhandler.NewTestFetcher(srv.Client())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := f.Fetch(ctx, srv.URL, "")
	if err == nil {
		t.Error("expected error on cancelled context")
	}
}
