package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/config"
)

const validJSON = `{
  "namespace": "acme",
  "flow": {
    "outputs": [
      {
        "name": "graph_stream",
        "type": "network",
        "subject": "http://0.0.0.0:7890/graph"
      }
    ],
    "delivery_mode": "at-least-once",
    "ack_timeout": "5s"
  },
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

	// Flow assertions
	if cfg.Flow.DeliveryMode != "at-least-once" {
		t.Errorf("delivery_mode: got %q, want %q", cfg.Flow.DeliveryMode, "at-least-once")
	}
	if cfg.Flow.AckTimeout != "5s" {
		t.Errorf("ack_timeout: got %q, want %q", cfg.Flow.AckTimeout, "5s")
	}
	if len(cfg.Flow.Outputs) != 1 {
		t.Fatalf("outputs count: got %d, want 1", len(cfg.Flow.Outputs))
	}
	out := cfg.Flow.Outputs[0]
	if out.Name != "graph_stream" {
		t.Errorf("output name: got %q, want %q", out.Name, "graph_stream")
	}
	if out.Type != "network" {
		t.Errorf("output type: got %q, want %q", out.Type, "network")
	}
	if out.Subject != "http://0.0.0.0:7890/graph" {
		t.Errorf("output subject: got %q, want %q", out.Subject, "http://0.0.0.0:7890/graph")
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

func TestLoadConfigFromReader_Defaults(t *testing.T) {
	input := `{
  "namespace": "myorg",
  "flow": {
    "outputs": [
      {"name": "out", "type": "network", "subject": "http://localhost:7890/graph"}
    ]
  },
  "sources": [
    {"type": "git", "url": "github.com/myorg/repo", "branch": "main"}
  ]
}`
	cfg, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Flow.DeliveryMode != "at-least-once" {
		t.Errorf("default delivery_mode: got %q, want %q", cfg.Flow.DeliveryMode, "at-least-once")
	}
	if cfg.Flow.AckTimeout != "5s" {
		t.Errorf("default ack_timeout: got %q, want %q", cfg.Flow.AckTimeout, "5s")
	}
}

func TestLoadConfigFromReader_MissingNamespace(t *testing.T) {
	input := `{
  "flow": {
    "outputs": [
      {"name": "out", "type": "network", "subject": "http://localhost:7890/graph"}
    ]
  },
  "sources": [
    {"type": "git", "url": "github.com/myorg/repo", "branch": "main"}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for missing namespace, got nil")
	}
}

func TestLoadConfigFromReader_NoOutputs(t *testing.T) {
	input := `{
  "namespace": "myorg",
  "flow": {"outputs": []},
  "sources": [
    {"type": "git", "url": "github.com/myorg/repo", "branch": "main"}
  ]
}`
	_, err := config.LoadConfigFromReader(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected validation error for empty outputs, got nil")
	}
}

func TestLoadConfigFromReader_NoSources(t *testing.T) {
	input := `{
  "namespace": "myorg",
  "flow": {
    "outputs": [
      {"name": "out", "type": "network", "subject": "http://localhost:7890/graph"}
    ]
  },
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
  "flow": {
    "outputs": [
      {"name": "out", "type": "network", "subject": "http://localhost:7890/graph"}
    ]
  },
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
  "flow": {
    "outputs": [
      {"name": "out", "type": "network", "subject": "http://localhost:7890/graph"}
    ]
  },
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
  "flow": {
    "outputs": [
      {"name": "out", "type": "network", "subject": "http://localhost:7890/graph"}
    ]
  },
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
  "flow": {
    "outputs": [
      {"name": "out", "type": "network", "subject": "http://localhost:7890/graph"}
    ]
  },
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

// TestAckTimeoutDuration verifies the AckTimeout can be parsed as a Go duration.
func TestAckTimeoutDuration(t *testing.T) {
	cfg, err := config.LoadConfigFromReader(strings.NewReader(validJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	d, err := time.ParseDuration(cfg.Flow.AckTimeout)
	if err != nil {
		t.Fatalf("AckTimeout %q is not a valid Go duration: %v", cfg.Flow.AckTimeout, err)
	}
	if d != 5*time.Second {
		t.Errorf("AckTimeout duration: got %v, want 5s", d)
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
