package urlhandler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	urlhandler "github.com/c360studio/semsource/handler/url"
	"github.com/c360studio/semsource/handler"
)

// stubSourceConfig adapts test values to handler.SourceConfig.
type stubSourceConfig struct {
	sourceType   string
	url          string
	watch        bool
	pollInterval string
}

func (s *stubSourceConfig) GetType() string            { return s.sourceType }
func (s *stubSourceConfig) GetPath() string            { return "" }
func (s *stubSourceConfig) GetPaths() []string         { return nil }
func (s *stubSourceConfig) GetURL() string             { return s.url }
func (s *stubSourceConfig) GetBranch() string          { return "" }
func (s *stubSourceConfig) IsWatchEnabled() bool       { return s.watch }
func (s *stubSourceConfig) GetKeyframeMode() string    { return "" }
func (s *stubSourceConfig) GetKeyframeInterval() string { return "" }
func (s *stubSourceConfig) GetSceneThreshold() float64 { return 0 }

var _ handler.SourceHandler = (*urlhandler.URLHandler)(nil)

func TestURLHandler_SourceType(t *testing.T) {
	h := urlhandler.New(nil)
	if got := h.SourceType(); got != handler.SourceTypeURL {
		t.Errorf("SourceType() = %q, want %q", got, handler.SourceTypeURL)
	}
}

func TestURLHandler_Supports(t *testing.T) {
	h := urlhandler.New(nil)

	tests := []struct {
		name string
		cfg  handler.SourceConfig
		want bool
	}{
		{
			name: "url type with https url",
			cfg:  &stubSourceConfig{sourceType: "url", url: "https://example.com"},
			want: true,
		},
		{
			name: "wrong type",
			cfg:  &stubSourceConfig{sourceType: "git", url: "https://example.com"},
			want: false,
		},
		{
			name: "empty url",
			cfg:  &stubSourceConfig{sourceType: "url", url: ""},
			want: false,
		},
		{
			name: "http url rejected (no TLS)",
			cfg:  &stubSourceConfig{sourceType: "url", url: "http://example.com"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.Supports(tt.cfg); got != tt.want {
				t.Errorf("Supports() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestURLHandler_Ingest_FetchesContent(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("ETag", `"abc123"`)
		_, _ = w.Write([]byte("<html><body>Hello SemSource</body></html>"))
	}))
	defer srv.Close()

	h := urlhandler.NewWithClient(nil, srv.Client())
	cfg := &stubSourceConfig{sourceType: "url", url: srv.URL}
	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("expected at least one entity from URL")
	}

	e := entities[0]
	if e.Domain != handler.DomainWeb {
		t.Errorf("Domain = %q, want %q", e.Domain, handler.DomainWeb)
	}
	if e.EntityType != "page" {
		t.Errorf("EntityType = %q, want %q", e.EntityType, "page")
	}
	if e.Instance == "" {
		t.Error("Instance must not be empty")
	}
	if e.System == "" {
		t.Error("System must not be empty")
	}

	// ETag stored in properties
	if e.Properties["etag"] == "" {
		t.Error("expected etag in properties")
	}
}

func TestURLHandler_Ingest_ContentHashInProperties(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("stable content"))
	}))
	defer srv.Close()

	h := urlhandler.NewWithClient(nil, srv.Client())
	cfg := &stubSourceConfig{sourceType: "url", url: srv.URL}
	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("expected at least one entity")
	}

	hash, ok := entities[0].Properties["content_hash"]
	if !ok {
		t.Fatal("expected content_hash in properties")
	}
	if hash == "" {
		t.Error("content_hash must not be empty")
	}
}

func TestURLHandler_Ingest_InstanceDeterministic(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	h := urlhandler.NewWithClient(nil, srv.Client())
	cfg := &stubSourceConfig{sourceType: "url", url: srv.URL}

	entities1, _ := h.Ingest(context.Background(), cfg)
	entities2, _ := h.Ingest(context.Background(), cfg)

	if len(entities1) == 0 || len(entities2) == 0 {
		t.Skip("no entities returned")
	}
	if entities1[0].Instance != entities2[0].Instance {
		t.Errorf("Instance not deterministic: %q vs %q", entities1[0].Instance, entities2[0].Instance)
	}
}

