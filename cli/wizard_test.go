package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
)

// wantedTestSources maps source type keys to the wizard prompt answers
// needed to complete that wizard's interactive flow.
var wantedTestSources = map[string][]string{
	"ast": {
		"",  // path (default ".")
		"1", // language: auto-detect
		"y", // watch
	},
	"config": {
		"go.mod", // path
		"",       // end multi-line
		"y",      // watch
	},
	"docs": {
		"docs/", // path
		"",      // end multi-line
		"y",     // watch
	},
	"git": {
		"https://github.com/example/repo", // url
		"",                                // branch (default "main")
		"y",                               // watch
	},
	"repo": {
		"https://github.com/example/repo", // url
		"",                                // branch (default: remote default)
		"",                                // language (default: auto-detect)
		"y",                               // watch
	},
	"image": {
		"assets/", // path
		"",        // end multi-line
		"",        // max file size (default "50MB")
		"y",       // generate thumbnails
	},
	"url": {
		"https://example.com", // url
		"",                    // end multi-line
		"",                    // poll interval (default "5m")
	},
}

// buildInitInput constructs terminal input for the Init wizard.
// It dynamically determines menu positions from the registered wizard list
// so the test is stable regardless of which optional wizards are available
// (e.g., video/audio become available when ffmpeg is installed).
func buildInitInput() string {
	lines := []string{
		"testorg", // namespace
	}

	// Determine which menu positions to toggle — only the source types we
	// have prompt answers for. Menu is 1-indexed.
	wizards := Wizards()
	for i, w := range wizards {
		if _, ok := wantedTestSources[w.TypeKey()]; ok {
			lines = append(lines, fmt.Sprintf("%d", i+1))
		}
	}
	lines = append(lines, "done")

	// Append wizard prompts in registration order (same order the Init
	// wizard iterates selected sources).
	for _, w := range wizards {
		if answers, ok := wantedTestSources[w.TypeKey()]; ok {
			lines = append(lines, answers...)
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

func TestInitWritesValidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")

	input := buildInitInput()
	term, _ := newTestTerm(input)

	// Pass empty ProjectInfo so nothing is pre-detected (test controls all input).
	noDetection := &ProjectInfo{}
	if err := Init(term, cfgPath, noDetection); err != nil {
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
	if len(cfg.Sources) == 0 {
		t.Error("expected at least one source")
	}

	// Verify all toggled source types are present.
	typesSeen := map[string]bool{}
	for _, s := range cfg.Sources {
		typesSeen[s.Type] = true
	}
	for _, want := range []string{"ast", "git", "docs", "config", "url", "image"} {
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

	// Pass empty ProjectInfo (no namespace default) and empty input.
	noDetection := &ProjectInfo{}
	term, _ := newTestTerm("\n")
	err := Init(term, cfgPath, noDetection)
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

func TestInitQuickWithDetectedProject(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")

	// Simulate a Go project with git and docs.
	info := &ProjectInfo{
		HasGit:      true,
		GitRemote:   "https://github.com/acme/myapp.git",
		Language:    "go",
		HasDocs:     true,
		DocPaths:    []string{"docs/", "README.md"},
		ConfigFiles: []string{"go.mod"},
		DirName:     "myapp",
		Namespace:   "acme",
	}

	// InitQuick should not need any input.
	term, _ := newTestTerm("")
	if err := InitQuick(term, cfgPath, info); err != nil {
		t.Fatalf("InitQuick failed: %v", err)
	}

	cfg := loadConfig(t, cfgPath)

	if cfg.Namespace != "acme" {
		t.Errorf("expected namespace 'acme', got %q", cfg.Namespace)
	}
	if len(cfg.Sources) < 3 {
		t.Errorf("expected at least 3 sources (git, ast, docs), got %d", len(cfg.Sources))
	}

	typesSeen := map[string]bool{}
	for _, s := range cfg.Sources {
		typesSeen[s.Type] = true
	}
	for _, want := range []string{"git", "ast", "docs", "config"} {
		if !typesSeen[want] {
			t.Errorf("expected source type %q, got types: %v", want, typesSeen)
		}
	}
}

func TestInitQuickFallsBackWhenNothingDetected(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")

	// Empty detection — should fall back to interactive (and fail on EOF).
	info := &ProjectInfo{}
	term, _ := newTestTerm("")
	err := InitQuick(term, cfgPath, info)
	// Expect failure since interactive mode gets no input.
	if err == nil {
		t.Fatal("expected error when falling back to interactive with no input")
	}
}

func TestShouldPreselect(t *testing.T) {
	info := &ProjectInfo{
		HasGit:      true,
		Language:    "go",
		HasDocs:     false,
		ConfigFiles: []string{"go.mod"},
		HasImages:   true,
		ImagePaths:  []string{"assets/"},
	}

	if !shouldPreselect("git", info) {
		t.Error("expected git to be preselected")
	}
	if !shouldPreselect("ast", info) {
		t.Error("expected ast to be preselected")
	}
	if shouldPreselect("docs", info) {
		t.Error("expected docs to NOT be preselected")
	}
	if !shouldPreselect("config", info) {
		t.Error("expected config to be preselected")
	}
	if shouldPreselect("url", info) {
		t.Error("expected url to NOT be preselected")
	}
	if !shouldPreselect("image", info) {
		t.Error("expected image to be preselected when HasImages is true")
	}
	// Image should NOT be preselected when HasImages is false.
	infoNoImages := &ProjectInfo{}
	if shouldPreselect("image", infoNoImages) {
		t.Error("expected image to NOT be preselected when HasImages is false")
	}
}

// writeMinimalConfig writes a semsource.json with one ast source so LoadConfig passes.
func writeMinimalConfig(t *testing.T, path, namespace string) {
	t.Helper()
	cfg := config.Config{
		Namespace: namespace,
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
