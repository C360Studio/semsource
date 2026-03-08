package audiosource

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the audio-source processor component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "audio-source",
		Factory:     NewComponent,
		Schema:      audioSourceSchema,
		Type:        "processor",
		Protocol:    "audio",
		Domain:      "semsource",
		Description: "Audio source for semsource audio file metadata extraction",
		Version:     "0.1.0",
	})
}
