package objectstoresource

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the objectstore-source processor component with the given
// registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "objectstore-source",
		Factory:     NewComponent,
		Schema:      objectStoreSourceSchema,
		Type:        "processor",
		Protocol:    "s3",
		Domain:      "semsource",
		Description: "S3-compatible object store source for semsource document artifact ingestion",
		Version:     "0.1.0",
	})
}
