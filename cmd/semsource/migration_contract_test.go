package main

import (
	"github.com/c360studio/semsource/graph"
	"github.com/c360studio/semstreams/component"
	"testing"
)

func TestPinFactoryRegistration(t *testing.T) {
	if err := registerComponentFactories(component.NewRegistry()); err != nil {
		t.Fatal(err)
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
