package codecontext

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/c360studio/semstreams/component"
)

// codeContextSchema defines the configuration schema for the code-context component.
var codeContextSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the code-context component with the given registry.
func Register(registry RegistryInterface, systems ...map[string][]string) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name: "code-context",
		Factory: func(raw json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
			cfg := DefaultConfig()
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return nil, err
			}
			if len(systems) > 0 {
				cfg.SourceSystems = append([]string{}, systems[0][cfg.Lens]...)
			}
			effective, err := json.Marshal(cfg)
			if err != nil {
				return nil, err
			}
			return NewComponent(effective, deps)
		},
		Ports:       DeclarePorts,
		Schema:      codeContextSchema,
		Type:        "processor",
		Protocol:    "code-context",
		Domain:      "semsource",
		Description: "Serves fused code_context queries (verbatim source + structure) over NATS and HTTP",
		Version:     "0.2.0",
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
	return component.PortConfig{}, nil
}
