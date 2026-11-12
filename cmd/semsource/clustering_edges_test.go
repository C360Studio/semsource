package main

import (
	"encoding/json"
	"os/exec"
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
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}: {{join .Imports \",\"}} {{join .TestImports \",\"}} {{join .XTestImports \",\"}}", "./...")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list ./...: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "/ui/node_modules/") {
			continue
		}
		if strings.Contains(line, "github.com/c360studio/semstreams") {
			t.Errorf("forbidden semstreams import detected: %s", line)
		}
	}
}