func TestURLHandler_Ingest_ContextCancelled(t *testing.T) {
	// Server that hangs to trigger context cancellation
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	h := urlhandler.NewWithClient(nil, srv.Client())
	cfg := &stubSourceConfig{sourceType: "url", url: srv.URL}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := h.Ingest(ctx, cfg)
	if err == nil {
		t.Error("expected error on cancelled context")
	}
}

func TestURLHandler_Ingest_HTTP404(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	h := urlhandler.NewWithClient(nil, srv.Client())
	cfg := &stubSourceConfig{sourceType: "url", url: srv.URL}
	_, err := h.Ingest(context.Background(), cfg)
	if err == nil {
		t.Error("expected error on HTTP 404")
	}
}

func TestURLHandler_Watch_NilWhenDisabled(t *testing.T) {
	h := urlhandler.New(nil)
	cfg := &stubSourceConfig{sourceType: "url", url: "https://example.com", watch: false}
	ch, err := h.Watch(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if ch != nil {
		t.Error("Watch should return nil channel when watch disabled")
	}
}

func TestURLHandler_Watch_PollsAndEmitsOnChange(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			_, _ = w.Write([]byte("initial content"))
		} else {
			_, _ = w.Write([]byte("changed content"))
		}
	}))
	defer srv.Close()

	h := urlhandler.NewWithClient(nil, srv.Client())
	cfg := &stubURLSourceConfig{
		stubSourceConfig: stubSourceConfig{
			sourceType: "url",
			url:        srv.URL,
			watch:      true,
		},
		pollInterval: "100ms",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := h.Watch(ctx, cfg)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if ch == nil {
		t.Fatal("Watch returned nil channel with watch enabled")
	}

	// Wait for first event (change from initial → changed)
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("Watch channel closed unexpectedly")
		}
		if ev.Operation == "" {
			t.Error("ChangeEvent.Operation must not be empty")
		}
		if ev.Path == "" {
			t.Error("ChangeEvent.Path must not be empty")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for watch event")
	}
}

func TestURLHandler_Watch_NoEventWhenContentUnchanged(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		_, _ = w.Write([]byte("same content every time"))
	}))
	defer srv.Close()

	h := urlhandler.NewWithClient(nil, srv.Client())
	cfg := &stubURLSourceConfig{
		stubSourceConfig: stubSourceConfig{
			sourceType: "url",
			url:        srv.URL,
			watch:      true,
		},
		pollInterval: "50ms",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	ch, err := h.Watch(ctx, cfg)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if ch == nil {
		t.Fatal("Watch returned nil channel")
	}

	// Let it poll at least 3 times; expect no events (content unchanged)
	time.Sleep(300 * time.Millisecond)
	cancel()

	var events int
	for ev := range ch {
		_ = ev
		events++
	}
	// The initial fetch may emit one event; subsequent identical fetches should not.
	// We allow at most 1 (the initial seed event from Watch).
	if events > 1 {
		t.Errorf("expected at most 1 event for unchanged content, got %d", events)
	}
}

func TestURLHandler_Watch_ChannelClosedOnContextCancel(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()

	h := urlhandler.NewWithClient(nil, srv.Client())
	cfg := &stubURLSourceConfig{
		stubSourceConfig: stubSourceConfig{
			sourceType: "url",
			url:        srv.URL,
			watch:      true,
		},
		pollInterval: "50ms",
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := h.Watch(ctx, cfg)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if ch == nil {
		t.Fatal("Watch returned nil channel")
	}

	// Let it run briefly then cancel
	time.Sleep(80 * time.Millisecond)
	cancel()

	timeout := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // channel closed as expected
			}
		case <-timeout:
			t.Fatal("Watch channel not closed after context cancellation")
		}
	}
}

// stubURLSourceConfig extends stubSourceConfig with a configurable poll interval.
type stubURLSourceConfig struct {
	stubSourceConfig
	pollInterval string
}

func (s *stubURLSourceConfig) GetPollInterval() string { return s.pollInterval }
