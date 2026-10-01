package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/internal/sourcelifecycle"
	"github.com/c360studio/semstreams/component"
)

type localProjectionProbe struct {
	component.Discoverable
	calls int
}

func (p *localProjectionProbe) ApplyRemoval(context.Context, sourceintent.RemovalProjection) (sourceintent.ProjectionResult, error) {
	p.calls++
	return sourceintent.ProjectionResult{Complete: true}, nil
}
func (p *localProjectionProbe) ApplyReactivation(context.Context, sourceintent.ReactivationProjection) (sourceintent.ProjectionResult, error) {
	p.calls++
	return sourceintent.ProjectionResult{Complete: true}, nil
}
func TestProjectionFactoryBindsSingleLocalOwner(t *testing.T) {
	registry := component.NewRegistry()
	bridge := &sourcelifecycle.LocalProjector{}
	decorator := &projectionRegistry{registry: registry, projector: bridge}
	probe := &localProjectionProbe{}
	declare := func(json.RawMessage, string) (component.PortConfig, error) { return component.PortConfig{}, nil }
	registration := component.RegistrationConfig{Name: "supersession", Type: "processor", Protocol: "lineage", Domain: "semsource", Version: "0.1.0", Ports: declare, Factory: func(json.RawMessage, component.Dependencies) (component.Discoverable, error) { return probe, nil }}
	if err := decorator.RegisterWithConfig(registration); err != nil {
		t.Fatal(err)
	}
	factory, ok := registry.GetFactory("supersession")
	if !ok {
		t.Fatal("factory not registered")
	}
	if _, err := factory(json.RawMessage(`{}`), component.Dependencies{}); err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.ApplyRemoval(context.Background(), sourceintent.RemovalProjection{}); err != nil || probe.calls != 1 {
		t.Fatalf("local owner not reached: calls=%d err=%v", probe.calls, err)
	}
	if _, err := factory(json.RawMessage(`{}`), component.Dependencies{}); err == nil {
		t.Fatal("second component silently replaced owner")
	}
}
