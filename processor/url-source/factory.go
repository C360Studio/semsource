package urlsource

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the url-source processor component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "url-source",
		Factory:     NewComponent,
		Schema:      urlSourceSchema,
		Type:        "processor",
		Protocol:    "url",
		Domain:      "semsource",
		Description: "HTTP/S URL source for semsource web page entity extraction and change detection",
		Version:     "0.1.0",
	})
}
