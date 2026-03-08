package astsource

import (
	"fmt"

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
		Schema:      astSourceSchema,
		Type:        "processor",
		Protocol:    "ast",
		Domain:      "semsource",
		Description: "Multi-language AST source for semsource code entity extraction and graph ingestion",
		Version:     "0.1.0",
	})
}
