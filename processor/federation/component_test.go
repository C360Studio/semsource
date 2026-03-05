package federation_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semsource/processor/federation"
	"github.com/c360studio/semstreams/component"
)

func TestNewComponent_DefaultConfig(t *testing.T) {
	rawCfg, _ := json.Marshal(map[string]string{
		"local_namespace": "acme",
		"merge_policy":    "standard",
	})

	comp, err := federation.NewComponent(rawCfg, component.Dependencies{})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	if comp == nil {
		t.Fatal("expected non-nil component")
	}
}

func TestNewComponent_EmptyConfig_UsesDefaults(t *testing.T) {
	// Empty config uses DefaultConfig() — LocalNamespace="public", which is valid.
	comp, err := federation.NewComponent(json.RawMessage("{}"), component.Dependencies{})
	if err != nil {
		t.Fatalf("NewComponent empty config: %v", err)
	}
	if comp == nil {
		t.Fatal("expected non-nil component")
	}
}

func TestNewComponent_InvalidConfig_ReturnsError(t *testing.T) {
	// MergePolicy missing
	rawCfg, _ := json.Marshal(map[string]string{
		"local_namespace": "acme",
		"merge_policy":    "bogus-policy",
	})
	_, err := federation.NewComponent(rawCfg, component.Dependencies{})
	if err == nil {
		t.Error("expected error for invalid merge policy")
	}
}

func TestNewComponent_InvalidJSON_ReturnsError(t *testing.T) {
	_, err := federation.NewComponent(json.RawMessage(`{not valid json}`), component.Dependencies{})
	if err == nil {
		t.Error("expected error for invalid JSON config")
	}
}

func TestComponent_Meta(t *testing.T) {
	rawCfg, _ := json.Marshal(map[string]string{
		"local_namespace": "acme",
		"merge_policy":    "standard",
	})
	comp, err := federation.NewComponent(rawCfg, component.Dependencies{})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}

	meta := comp.Meta()
	if meta.Name != "federation-processor" {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, "federation-processor")
	}
	if meta.Type != "processor" {
		t.Errorf("Meta.Type = %q, want %q", meta.Type, "processor")
	}
	if meta.Version == "" {
		t.Error("Meta.Version must not be empty")
	}
	if meta.Description == "" {
		t.Error("Meta.Description must not be empty")
	}
}

func TestComponent_InputPorts(t *testing.T) {
	rawCfg, _ := json.Marshal(map[string]string{
		"local_namespace": "acme",
		"merge_policy":    "standard",
	})
	comp, err := federation.NewComponent(rawCfg, component.Dependencies{})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}

	ports := comp.InputPorts()
	if len(ports) != 1 {
		t.Fatalf("expected 1 input port, got %d", len(ports))
	}
	if ports[0].Name == "" {
		t.Error("InputPort.Name must not be empty")
	}
	if ports[0].Direction != component.DirectionInput {
		t.Errorf("InputPort.Direction = %v, want %v", ports[0].Direction, component.DirectionInput)
	}
	if !ports[0].Required {
		t.Error("InputPort.Required must be true")
	}
}

func TestComponent_OutputPorts(t *testing.T) {
	rawCfg, _ := json.Marshal(map[string]string{
		"local_namespace": "acme",
		"merge_policy":    "standard",
	})
	comp, err := federation.NewComponent(rawCfg, component.Dependencies{})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}

	ports := comp.OutputPorts()
	if len(ports) != 1 {
		t.Fatalf("expected 1 output port, got %d", len(ports))
	}
	if ports[0].Direction != component.DirectionOutput {
		t.Errorf("OutputPort.Direction = %v, want %v", ports[0].Direction, component.DirectionOutput)
	}
	if !ports[0].Required {
		t.Error("OutputPort.Required must be true")
	}
}

func TestComponent_ConfigSchema(t *testing.T) {
	rawCfg, _ := json.Marshal(map[string]string{
		"local_namespace": "acme",
		"merge_policy":    "standard",
	})
	comp, err := federation.NewComponent(rawCfg, component.Dependencies{})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}

	schema := comp.ConfigSchema()
	if len(schema.Properties) == 0 {
		t.Error("ConfigSchema.Properties must not be empty")
	}
}

func TestComponent_Health_StoppedComponent(t *testing.T) {
	rawCfg, _ := json.Marshal(map[string]string{
		"local_namespace": "acme",
		"merge_policy":    "standard",
	})
	comp, err := federation.NewComponent(rawCfg, component.Dependencies{})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}

	health := comp.Health()
	// Component has not been started — should report as not healthy.
	if health.Healthy {
		t.Error("Health.Healthy should be false for stopped component")
	}
	if health.Status == "" {
		t.Error("Health.Status must not be empty")
	}
}

func TestComponent_DataFlow(t *testing.T) {
	rawCfg, _ := json.Marshal(map[string]string{
		"local_namespace": "acme",
		"merge_policy":    "standard",
	})
	comp, err := federation.NewComponent(rawCfg, component.Dependencies{})
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}

	// DataFlow must return without panicking.
	flow := comp.DataFlow()
	_ = flow
}

func TestRegister_NilRegistry_ReturnsError(t *testing.T) {
	err := federation.Register(nil)
	if err == nil {
		t.Error("expected error for nil registry")
	}
}

func TestRegister_ValidRegistry(t *testing.T) {
	registry := component.NewRegistry()
	err := federation.Register(registry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Verify the component type is listed.
	types := registry.ListComponentTypes()
	found := false
	for _, name := range types {
		if name == "federation-processor" {
			found = true
			break
		}
	}
	if !found {
		t.Error("federation-processor not found in registry after Register()")
	}
}

func TestRegister_Idempotency(t *testing.T) {
	registry := component.NewRegistry()
	if err := federation.Register(registry); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	// Second registration should fail (duplicate factory).
	if err := federation.Register(registry); err == nil {
		t.Error("expected error on duplicate registration")
	}
}

func TestRegister_SchemaAvailable(t *testing.T) {
	registry := component.NewRegistry()
	if err := federation.Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}

	schema, err := registry.GetComponentSchema("federation-processor")
	if err != nil {
		t.Fatalf("GetComponentSchema: %v", err)
	}
	if len(schema.Properties) == 0 {
		t.Error("expected non-empty schema properties after registration")
	}
}
