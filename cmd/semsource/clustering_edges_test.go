package main

import (
	"encoding/json"
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

func boolPtr(b bool) *bool { return &b }

// TestClusteringEdges_OmittedWhenUnset pins the no-op guarantee: a config that
// says nothing about edge synthesis must produce a clustering config carrying
// no synthesis key at all. The substrate reads absence as "keep the defaults",
// so emitting an empty object here would be a behavior change disguised as a
// passthrough.
func TestClusteringEdges_OmittedWhenUnset(t *testing.T) {
	cfg := &config.Config{
		Namespace: "acme",
		Graph:     &config.GraphConfig{EnableClustering: true},
	}

	got, ok := clusteringConfigFor(t, cfg)
	if !ok {
		t.Fatal("graph-clustering component not composed with EnableClustering: true")
	}
	if raw, present := got["entity_id_edges"]; present {
		t.Errorf("entity_id_edges must be absent when unset, got %s", raw)
	}
}

// TestClusteringEdges_TriStatePreserved is the load-bearing one: setting a
// single toggle must emit exactly that key. A nil *bool marshalled as false
// would disable synthesis the operator never asked to disable — and that is
// precisely the collapse this config exists to control.
func TestClusteringEdges_TriStatePreserved(t *testing.T) {
	cfg := &config.Config{
		Namespace: "acme",
		Graph: &config.GraphConfig{
			EnableClustering: true,
			EntityIDEdges: &config.EntityIDEdgesConfig{
				IncludeSystemPeers: boolPtr(false),
			},
		},
	}

	got, ok := clusteringConfigFor(t, cfg)
	if !ok {
		t.Fatal("graph-clustering component not composed")
	}
	raw, present := got["entity_id_edges"]
	if !present {
		t.Fatal("entity_id_edges absent although include_system_peers was set")
	}

	var edges map[string]any
	if err := json.Unmarshal(raw, &edges); err != nil {
		t.Fatalf("unmarshal entity_id_edges: %v", err)
	}
	if v, present := edges["include_system_peers"]; !present || v != false {
		t.Errorf("include_system_peers = %v (present %v), want false", v, present)
	}
	if len(edges) != 1 {
		t.Errorf("exactly one key expected, got %v — an unset field was manufactured", edges)
	}
}

// TestClusteringEdges_InertWithoutClustering covers the tier-0/1 case: the
// block is accepted but composes nothing, because those tiers run no
// clustering. Carrying it must not be an error, so one config file can serve a
// deployment that turns clustering on later.
func TestClusteringEdges_InertWithoutClustering(t *testing.T) {
	cfg := &config.Config{
		Namespace: "acme",
		Sources:   []config.SourceEntry{{Type: "docs", Paths: []string{"."}}},
		Graph: &config.GraphConfig{
			EntityIDEdges: &config.EntityIDEdgesConfig{IncludeSystemPeers: boolPtr(false)},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("config carrying entity_id_edges without clustering must validate, got %v", err)
	}
	if _, ok := clusteringConfigFor(t, cfg); ok {
		t.Error("graph-clustering composed although EnableClustering is false")
	}
}

// TestClusteringEdges_RejectsNegativeNumerics keeps a nonsensical value from
// reaching the substrate, where a negative cap or weight has no defined
// meaning. Omission, not a negative number, is how an operator asks for the
// default.
func TestClusteringEdges_RejectsNegativeNumerics(t *testing.T) {
	for name, edges := range map[string]*config.EntityIDEdgesConfig{
		"sibling_weight":     {SiblingWeight: -0.5},
		"system_peer_weight": {SystemPeerWeight: -1},
		"max_siblings":       {MaxSiblings: -1},
		"max_system_peers":   {MaxSystemPeers: -3},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{
				Namespace: "acme",
				Sources:   []config.SourceEntry{{Type: "docs", Paths: []string{"."}}},
				Graph:     &config.GraphConfig{EnableClustering: true, EntityIDEdges: edges},
			}
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("negative %s must be rejected", name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error must name the offending field %q, got %v", name, err)
			}
		})
	}
}
