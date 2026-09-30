package supersession

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/c360studio/semstreams/component"
)

// supersessionSchema is the generated config schema for the component.
var supersessionSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// RegistryInterface is the minimal registry surface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the supersession component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "supersession",
		Factory:     NewComponent,
		Ports:       DeclarePorts,
		Schema:      supersessionSchema,
		Type:        "processor",
		Protocol:    "lineage",
		Domain:      "semsource",
		Description: "Relates code entities across versions with directional supersession lineage edges",
		Version:     "0.1.0",
	})
}

// DeclarePorts reports the constructor's ports without acquiring runtime resources.
func DeclarePorts(raw json.RawMessage, _ string) (component.PortConfig, error) {
	cfg := DefaultConfig()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return component.PortConfig{}, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return component.PortConfig{}, err
	}
	if cfg.Ports == nil {
		return component.PortConfig{}, nil
	}
	return component.PortConfig{Outputs: cfg.Ports.Outputs}, nil
}
