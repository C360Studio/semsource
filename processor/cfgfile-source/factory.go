package cfgfilesource

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the cfgfile-source processor component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "cfgfile-source",
		Factory:     NewComponent,
		Schema:      cfgfileSourceSchema,
		Type:        "processor",
		Protocol:    "config",
		Domain:      "semsource",
		Description: "Config file source for semsource module, package, image, and dependency entity extraction",
		Version:     "0.1.0",
	})
}
