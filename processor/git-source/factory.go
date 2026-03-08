package gitsource

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the git-source processor component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "git-source",
		Factory:     NewComponent,
		Schema:      gitSourceSchema,
		Type:        "processor",
		Protocol:    "git",
		Domain:      "semsource",
		Description: "Git repository source for semsource commit, author, and branch entity extraction",
		Version:     "0.1.0",
	})
}
