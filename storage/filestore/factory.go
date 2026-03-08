package filestore

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface required to register a
// component factory. Using a local interface keeps this package decoupled from
// any concrete registry implementation.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the filestore storage component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "filestore",
		Factory:     NewComponent,
		Schema:      filestoreSchema,
		Type:        "storage",
		Protocol:    "filesystem",
		Domain:      "semsource",
		Description: "Local filesystem storage backend for semsource binary content",
		Version:     "0.1.0",
	})
}
