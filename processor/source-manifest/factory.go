package sourcemanifest

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/c360studio/semstreams/component"
)

// manifestSchema defines the configuration schema for the source-manifest component.
var manifestSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the source-manifest component with the given registry.
func Register(registry RegistryInterface, ingest ...IngestHandlerConfig) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name: "source-manifest",
		Factory: func(raw json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
			built, err := NewComponent(raw, deps)
			if err != nil {
				return nil, err
			}
			if len(ingest) > 0 {
				cfg := ingest[0]
				built.(*Component).ingestCfg = &cfg
			}
			return built, nil
		},
		Ports:       DeclarePorts,
		Schema:      manifestSchema,
		Type:        "processor",
		Protocol:    "manifest",
		Domain:      "semsource",
		Description: "Publishes configured source manifest to graph stream and serves NATS queries",
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
