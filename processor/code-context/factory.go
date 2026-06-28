package codecontext

import (
	"fmt"
	"reflect"

	"github.com/c360studio/semstreams/component"
)

// codeContextSchema defines the configuration schema for the code-context component.
var codeContextSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the code-context component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "code-context",
		Factory:     NewComponent,
		Schema:      codeContextSchema,
		Type:        "processor",
		Protocol:    "code-context",
		Domain:      "semsource",
		Description: "Serves fused code_context queries (verbatim source + structure) over NATS and HTTP",
		Version:     "0.2.0",
	})
}
