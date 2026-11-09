package main

import (
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/types"
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
	floor, ok := reg.IndexingProfileFor(graph.EntityType.Key())
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
