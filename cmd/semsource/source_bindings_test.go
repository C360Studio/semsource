package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourceintent"
	"github.com/c360studio/semsource/internal/sourcelifecycle"
	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/types"
)

type bindingProbe struct {
	component.Discoverable
	binding sourceintent.Binding
}

func (p *bindingProbe) BindSourceLifecycle(b sourceintent.Binding, _ sourceintent.PublicationObserver) error {
	p.binding = b
	return nil
}

type bindingObserver struct{}

func (bindingObserver) Published(context.Context, sourceintent.Publication) error     { return nil }
func (bindingObserver) SeedFinished(context.Context, sourceintent.SeedManifest) error { return nil }

func TestSourceFactoryBindings(t *testing.T) {
	cfg := types.ComponentConfig{Name: "doc-source", Type: "processor", Enabled: true, Config: json.RawMessage(`{"instance_name":"docs","paths":["/docs"]}`)}
	digest, err := sourcelifecycle.ConfigDigest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	binding := sourceintent.Binding{Authority: entityid.Authority{Org: "acme", Platform: "p"}, Namespace: "ns", Handle: "docs", Generation: 2, BootEpoch: "boot", Factory: "doc-source", ConfigDigest: digest}
	configs := semconfig.ComponentConfigs{"docs": cfg}
	registry := component.NewRegistry()
	decorated, err := newLifecycleRegistry(registry, configs, map[string]sourceintent.Binding{"docs": binding}, bindingObserver{})
	if err != nil {
		t.Fatal(err)
	}
	probe := &bindingProbe{}
	if err := decorated.RegisterWithConfig(component.RegistrationConfig{Name: "doc-source", Type: "processor", Protocol: "docs", Domain: "semsource", Version: "0.1.0", Ports: func(json.RawMessage, string) (component.PortConfig, error) { return component.PortConfig{}, nil }, Factory: func(json.RawMessage, component.Dependencies) (component.Discoverable, error) { return probe, nil }}); err != nil {
		t.Fatal(err)
	}
	factory, ok := registry.GetFactory("doc-source")
	if !ok {
		t.Fatal("factory not preserved")
	}
	if _, err := factory(cfg.Config, component.Dependencies{Platform: component.PlatformMeta{Org: "acme", Platform: "p"}}); err != nil {
		t.Fatal(err)
	}
	if probe.binding != binding {
		t.Fatal("wrong immutable binding")
	}
	for _, raw := range []string{`{"instance_name":"docs","paths":["/different"]}`, `{"instance_name":"other","paths":["/docs"]}`, `{"paths":["/docs"]}`} {
		if _, err := factory(json.RawMessage(raw), component.Dependencies{}); err == nil {
			t.Fatalf("accepted unadmitted raw config %s", raw)
		}
	}
	for _, bad := range []types.ComponentConfig{{Name: "doc-source", Enabled: true, Config: json.RawMessage(`{"paths":[]}`)}, {Name: "doc-source", Enabled: true, Config: json.RawMessage(`{"instance_name":"other"}`)}} {
		if _, err := newLifecycleRegistry(component.NewRegistry(), semconfig.ComponentConfigs{"docs": bad}, nil, nil); err == nil {
			t.Fatal("preflight admitted missing/mismatched instance_name")
		}
	}
}
