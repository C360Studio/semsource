// Package codecontext provides the code-context fusion gateway component.
package codecontext

import "fmt"

// Config selects which lens the component instance serves. Run one instance per
// lens (a "code" instance and a "docs" instance); the rest of the behaviour is
// identical, driven by the shared fusion engine.
type Config struct {
	// Lens is the domain this instance serves: "code" or "docs".
	Lens string `json:"lens" schema:"type:string,description:Lens to serve (code|docs),category:basic,required:true"`
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if c.Lens != "code" && c.Lens != "docs" {
		return fmt.Errorf("lens must be \"code\" or \"docs\", got %q", c.Lens)
	}
	return nil
}

// DefaultConfig returns the default configuration (code lens).
func DefaultConfig() Config {
	return Config{Lens: "code"}
}
