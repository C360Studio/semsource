package videosource

import (
	"encoding/json"
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
		Ports:       DeclarePorts,
		Schema:      videoSourceSchema,
		Type:        "processor",
		Protocol:    "video",
		Domain:      "semsource",
		Description: "Video source for semsource metadata and keyframe entity extraction",
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
