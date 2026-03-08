package docsource

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the doc-source processor component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "doc-source",
		Factory:     NewComponent,
		Schema:      docSourceSchema,
		Type:        "processor",
		Protocol:    "docs",
		Domain:      "semsource",
		Description: "Document source for semsource markdown and plain-text entity extraction",
		Version:     "0.1.0",
	})
}
