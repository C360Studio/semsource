package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/config"
)

// addFixture writes a one-source config in a fresh working directory and
// returns its contents, so a test can tell whether a command rewrote it.
func addFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join(dir, "ws"), 0o755); err != nil {
		t.Fatal(err)
	}
	const original = `{"namespace":"acme","sources":[{"type":"docs","paths":["."]}]}`
	if err := os.WriteFile("semsource.json", []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	return original
}

// TestAddCmd_TypeBeforeFlags: the documented form puts the source type before
// its flags. Every flag was dropped and a default source appended, exit 0,
// because the add subcommand parsed the type's flags with a set that only
// knew --config (#267). Tests of cli.Add alone never crossed that boundary.
//
// spec: openspec/specs/advertised-surface-coverage/spec.md#Requirement: README CLI examples are command-tested
func TestAddCmd_TypeBeforeFlags(t *testing.T) {
	for _, args := range [][]string{
		{"ast", "--path", "./ws", "--language", "go"},
		{"--config", "semsource.json", "ast", "--path", "./ws", "--language", "go"},
	} {
		addFixture(t)
		if err := addCmd(args); err != nil {
			t.Fatalf("add %v: %v", args, err)
		}
		cfg, err := config.LoadConfig("semsource.json")
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.Sources) != 2 {
			t.Fatalf("add %v: %d sources, want 2", args, len(cfg.Sources))
		}
		got := cfg.Sources[1]
		if got.Type != "ast" || got.Path != "./ws" || got.Language != "go" {
			t.Errorf("add %v wrote %+v, want type ast, path ./ws, language go", args, got)
		}
	}
}

// TestAddCmd_RejectsWhatItCannotApply: an unknown flag, a stray argument, or
// a help request fails the command and leaves the file as it was.
//
// spec: openspec/specs/advertised-surface-coverage/spec.md#Requirement: README CLI examples are command-tested
func TestAddCmd_RejectsWhatItCannotApply(t *testing.T) {
	for _, args := range [][]string{
		{"ast", "--path", "./ws", "--lang", "go"},
		{"ast", "./ws"},
		{"ast", "-h"},
		{"--nope", "ast"},
	} {
		original := addFixture(t)
		if err := addCmd(args); err == nil {
			t.Errorf("add %v succeeded", args)
		}
		data, err := os.ReadFile("semsource.json")
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != original {
			t.Errorf("add %v rewrote the config:\n%s", args, data)
		}
	}
}

// TestAddCmd_READMEForms runs the README's other non-interactive examples
// through the command boundary and checks each flag reached the entry.
//
// spec: openspec/specs/advertised-surface-coverage/spec.md#Requirement: README CLI examples are command-tested
func TestAddCmd_READMEForms(t *testing.T) {
	tests := []struct {
		args  []string
		check func(config.SourceEntry) bool
	}{
		{[]string{"repo", "--url", "github.com/org/repo", "--branch", "main"},
			func(e config.SourceEntry) bool {
				return e.Type == "repo" && e.URL == "github.com/org/repo" && e.Branch == "main"
			}},
		{[]string{"docs", "--paths", "docs/,README.md"},
			func(e config.SourceEntry) bool {
				return e.Type == "docs" && len(e.Paths) == 2 && e.Paths[1] == "README.md"
			}},
		{[]string{"url", "--urls", "https://api.example.com/docs", "--poll-interval", "10m"},
			func(e config.SourceEntry) bool {
				return e.Type == "url" && len(e.URLs) == 1 && e.URLs[0] == "https://api.example.com/docs" && e.PollInterval == "10m"
			}},
	}
	for _, tc := range tests {
		addFixture(t)
		if err := addCmd(tc.args); err != nil {
			t.Fatalf("add %v: %v", tc.args, err)
		}
		cfg, err := config.LoadConfig("semsource.json")
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.Sources[len(cfg.Sources)-1]; !tc.check(got) {
			t.Errorf("add %v wrote %+v", tc.args, got)
		}
	}
}
