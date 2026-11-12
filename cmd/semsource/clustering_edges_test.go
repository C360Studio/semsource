package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
)

// clusteringConfigFor composes the graph subsystem and returns the
// graph-clustering component's marshalled config, or ok=false when no such
// component was composed.
func clusteringConfigFor(t *testing.T, cfg *config.Config) (map[string]json.RawMessage, bool) {
	t.Helper()
	comps, err := graphSubsystemComponents(cfg)
	if err != nil {
		t.Fatalf("graphSubsystemComponents: %v", err)
	}
	c, ok := comps["graph-clustering"]
	if !ok {
		return nil, false
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(c.Config, &got); err != nil {
		t.Fatalf("unmarshal graph-clustering config: %v", err)
	}
	return got, true
}

// spec: openspec/changes/qualify-semengine-tier0/specs/tier0-consumer-qualification/spec.md#Requirement: Tier 0 qualifies an exclusive no-embedder composition
func TestClusteringConfig_NoEntityIDEdgesEmitted(t *testing.T) {
	cfg := &config.Config{
		Namespace: "acme",
		Graph:     &config.GraphConfig{EnableClustering: true},
	}

	got, ok := clusteringConfigFor(t, cfg)
	if !ok {
		t.Fatal("graph-clustering component not composed with EnableClustering: true")
	}
	if raw, present := got["entity_id_edges"]; present {
		t.Errorf("entity_id_edges must be absent, got %s", raw)
	}
}

// spec: openspec/changes/qualify-semengine-tier0/specs/tier0-consumer-qualification/spec.md#Requirement: Tier 0 qualifies an exclusive no-embedder composition
func TestNoSemStreamsImports(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	cmd := exec.Command(
		"go", "list",
		"-tags=integration,e2e,garage",
		"-f", "{{.ImportPath}}: {{join .Imports \",\"}} {{join .TestImports \",\"}} {{join .XTestImports \",\"}}",
		"./...",
	)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list ./...: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 20 {
		t.Fatalf("expected module-wide go list to inspect at least 20 packages, got %d", len(lines))
	}
	for _, line := range lines {
		if strings.Contains(line, "/ui/node_modules/") {
			continue
		}
		if strings.Contains(line, "github.com/c360studio/semstreams") {
			t.Errorf("forbidden semstreams import detected: %s", line)
		}
	}

	for _, modFile := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join(repoRoot, modFile))
		if err != nil {
			t.Fatalf("read %s: %v", modFile, err)
		}
		if strings.Contains(string(data), "github.com/c360studio/semstreams") {
			t.Errorf("%s still references github.com/c360studio/semstreams", modFile)
		}
	}
}
