package supersession

import (
	"fmt"
	"reflect"

	"github.com/c360studio/semstreams/component"
)

// supersessionSchema is the generated config schema for the component.
var supersessionSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// RegistryInterface is the minimal registry surface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the supersession component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "supersession",
		Factory:     NewComponent,
		Schema:      supersessionSchema,
		Type:        "processor",
		Protocol:    "lineage",
		Domain:      "semsource",
		Description: "Relates code entities across versions with directional supersession lineage edges",
		Version:     "0.1.0",
	})
}
