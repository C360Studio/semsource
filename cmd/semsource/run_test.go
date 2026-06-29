package main

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semsource/config"
)

func TestGraphSubsystemComponents_ObserveOnlyOwnerLease(t *testing.T) {
	components, err := graphSubsystemComponents(&config.Config{})
	if err != nil {
		t.Fatalf("graphSubsystemComponents() error = %v", err)
	}

	graphIngest, ok := components["graph-ingest"]
	if !ok {
		t.Fatal("graph-ingest component not configured")
	}

	var raw map[string]any
	if err := json.Unmarshal(graphIngest.Config, &raw); err != nil {
		t.Fatalf("unmarshal graph-ingest config: %v", err)
	}

	got, ok := raw["enforce_owner_lease"].(bool)
	if !ok {
		t.Fatalf("enforce_owner_lease missing or not bool: %#v", raw["enforce_owner_lease"])
	}
	if got {
		t.Fatal("enforce_owner_lease should remain false for the first governed migration")
	}
}

func TestGraphSubsystemComponents_GraphQueryPortsCoverSemStreamsBeta114(t *testing.T) {
	components, err := graphSubsystemComponents(&config.Config{})
	if err != nil {
		t.Fatalf("graphSubsystemComponents() error = %v", err)
	}

	graphQuery, ok := components["graph-query"]
	if !ok {
		t.Fatal("graph-query component not configured")
	}

	var raw map[string]any
	if err := json.Unmarshal(graphQuery.Config, &raw); err != nil {
		t.Fatalf("unmarshal graph-query config: %v", err)
	}

	inputs := portDefinitions(t, raw, "inputs")
	want := map[string]string{
		"query_entity":          "graph.query.entity",
		"query_entity_by_alias": "graph.query.entityByAlias",
		"query_batch":           "graph.query.batch",
		"query_relationships":   "graph.query.relationships",
		"query_path_search":     "graph.query.pathSearch",
		"query_hierarchy_stats": "graph.query.hierarchyStats",
		"query_prefix":          "graph.query.prefix",
		"query_spatial":         "graph.query.spatial",
		"query_temporal":        "graph.query.temporal",
		"query_semantic":        "graph.query.semantic",
		"query_similar":         "graph.query.similar",
		"local_search":          "graph.query.localSearch",
		"global_search":         "graph.query.globalSearch",
		"query_summary":         "graph.query.summary",
		"query_search_graph":    "graph.query.searchGraph",
	}
	assertPortSubjects(t, inputs, want)
}

func TestGraphSubsystemComponents_GraphGatewayAdvertisesQueriesAndMutations(t *testing.T) {
	components, err := graphSubsystemComponents(&config.Config{})
	if err != nil {
		t.Fatalf("graphSubsystemComponents() error = %v", err)
	}

	graphGateway, ok := components["graph-gateway"]
	if !ok {
		t.Fatal("graph-gateway component not configured")
	}

	var raw map[string]any
	if err := json.Unmarshal(graphGateway.Config, &raw); err != nil {
		t.Fatalf("unmarshal graph-gateway config: %v", err)
	}

	outputs := portDefinitions(t, raw, "outputs")
	want := map[string]string{
		"queries":   "graph.query.*",
		"mutations": "graph.mutation.*",
	}
	assertPortSubjects(t, outputs, want)
}

func portDefinitions(t *testing.T, raw map[string]any, direction string) map[string]string {
	t.Helper()

	ports, ok := raw["ports"].(map[string]any)
	if !ok {
		t.Fatalf("ports missing or malformed: %#v", raw["ports"])
	}

	defs, ok := ports[direction].([]any)
	if !ok {
		t.Fatalf("%s ports missing or malformed: %#v", direction, ports[direction])
	}

	result := make(map[string]string, len(defs))
	for _, def := range defs {
		port, ok := def.(map[string]any)
		if !ok {
			t.Fatalf("port definition malformed: %#v", def)
		}
		name, nameOK := port["name"].(string)
		subject, subjectOK := port["subject"].(string)
		if !nameOK || !subjectOK {
			t.Fatalf("port name/subject malformed: %#v", port)
		}
		if _, exists := result[name]; exists {
			t.Fatalf("duplicate port name %q", name)
		}
		result[name] = subject
	}
	return result
}

func assertPortSubjects(t *testing.T, got, want map[string]string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("port count = %d, want %d; got %#v", len(got), len(want), got)
	}
	for name, wantSubject := range want {
		gotSubject, ok := got[name]
		if !ok {
			t.Fatalf("missing port %q; got %#v", name, got)
		}
		if gotSubject != wantSubject {
			t.Fatalf("port %q subject = %q, want %q", name, gotSubject, wantSubject)
		}
	}
}
