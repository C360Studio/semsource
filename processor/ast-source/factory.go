package astsource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the ast-source processor component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "ast-source",
		Factory:     NewComponent,
		Ports:       DeclarePorts,
		Schema:      astSourceSchema,
		Type:        "processor",
		Protocol:    "ast",
		Domain:      "semsource",
		Description: "Multi-language AST source for semsource code entity extraction and graph ingestion",
		Version:     "0.1.0",
	})
}

// DeclarePorts reports the constructor's ports without acquiring runtime resources.
func DeclarePorts(raw json.RawMessage, _ string) (component.PortConfig, error) {
	cfg := DefaultConfig()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return component.PortConfig{}, fmt.Errorf("decode config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return component.PortConfig{}, fmt.Errorf("config must contain one JSON value")
	}
	if err := cfg.Validate(); err != nil {
		return component.PortConfig{}, err
	}
	if cfg.Ports == nil {
		return component.PortConfig{}, nil
	}
	return component.PortConfig{Outputs: cfg.Ports.Outputs}, nil
}
