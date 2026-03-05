package handler_test

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semsource/handler"
)

// mockConfig implements SourceConfig for testing.
type mockConfig struct {
	sourceType   string
	path         string
	url          string
	watchEnabled bool
}

func (m *mockConfig) GetType() string       { return m.sourceType }
func (m *mockConfig) GetPath() string       { return m.path }
func (m *mockConfig) GetURL() string        { return m.url }
func (m *mockConfig) IsWatchEnabled() bool  { return m.watchEnabled }

// mockHandler implements SourceHandler for interface compliance testing.
type mockHandler struct {
	sourceType    string
	ingestResult  []handler.RawEntity
	ingestErr     error
	watchCh       chan handler.ChangeEvent
	watchErr      error
	supportsResult bool
}

func (m *mockHandler) SourceType() string { return m.sourceType }

func (m *mockHandler) Ingest(_ context.Context, _ handler.SourceConfig) ([]handler.RawEntity, error) {
	return m.ingestResult, m.ingestErr
}

func (m *mockHandler) Watch(_ context.Context, _ handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	if m.watchCh == nil {
		return nil, m.watchErr
	}
	return m.watchCh, m.watchErr
}

func (m *mockHandler) Supports(_ handler.SourceConfig) bool { return m.supportsResult }

// Compile-time assertion that mockHandler satisfies SourceHandler.
var _ handler.SourceHandler = (*mockHandler)(nil)

// Compile-time assertion that mockConfig satisfies SourceConfig.
var _ handler.SourceConfig = (*mockConfig)(nil)

func TestSourceHandler_Interface(t *testing.T) {
	t.Run("handler returns correct source type", func(t *testing.T) {
		h := &mockHandler{sourceType: "ast"}
		if h.SourceType() != "ast" {
			t.Errorf("expected SourceType=ast, got %s", h.SourceType())
		}
	})

	t.Run("Supports returns false for incompatible config", func(t *testing.T) {
		h := &mockHandler{sourceType: "git", supportsResult: false}
		cfg := &mockConfig{sourceType: "url"}
		if h.Supports(cfg) {
			t.Error("expected Supports=false for mismatched type")
		}
	})

	t.Run("Supports returns true for compatible config", func(t *testing.T) {
		h := &mockHandler{sourceType: "git", supportsResult: true}
		cfg := &mockConfig{sourceType: "git"}
		if !h.Supports(cfg) {
			t.Error("expected Supports=true for matching type")
		}
	})

	t.Run("Ingest returns entities", func(t *testing.T) {
		entities := []handler.RawEntity{
			{
				SourceType: "git",
				Domain:     "git",
				System:     "github.com-acme-gcs",
				EntityType: "commit",
				Instance:   "a3f9b2",
			},
		}
		h := &mockHandler{sourceType: "git", ingestResult: entities}
		cfg := &mockConfig{sourceType: "git", path: "/repo"}

		result, err := h.Ingest(context.Background(), cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("expected 1 entity, got %d", len(result))
		}
		if result[0].Instance != "a3f9b2" {
			t.Errorf("expected Instance=a3f9b2, got %s", result[0].Instance)
		}
	})

	t.Run("Ingest respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // pre-cancel

		h := &mockHandler{sourceType: "ast", ingestErr: ctx.Err()}
		cfg := &mockConfig{sourceType: "ast"}

		_, err := h.Ingest(ctx, cfg)
		if err == nil {
			t.Error("expected error from cancelled context, got nil")
		}
	})

	t.Run("Watch returns nil for non-watching handler", func(t *testing.T) {
		h := &mockHandler{sourceType: "url"} // watchCh nil by default
		cfg := &mockConfig{sourceType: "url", watchEnabled: false}

		ch, err := h.Watch(context.Background(), cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch != nil {
			t.Error("expected nil channel for non-watching handler")
		}
	})

	t.Run("Watch returns channel for watching handler", func(t *testing.T) {
		watchCh := make(chan handler.ChangeEvent, 1)
		h := &mockHandler{sourceType: "ast", watchCh: watchCh}
		cfg := &mockConfig{sourceType: "ast", path: "/project", watchEnabled: true}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := h.Watch(ctx, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch == nil {
			t.Fatal("expected non-nil channel for watching handler")
		}

		// Send an event through the channel.
		watchCh <- handler.ChangeEvent{
			Path:      "/project/main.go",
			Operation: handler.OperationModify,
			Timestamp: time.Now(),
		}

		select {
		case ev := <-ch:
			if ev.Path != "/project/main.go" {
				t.Errorf("unexpected Path: %s", ev.Path)
			}
			if ev.Operation != handler.OperationModify {
				t.Errorf("unexpected Operation: %s", ev.Operation)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("timed out waiting for change event")
		}
	})
}

func TestSourceConfig_Interface(t *testing.T) {
	t.Run("filesystem config", func(t *testing.T) {
		cfg := &mockConfig{
			sourceType:   "ast",
			path:         "/project/src",
			watchEnabled: true,
		}
		if cfg.GetType() != "ast" {
			t.Errorf("expected type=ast, got %s", cfg.GetType())
		}
		if cfg.GetPath() != "/project/src" {
			t.Errorf("expected path=/project/src, got %s", cfg.GetPath())
		}
		if cfg.GetURL() != "" {
			t.Errorf("expected empty URL, got %s", cfg.GetURL())
		}
		if !cfg.IsWatchEnabled() {
			t.Error("expected watch enabled")
		}
	})

	t.Run("URL config", func(t *testing.T) {
		cfg := &mockConfig{
			sourceType:   "url",
			url:          "https://docs.example.com",
			watchEnabled: false,
		}
		if cfg.GetURL() != "https://docs.example.com" {
			t.Errorf("unexpected URL: %s", cfg.GetURL())
		}
		if cfg.GetPath() != "" {
			t.Errorf("expected empty path for URL config, got %s", cfg.GetPath())
		}
		if cfg.IsWatchEnabled() {
			t.Error("expected watch disabled")
		}
	})
}
