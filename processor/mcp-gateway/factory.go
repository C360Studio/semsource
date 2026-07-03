package mcpgateway

import (
	"fmt"
	"reflect"

	"github.com/c360studio/semstreams/component"
)

// mcpGatewaySchema is the config schema derived from Config.
var mcpGatewaySchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// RegistryInterface is the minimal registry surface needed to register.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the mcp-gateway component factory.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "mcp-gateway",
		Factory:     NewComponent,
		Schema:      mcpGatewaySchema,
		Type:        "processor",
		Protocol:    "mcp",
		Domain:      "semsource",
		Description: "MCP gateway exposing source-registration tools over Streamable HTTP",
		Version:     "0.1.0",
	})
}
