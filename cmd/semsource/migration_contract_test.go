package main

import (
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/graph"
)

func TestPinFactoryRegistration(t *testing.T) {
	if err := registerComponentFactories(component.NewRegistry()); err != nil {
		t.Fatal(err)
	}
}

// TestComposedComponentsDeclare proves the explicit registration covers every
// factory buildSemstreamsConfig composes (clustering on, so graph-clustering is
// included), and that the boot check refuses a factory nobody registered — the
// pinned ComponentManager would otherwise log it and skip it.
func TestComposedComponentsDeclare(t *testing.T) {
	registry := component.NewRegistry()
	if err := registerComponentFactories(registry); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Namespace: "acme", Graph: &config.GraphConfig{EnableClustering: true}}
	ssCfg, err := buildSemstreamsConfig(cfg, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ssCfg.Components["graph-clustering"]; !ok {
		t.Fatal("graph-clustering not composed; the test would not cover its registration")
	}
	for instance, cc := range ssCfg.Components {
		if _, err := registry.Declare(instance, cc); err != nil {
			t.Errorf("Declare(%s, factory %s): %v", instance, cc.Name, err)
		}
	}

	if err := requireRegisteredFactories(registry, ssCfg.Components); err != nil {
		t.Fatalf("composed set: %v", err)
	}
	ssCfg.Components["rule"] = types.ComponentConfig{Name: "rule", Type: types.ComponentTypeProcessor, Enabled: true}
	err = requireRegisteredFactories(registry, ssCfg.Components)
	if err == nil || !strings.Contains(err.Error(), `unregistered factory "rule"`) {
		t.Fatalf("unregistered factory: err = %v", err)
	}
}

func TestPinPayloadRegistryBootstrap(t *testing.T) {
	if _, err := buildPayloadRegistry(); err != nil {
		t.Fatal(err)
	}
}

func TestSourcePayloadBindsRegistryContract(t *testing.T) {
	reg, err := buildPayloadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	floor, ok := reg.IndexingProfileFor(graph.EntityType)
	if !ok || floor != graph.IndexingProfileControl {
		t.Fatalf("floor=%q registered=%v", floor, ok)
	}
	for _, contract := range reg.Contracts() {
		if contract.Name == graph.EntityType.Key() && contract.MessageType == graph.EntityType {
			return
		}
	}
	t.Fatal("source entity contract missing from payload authority")
}

// spec: openspec/specs/compose-deployment/spec.md#Requirement: Every documented config path boots
// spec: openspec/specs/tier1-consumer-qualification/spec.md#Requirement: Tier 1 is an explicit opt-in neural composition
func TestDefaultComposeConfigDeclaresComponents(t *testing.T) {
	for _, tc := range []struct {
		path      string
		wantEmbed bool
	}{
		{path: "../../configs/mvp.json"},
		{path: "../../configs/tiers/tier1-semantic.json", wantEmbed: true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			cfg, err := config.LoadConfig(tc.path)
			if err != nil {
				t.Fatalf("LoadConfig(%s): %v", tc.path, err)
			}
			if !cfg.Graph.LexicalSearchEnabled() {
				t.Fatalf("%s must run word search", tc.path)
			}
			if got := cfg.Graph.SemanticSearchEnabled(); got != tc.wantEmbed {
				t.Fatalf("%s graph.semantic_search = %v, want %v", tc.path, got, tc.wantEmbed)
			}
			ssCfg, err := buildSemstreamsConfig(cfg, cfg.Namespace)
			if err != nil {
				t.Fatalf("buildSemstreamsConfig(%s): %v", tc.path, err)
			}
			if _, ok := ssCfg.Components["graph-lexical"]; !ok {
				t.Fatalf("%s must compose graph-lexical", tc.path)
			}
			if _, ok := ssCfg.Components["graph-embedding"]; ok != tc.wantEmbed {
				t.Fatalf("%s composes graph-embedding = %v, want %v: tier 0 has no embedder", tc.path, ok, tc.wantEmbed)
			}
			registry := component.NewRegistry()
			if err := registerComponentFactories(registry, componentWiring{ModelRegistry: cfg.ModelRegistry}); err != nil {
				t.Fatal(err)
			}
			if err := requireRegisteredFactories(registry, ssCfg.Components); err != nil {
				t.Fatalf("requireRegisteredFactories: %v", err)
			}
			for instance, cc := range ssCfg.Components {
				if _, err := registry.Declare(instance, cc); err != nil {
					t.Errorf("Declare(%s, factory %s): %v", instance, cc.Name, err)
				}
			}
		})
	}
}
