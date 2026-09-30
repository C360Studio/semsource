package gitsource

import (
	"encoding/json"
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the git-source processor component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "git-source",
		Factory:     NewComponent,
		Ports:       DeclarePorts,
		Schema:      gitSourceSchema,
		Type:        "processor",
		Protocol:    "git",
		Domain:      "semsource",
		Description: "Git repository source for semsource commit, author, and branch entity extraction",
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
