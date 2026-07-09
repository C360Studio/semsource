package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
)

func TestRemoveByIndexMutatesConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeSourcesConfig(t, cfgPath, []config.SourceEntry{
		{Type: "ast", Path: "./src", Language: "go", Watch: true},
		{Type: "repo", URL: "github.com/org/repo", Branch: "main", Watch: true},
		{Type: "docs", Paths: []string{"docs/", "README.md"}, Watch: true},
	})

	term, out := newTestTerm("")
	if err := Remove(term, cfgPath, 1); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	cfg := loadConfig(t, cfgPath)
	if got := sourceTypes(cfg.Sources); !equalStrings(got, []string{"ast", "docs"}) {
		t.Fatalf("source types after remove = %v, want [ast docs]", got)
	}
	if !strings.Contains(out.String(), "Removed repo source") {
		t.Fatalf("remove output missing success message:\n%s", out.String())
	}
}

func TestRemoveInteractiveMutatesConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeSourcesConfig(t, cfgPath, []config.SourceEntry{
		{Type: "ast", Path: "./src", Language: "go", Watch: true},
		{Type: "docs", Paths: []string{"docs/", "README.md"}, Watch: true},
		{Type: "url", URLs: []string{"https://api.example.com/docs"}, PollInterval: "10m"},
	})

	term, out := newTestTerm("2\n")
	if err := Remove(term, cfgPath, -1); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	cfg := loadConfig(t, cfgPath)
	if got := sourceTypes(cfg.Sources); !equalStrings(got, []string{"ast", "url"}) {
		t.Fatalf("source types after remove = %v, want [ast url]", got)
	}
	for _, want := range []string{"Remove a source", "Which source to remove?", "Removed docs source"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("remove output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRemoveInvalidIndexDoesNotMutateConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "semsource.json")
	writeSourcesConfig(t, cfgPath, []config.SourceEntry{
		{Type: "ast", Path: "./src", Language: "go", Watch: true},
		{Type: "docs", Paths: []string{"docs/", "README.md"}, Watch: true},
	})

	term, _ := newTestTerm("")
	err := Remove(term, cfgPath, 9)
	if err == nil {
		t.Fatal("expected invalid index error")
	}
	if !strings.Contains(err.Error(), "invalid source index 10") {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := loadConfig(t, cfgPath)
	if got := sourceTypes(cfg.Sources); !equalStrings(got, []string{"ast", "docs"}) {
		t.Fatalf("source types after failed remove = %v, want [ast docs]", got)
	}
}

func sourceTypes(sources []config.SourceEntry) []string {
	types := make([]string, 0, len(sources))
	for _, source := range sources {
		types = append(types, source.Type)
	}
	return types
}
