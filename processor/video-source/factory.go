package videosource

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the video-source processor component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "video-source",
		Factory:     NewComponent,
		Schema:      videoSourceSchema,
		Type:        "processor",
		Protocol:    "video",
		Domain:      "semsource",
		Description: "Video source for semsource metadata and keyframe entity extraction",
		Version:     "0.1.0",
	})
}
