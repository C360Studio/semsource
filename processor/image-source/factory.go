package imagesource

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the image-source processor component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "image-source",
		Factory:     NewComponent,
		Schema:      imageSourceSchema,
		Type:        "processor",
		Protocol:    "image",
		Domain:      "semsource",
		Description: "Image source for semsource image metadata entity extraction",
		Version:     "0.1.0",
	})
}
