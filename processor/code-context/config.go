// Package codecontext provides the code-context fusion gateway component.
package codecontext

import "fmt"

// Config selects which lens the component instance serves. Run one instance per
// lens (a "code" instance and a "docs" instance); the rest of the behaviour is
// identical, driven by the shared fusion engine.
type Config struct {
	// SourceSystems is the complete boot source set used to constrain NL retrieval before ranking.
	SourceSystems []string `json:"source_systems,omitempty" schema:"type:array,description:Boot source system slugs for scoped retrieval,category:advanced"`
	// Lens is the domain this instance serves: "code" or "docs".
	Lens string `json:"lens" schema:"type:string,description:Lens to serve (code|docs),category:basic,required"`
	// SemanticEnabled is false when the composition runs no search index
	// (Tier 0 foundation). The search verb then refuses as not_enabled
	// instead of reporting the absent index as a transient outage.
	SemanticEnabled *bool `json:"semantic_enabled,omitempty" schema:"type:bool,description:Whether the composition runs a search index,default:true,category:advanced"`
}

func (c *Config) semanticEnabled() bool {
	return c.SemanticEnabled == nil || *c.SemanticEnabled
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
