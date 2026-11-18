package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
)

// TestGeneratedConfigNeverCarriesGitToken: component configurations are
// replicated through the config bucket, so the token must not be in any of
// them, whether generated from the file at boot or built for a source added
// at run time (#272).
//
// spec: openspec/specs/runtime-configuration/spec.md#Requirement: The git token is resolved from the environment, never persisted into configuration
func TestGeneratedConfigNeverCarriesGitToken(t *testing.T) {
	const probe = "probe-token-272"
	t.Setenv(config.EnvGitToken, probe)

	cfg := fingerprintFixture()
	cfg.Sources = append(cfg.Sources, config.SourceEntry{Type: "repo", URL: "https://github.com/acme/private", Branch: "main"})
	ssCfg, err := buildSemstreamsConfig(cfg, "acme")
	if err != nil {
		t.Fatalf("buildSemstreamsConfig: %v", err)
	}
	assertNoToken(t, "file-generated composition", ssCfg, probe)

	spawned, err := sourcespawn.Build(config.SourceEntry{Type: "git", URL: "https://github.com/acme/private", Branch: "main"},
		sourcespawn.Options{Org: "acme", WorkspaceDir: t.TempDir()})
	if err != nil {
		t.Fatalf("sourcespawn.Build: %v", err)
	}
	if len(spawned) == 0 {
		t.Fatal("sourcespawn.Build returned no components")
	}
	assertNoToken(t, "runtime-spawned components", spawned, probe)
}

func assertNoToken(t *testing.T, what string, v any, probe string) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %s: %v", what, err)
	}
	if strings.Contains(string(data), probe) || strings.Contains(string(data), "git_token") {
		t.Fatalf("%s carries the git token: %s", what, data)
	}
}
