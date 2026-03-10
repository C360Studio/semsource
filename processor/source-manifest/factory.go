package sourcemanifest

import (
	"fmt"
	"reflect"

	"github.com/c360studio/semstreams/component"
)

// manifestSchema defines the configuration schema for the source-manifest component.
var manifestSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the source-manifest component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "source-manifest",
		Factory:     NewComponent,
		Schema:      manifestSchema,
		Type:        "processor",
		Protocol:    "manifest",
		Domain:      "semsource",
		Description: "Publishes configured source manifest to graph stream and serves NATS queries",
		Version:     "0.1.0",
	})
}
