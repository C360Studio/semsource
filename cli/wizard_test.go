package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
)

// buildInitInput constructs terminal input for the Init wizard.
// It simulates:
//   - namespace entry
//   - graph stream address (default)
//   - toggle menu: type "done" to accept defaults (all available selected)
//   - AST wizard prompts: path (default "."), language index 1 (auto-detect), watch yes
//   - Git wizard prompts: url, branch (default "main"), watch yes
//   - Doc wizard prompts: one path, watch yes
//   - Config wizard prompts: one path, watch yes
//   - URL wizard prompts: one URL, poll interval (default "5m")
func buildInitInput() string {
	lines := []string{
		"testorg",    // namespace
		"",           // graph stream address (default localhost:7890)
		"done",       // source menu: accept defaults (all available toggled on)
		// AST wizard
		"",           // path (default ".")
		"1",          // language: auto-detect
		"y",          // watch
		// Git wizard
		"https://github.com/example/repo", // url
		"",           // branch (default "main")
		"y",          // watch
		// Doc wizard
		"docs/",      // path
		"",           // end multi-line
		"y",          // watch
		// Config wizard
		"go.mod",     // path
		"",           // end multi-line
		"y",          // watch
		// URL wizard
		"https://example.com", // url
		"",           // end multi-line
		"",           // poll interval (default "5m")
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestInitWritesValidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")

	input := buildInitInput()
	term, _ := newTestTerm(input)

	if err := Init(term, cfgPath); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// File must exist.
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	// Must be valid JSON.
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v\n\nRaw:\n%s", err, string(data))
	}

	// Basic assertions.
	if cfg.Namespace != "testorg" {
		t.Errorf("expected namespace 'testorg', got %q", cfg.Namespace)
	}
	if len(cfg.Flow.Outputs) == 0 {
		t.Error("expected at least one flow output")
	}
	if len(cfg.Sources) == 0 {
		t.Error("expected at least one source")
	}

	// Verify at least ast and git sources are present.
	typesSeen := map[string]bool{}
	for _, s := range cfg.Sources {
		typesSeen[s.Type] = true
	}
	for _, want := range []string{"ast", "git", "docs", "config", "url"} {
		if !typesSeen[want] {
			t.Errorf("expected source type %q in config, got types: %v", want, typesSeen)
		}
	}
}

func TestInitAbortOnExistingConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")

	// Write a dummy existing config.
	_ = os.WriteFile(cfgPath, []byte(`{"namespace":"old"}`), 0644)

	// Say "n" to overwrite prompt.
	term, _ := newTestTerm("n\n")
	if err := Init(term, cfgPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should still contain the old content.
	data, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(data), "old") {
		t.Error("expected old config to be preserved")
	}
}

func TestInitRequiresNamespace(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")

	// Empty namespace → error.
	term, _ := newTestTerm("\n")
	err := Init(term, cfgPath)
	if err == nil {
		t.Fatal("expected error when namespace is empty")
	}
	if !strings.Contains(err.Error(), "namespace") {
		t.Errorf("expected namespace error, got: %v", err)
	}
}

func TestAddNonInteractiveAST(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeMinimalConfig(t, cfgPath, "myorg")

	term, _ := newTestTerm("")
	args := []string{"ast", "--path", "./src", "--language", "go", "--watch=true"}
	if err := Add(term, cfgPath, args); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	cfg := loadConfig(t, cfgPath)
	// Should have original source + new ast.
	found := false
	for _, s := range cfg.Sources {
		if s.Type == "ast" && s.Path == "./src" && s.Language == "go" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ast source with path ./src, got: %+v", cfg.Sources)
	}
}

func TestAddNonInteractiveGitMissingURL(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeMinimalConfig(t, cfgPath, "myorg")

	term, _ := newTestTerm("")
	err := Add(term, cfgPath, []string{"git"})
	if err == nil {
		t.Fatal("expected error for missing --url")
	}
}

func TestAddUnknownType(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeMinimalConfig(t, cfgPath, "myorg")

	term, _ := newTestTerm("")
	err := Add(term, cfgPath, []string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

// writeMinimalConfig writes a semsource.json with one ast source so LoadConfig passes.
func writeMinimalConfig(t *testing.T, path, namespace string) {
	t.Helper()
	cfg := config.Config{
		Namespace: namespace,
		Flow: config.FlowConfig{
			Outputs: []config.OutputConfig{
				{Name: "out", Type: "network", Subject: "http://localhost:7890/graph"},
			},
			DeliveryMode: "at-least-once",
			AckTimeout:   "5s",
		},
		Sources: []config.SourceEntry{
			{Type: "ast", Path: "."},
		},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func loadConfig(t *testing.T, path string) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}
