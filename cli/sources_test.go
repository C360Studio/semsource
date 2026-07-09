package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
)

func TestSourcesListsConfiguredEntries(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeSourcesConfig(t, cfgPath, []config.SourceEntry{
		{Type: "ast", Path: "./src", Language: "go", Watch: true},
		{Type: "repo", URL: "github.com/org/repo", Branch: "main", Watch: true},
		{Type: "docs", Paths: []string{"docs/", "README.md"}, Watch: true},
		{Type: "url", URLs: []string{"https://api.example.com/docs"}, PollInterval: "10m"},
	})

	term, out := newTestTerm("")
	if err := Sources(term, cfgPath); err != nil {
		t.Fatalf("Sources failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Sources in ",
		"(4)",
		"TYPE",
		"PATH / URL",
		"WATCH",
		"EXTRA",
		"ast",
		"./src",
		"lang=go",
		"repo",
		"github.com/org/repo",
		"branch=main",
		"docs",
		"docs/, README.md",
		"url",
		"https://api.example.com/docs",
		"poll=10m",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("sources output missing %q:\n%s", want, got)
		}
	}
}

func TestSourcesTruncatesLongLocations(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	longURL := "https://api.example.com/docs/reference/very/long/path"
	writeSourcesConfig(t, cfgPath, []config.SourceEntry{
		{Type: "url", URLs: []string{longURL}, PollInterval: "10m"},
	})

	term, out := newTestTerm("")
	if err := Sources(term, cfgPath); err != nil {
		t.Fatalf("Sources failed: %v", err)
	}

	got := out.String()
	if strings.Contains(got, longURL) {
		t.Fatalf("sources output should truncate long URL:\n%s", got)
	}
	if !strings.Contains(got, truncate(longURL, 30)) {
		t.Fatalf("sources output missing truncated URL:\n%s", got)
	}
}

func writeSourcesConfig(t *testing.T, path string, sources []config.SourceEntry) {
	t.Helper()
	cfg := config.Config{
		Namespace: "myorg",
		Sources:   sources,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
