// Package sourcemanifest provides the source-manifest component for semsource.
// It publishes a manifest of configured sources to the graph stream and
// responds to NATS request/reply queries on graph.query.sources.
package sourcemanifest

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// Config holds configuration for the source-manifest component.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// Namespace is the organization namespace (e.g. "acme").
	Namespace string `json:"namespace" schema:"type:string,description:Organization namespace,category:basic,required:true"`

	// Sources is the resolved list of configured source entries.
	Sources []ManifestSource `json:"sources" schema:"type:array,description:Configured source entries,category:basic,required:true"`
}

// ManifestSource is the external-facing representation of a configured source.
// It contains only the fields useful for downstream consumers to understand
// what data SemSource is ingesting.
type ManifestSource struct {
	Type         string   `json:"type"`
	Path         string   `json:"path,omitempty"`
	Paths        []string `json:"paths,omitempty"`
	URL          string   `json:"url,omitempty"`
	URLs         []string `json:"urls,omitempty"`
	Language     string   `json:"language,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	Watch        bool     `json:"watch"`
	PollInterval string   `json:"poll_interval,omitempty"`
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	return nil
}

// DefaultConfig returns the default configuration for the source-manifest component.
func DefaultConfig() Config {
	outputDefs := []component.PortDefinition{
		{
			Name:        "graph.ingest",
			Type:        "jetstream",
			Subject:     "graph.ingest.manifest",
			StreamName:  "GRAPH",
			Required:    true,
			Description: "Source manifest broadcast for downstream consumers",
		},
	}

	return Config{
		Ports: &component.PortConfig{
			Outputs: outputDefs,
		},
	}
}
