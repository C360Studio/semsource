package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/config"
)

const validJSON = `{
  "namespace": "acme",
  "sources": [
    {
      "type": "git",
      "url": "github.com/acme/gcs",
      "branch": "main",
      "watch": true
    },
    {
      "type": "ast",
      "path": "./",
      "language": "go",
      "watch": true
    },
    {
      "type": "docs",
      "paths": ["README.md", "docs/"],
      "watch": true
    },
    {
      "type": "config",
      "paths": ["go.mod", "Dockerfile"],
      "watch": true
    },
    {
      "type": "url",
      "urls": ["https://docs.acme.io/gcs"],
      "poll_interval": "300s"
    }
  ]
}`

func TestLoadConfigFromReader_ValidJSON(t *testing.T) {
	cfg, err := config.LoadConfigFromReader(strings.NewReader(validJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Namespace != "acme" {
		t.Errorf("namespace: got %q, want %q", cfg.Namespace, "acme")
	}

	// Sources assertions
	if len(cfg.Sources) != 5 {
		t.Fatalf("sources count: got %d, want 5", len(cfg.Sources))
	}

	git := cfg.Sources[0]
	if git.Type != "git" {
		t.Errorf("source[0] type: got %q, want %q", git.Type, "git")
	}
	if git.URL != "github.com/acme/gcs" {
		t.Errorf("source[0] url: got %q, want %q", git.URL, "github.com/acme/gcs")
	}
	if git.Branch != "main" {
		t.Errorf("source[0] branch: got %q, want %q", git.Branch, "main")
	}
	if !git.Watch {
		t.Error("source[0] watch: got false, want true")
	}

	ast := cfg.Sources[1]
	if ast.Type != "ast" {
		t.Errorf("source[1] type: got %q, want %q", ast.Type, "ast")
	}
	if ast.Path != "./" {
		t.Errorf("source[1] path: got %q, want %q", ast.Path, "./")
	}
	if ast.Language != "go" {
		t.Errorf("source[1] language: got %q, want %q", ast.Language, "go")
	}

	docs := cfg.Sources[2]
	if docs.Type != "docs" {
		t.Errorf("source[2] type: got %q, want %q", docs.Type, "docs")
	}
	if len(docs.Paths) != 2 {
		t.Errorf("source[2] paths count: got %d, want 2", len(docs.Paths))
	}

	cfgSrc := cfg.Sources[3]
	if cfgSrc.Type != "config" {
		t.Errorf("source[3] type: got %q, want %q", cfgSrc.Type, "config")
	}
	if len(cfgSrc.Paths) != 2 {
		t.Errorf("source[3] paths count: got %d, want 2", len(cfgSrc.Paths))
	}

	url := cfg.Sources[4]
	if url.Type != "url" {
		t.Errorf("source[4] type: got %q, want %q", url.Type, "url")
	}
	if len(url.URLs) != 1 || url.URLs[0] != "https://docs.acme.io/gcs" {
		t.Errorf("source[4] urls: got %v, want [https://docs.acme.io/gcs]", url.URLs)
	}
	if url.PollInterval != "300s" {
		t.Errorf("source[4] poll_interval: got %q, want %q", url.PollInterval, "300s")
	}
}

func TestLoadConfigFromReader_WorkspaceDirDefault(t *testing.T) {
	// When workspace_dir is absent, applyDefaults should set it to
	// ~/.semsource/repos derived from os.UserHomeDir().
	input := `{
  "namespace": "myorg",
  "sources": [
    {"type": "git", "url": "github.com/myorg/repo"}
  ]
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}
	want := filepath.Join(home, ".semsource", "repos")
	if cfg.WorkspaceDir != want {
		t.Errorf("WorkspaceDir default: got %q, want %q", cfg.WorkspaceDir, want)
	}
}

func TestLoadConfigFromReader_WorkspaceDirExplicit(t *testing.T) {
	// When workspace_dir is provided, it must not be overridden by the default.
	input := `{
  "namespace": "myorg",
  "sources": [
    {"type": "git", "url": "github.com/myorg/repo"}
  ],
  "workspace_dir": "/custom/workspace"
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.WorkspaceDir != "/custom/workspace" {
		t.Errorf("WorkspaceDir: got %q, want %q", cfg.WorkspaceDir, "/custom/workspace")
	}
}

func TestLoadConfigFromReader_WebSocketDefaults(t *testing.T) {
	input := `{
  "namespace": "myorg",
  "sources": [
    {"type": "git", "url": "github.com/myorg/repo"}
  ]
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WebSocketBind != "0.0.0.0:7890" {
		t.Errorf("WebSocketBind default: got %q, want %q", cfg.WebSocketBind, "0.0.0.0:7890")
	}
	if cfg.WebSocketPath != "/graph" {
		t.Errorf("WebSocketPath default: got %q, want %q", cfg.WebSocketPath, "/graph")
	}
}

func TestLoadConfigFromReader_WebSocketExplicit(t *testing.T) {
	input := `{
  "namespace": "myorg",
  "sources": [
    {"type": "git", "url": "github.com/myorg/repo"}
  ],
  "websocket_bind": "0.0.0.0:9999",
  "websocket_path": "/ws"
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WebSocketBind != "0.0.0.0:9999" {
		t.Errorf("WebSocketBind: got %q, want %q", cfg.WebSocketBind, "0.0.0.0:9999")
	}
	if cfg.WebSocketPath != "/ws" {
		t.Errorf("WebSocketPath: got %q, want %q", cfg.WebSocketPath, "/ws")
	}
}

func TestLoadConfigFromReader_WebSocketEnvOverride(t *testing.T) {
	t.Setenv("SEMSOURCE_WS_BIND", "0.0.0.0:8888")
	t.Setenv("SEMSOURCE_WS_PATH", "/stream")

	input := `{
  "namespace": "myorg",
  "sources": [
    {"type": "git", "url": "github.com/myorg/repo"}
  ],
  "websocket_bind": "0.0.0.0:9999",
  "websocket_path": "/ws"
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WebSocketBind != "0.0.0.0:8888" {
		t.Errorf("WebSocketBind env override: got %q, want %q", cfg.WebSocketBind, "0.0.0.0:8888")
	}
	if cfg.WebSocketPath != "/stream" {
		t.Errorf("WebSocketPath env override: got %q, want %q", cfg.WebSocketPath, "/stream")
	}
}

func TestLoadConfigFromReader_MissingNamespace(t *testing.T) {
	input := `{
  "sources": [
    {"type": "git", "url": "github.com/myorg/repo", "branch": "main"}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for missing namespace, got nil")
	}
}

func TestLoadConfigFromReader_NoSources(t *testing.T) {
	input := `{
  "namespace": "myorg",
  "sources": []
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for empty sources, got nil")
	}
}

func TestLoadConfigFromReader_InvalidSourceType(t *testing.T) {
	input := `{
  "namespace": "myorg",
  "sources": [
    {"type": "database", "url": "postgres://localhost/mydb"}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for invalid source type, got nil")
	}
}

func TestLoadConfigFromReader_GitMissingURL(t *testing.T) {
	input := `{
  "namespace": "myorg",
  "sources": [
    {"type": "git", "branch": "main"}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for git source missing url, got nil")
	}
}

func TestLoadConfigFromReader_ASTMissingPath(t *testing.T) {
	input := `{
  "namespace": "myorg",
  "sources": [
    {"type": "ast", "language": "go"}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for ast source missing path, got nil")
	}
}

func TestLoadConfigFromReader_URLMissingURLs(t *testing.T) {
	input := `{
  "namespace": "myorg",
  "sources": [
    {"type": "url", "poll_interval": "60s"}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for url source missing urls, got nil")
	}
}

func TestLoadConfigFromReader_InvalidJSON(t *testing.T) {
	_, err := config.LoadConfigFromReader(strings.NewReader("{invalid json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := config.LoadConfig("/nonexistent/path/semsource.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestPollIntervalDuration verifies url source poll_interval can be parsed as a Go duration.
func TestPollIntervalDuration(t *testing.T) {
	cfg, err := config.LoadConfigFromReader(strings.NewReader(validJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	url := cfg.Sources[4]
	d, err := time.ParseDuration(url.PollInterval)
	if err != nil {
		t.Fatalf("PollInterval %q is not a valid Go duration: %v", url.PollInterval, err)
	}
	if d != 300*time.Second {
		t.Errorf("PollInterval duration: got %v, want 300s", d)
	}
}

func TestLoadConfigFromReader_ImageSource(t *testing.T) {
	trueVal := true
	_ = trueVal // used only to confirm the bool pointer round-trips correctly

	input := `{
  "namespace": "acme",
  "sources": [
    {
      "type": "image",
      "paths": ["assets/images/"],
      "extensions": ["png", "jpg"],
      "max_file_size": "50MB",
      "generate_thumbnails": true,
      "thumbnail_max_dim": 512,
      "watch": true
    }
  ]
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	src := cfg.Sources[0]
	if src.Type != "image" {
		t.Errorf("type: got %q, want %q", src.Type, "image")
	}
	if len(src.Paths) != 1 || src.Paths[0] != "assets/images/" {
		t.Errorf("paths: got %v, want [assets/images/]", src.Paths)
	}
	if len(src.Extensions) != 2 || src.Extensions[0] != "png" || src.Extensions[1] != "jpg" {
		t.Errorf("extensions: got %v, want [png jpg]", src.Extensions)
	}
	if src.MaxFileSize != "50MB" {
		t.Errorf("max_file_size: got %q, want %q", src.MaxFileSize, "50MB")
	}
	if src.GenerateThumbnails == nil || !*src.GenerateThumbnails {
		t.Errorf("generate_thumbnails: got %v, want true", src.GenerateThumbnails)
	}
	if src.ThumbnailMaxDim != 512 {
		t.Errorf("thumbnail_max_dim: got %d, want 512", src.ThumbnailMaxDim)
	}
	if !src.Watch {
		t.Error("watch: got false, want true")
	}
}

func TestLoadConfigFromReader_ImageMissingPaths(t *testing.T) {
	input := `{
  "namespace": "acme",
  "sources": [
    {"type": "image"}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for image source missing paths, got nil")
	}
}

func TestLoadConfigFromReader_ImageWithoutObjectStore(t *testing.T) {
	// Image sources work in metadata-only mode without an ObjectStore.
	input := `{
  "namespace": "acme",
  "sources": [
    {"type": "image", "paths": ["assets/"]}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("image sources should work without object_store (metadata-only mode): %v", err)
	}
}

func TestLoadConfigFromReader_ImageEmptyExtension(t *testing.T) {
	input := `{
  "namespace": "acme",
  "sources": [
    {"type": "image", "paths": ["assets/"], "extensions": ["png", ""]}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for image source with empty extension string, got nil")
	}
}

func TestLoadConfigFromReader_VideoSource(t *testing.T) {
	input := `{
  "namespace": "acme",
  "sources": [
    {
      "type": "video",
      "paths": ["recordings/", "training/"],
      "keyframe_mode": "interval",
      "keyframe_interval": "30s",
      "max_file_size": "2GB",
      "watch": true
    }
  ]
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	src := cfg.Sources[0]
	if src.Type != "video" {
		t.Errorf("type: got %q, want %q", src.Type, "video")
	}
	if len(src.Paths) != 2 || src.Paths[0] != "recordings/" || src.Paths[1] != "training/" {
		t.Errorf("paths: got %v, want [recordings/ training/]", src.Paths)
	}
	if src.KeyframeMode != "interval" {
		t.Errorf("keyframe_mode: got %q, want %q", src.KeyframeMode, "interval")
	}
	if src.KeyframeInterval != "30s" {
		t.Errorf("keyframe_interval: got %q, want %q", src.KeyframeInterval, "30s")
	}
	if src.MaxFileSize != "2GB" {
		t.Errorf("max_file_size: got %q, want %q", src.MaxFileSize, "2GB")
	}
	if !src.Watch {
		t.Error("watch: got false, want true")
	}

	// Verify keyframe_interval is a valid Go duration.
	d, err := time.ParseDuration(src.KeyframeInterval)
	if err != nil {
		t.Fatalf("keyframe_interval %q is not a valid Go duration: %v", src.KeyframeInterval, err)
	}
	if d != 30*time.Second {
		t.Errorf("keyframe_interval duration: got %v, want 30s", d)
	}
}

func TestLoadConfigFromReader_VideoMissingPaths(t *testing.T) {
	input := `{
  "namespace": "acme",
  "sources": [
    {"type": "video"}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for video source missing paths, got nil")
	}
}

func TestLoadConfigFromReader_VideoWithoutMediaStoreDir_MetadataOnlyIsValid(t *testing.T) {
	// Video sources no longer require an objectstore or media_store_dir.
	// Omitting both causes the handler to run in metadata-only mode, which is
	// explicitly supported after the migration from objectstore to filestore.
	input := `{
  "namespace": "acme",
  "sources": [
    {"type": "video", "paths": ["recordings/"]}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("expected valid config for video source in metadata-only mode, got: %v", err)
	}
}

func TestLoadConfigFromReader_VideoInvalidKeyframeMode(t *testing.T) {
	input := `{
  "namespace": "acme",
  "sources": [
    {"type": "video", "paths": ["recordings/"], "keyframe_mode": "magic"}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for video source with invalid keyframe_mode, got nil")
	}
}

func TestLoadConfigFromReader_ModeDefault(t *testing.T) {
	input := `{
  "namespace": "acme",
  "sources": [{"type": "ast", "path": "./", "language": "go"}]
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Mode != config.ModeStandalone {
		t.Errorf("mode: got %q, want %q", cfg.Mode, config.ModeStandalone)
	}
	if cfg.IsHeadless() {
		t.Error("IsHeadless() = true, want false for default mode")
	}
}

func TestLoadConfigFromReader_ModeHeadless(t *testing.T) {
	input := `{
  "namespace": "acme",
  "mode": "headless",
  "sources": [{"type": "ast", "path": "./", "language": "go"}]
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Mode != config.ModeHeadless {
		t.Errorf("mode: got %q, want %q", cfg.Mode, config.ModeHeadless)
	}
	if !cfg.IsHeadless() {
		t.Error("IsHeadless() = false, want true")
	}
}

func TestLoadConfigFromReader_ModeStandalone(t *testing.T) {
	input := `{
  "namespace": "acme",
  "mode": "standalone",
  "sources": [{"type": "ast", "path": "./", "language": "go"}]
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Mode != config.ModeStandalone {
		t.Errorf("mode: got %q, want %q", cfg.Mode, config.ModeStandalone)
	}
}

func TestLoadConfigFromReader_ModeInvalid(t *testing.T) {
	input := `{
  "namespace": "acme",
  "mode": "turbo",
  "sources": [{"type": "ast", "path": "./", "language": "go"}]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for invalid mode")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Errorf("error should mention mode, got: %v", err)
	}
}

func TestLoadConfigFromReader_ModeEnvOverride(t *testing.T) {
	t.Setenv("SEMSOURCE_MODE", "headless")
	input := `{
  "namespace": "acme",
  "mode": "standalone",
  "sources": [{"type": "ast", "path": "./", "language": "go"}]
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Mode != config.ModeHeadless {
		t.Errorf("mode: got %q, want %q (env should override config)", cfg.Mode, config.ModeHeadless)
	}
}

func TestLoadConfigFromReader_GraphConfig(t *testing.T) {
	input := `{
  "namespace": "acme",
  "sources": [{"type": "ast", "path": "./", "language": "go"}],
  "graph": {
    "gateway_bind": "127.0.0.1:9000",
    "enable_playground": false,
    "embedder_type": "http",
    "embedding_batch_size": 100,
    "coalesce_ms": 500
  }
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Graph == nil {
		t.Fatal("graph config is nil")
	}
	if cfg.Graph.GatewayBind != "127.0.0.1:9000" {
		t.Errorf("gateway_bind: got %q, want %q", cfg.Graph.GatewayBind, "127.0.0.1:9000")
	}
	if cfg.Graph.EnablePlayground == nil || *cfg.Graph.EnablePlayground {
		t.Error("enable_playground: want false")
	}
	if cfg.Graph.EmbedderType != "http" {
		t.Errorf("embedder_type: got %q, want %q", cfg.Graph.EmbedderType, "http")
	}
	if cfg.Graph.EmbeddingBatchSize != 100 {
		t.Errorf("embedding_batch_size: got %d, want 100", cfg.Graph.EmbeddingBatchSize)
	}
	if cfg.Graph.CoalesceMs != 500 {
		t.Errorf("coalesce_ms: got %d, want 500", cfg.Graph.CoalesceMs)
	}
}

func TestLoadConfigFromReader_MetricsConfig(t *testing.T) {
	input := `{
  "namespace": "acme",
  "sources": [{"type": "ast", "path": "./", "language": "go"}],
  "metrics": {
    "port": 9999,
    "path": "/prom"
  }
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Metrics == nil {
		t.Fatal("metrics config is nil")
	}
	if cfg.Metrics.Port != 9999 {
		t.Errorf("port: got %d, want 9999", cfg.Metrics.Port)
	}
	if cfg.Metrics.Path != "/prom" {
		t.Errorf("path: got %q, want %q", cfg.Metrics.Path, "/prom")
	}
}

func TestLoadConfigFromReader_StreamOverrides(t *testing.T) {
	input := `{
  "namespace": "acme",
  "sources": [{"type": "ast", "path": "./", "language": "go"}],
  "streams": {
    "GRAPH": {
      "storage": "file",
      "max_age": "24h",
      "max_bytes": 1073741824,
      "replicas": 3
    }
  }
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Streams == nil {
		t.Fatal("streams config is nil")
	}
	graph, ok := cfg.Streams["GRAPH"]
	if !ok {
		t.Fatal("GRAPH stream override missing")
	}
	if graph.Storage != "file" {
		t.Errorf("storage: got %q, want %q", graph.Storage, "file")
	}
	if graph.MaxAge != "24h" {
		t.Errorf("max_age: got %q, want %q", graph.MaxAge, "24h")
	}
	if graph.MaxBytes == nil || *graph.MaxBytes != 1073741824 {
		t.Errorf("max_bytes: got %v, want 1073741824", graph.MaxBytes)
	}
	if graph.Replicas == nil || *graph.Replicas != 3 {
		t.Errorf("replicas: got %v, want 3", graph.Replicas)
	}
}
