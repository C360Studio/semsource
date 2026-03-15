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

	// ExpectedSourceCount is the number of source components to wait for
	// before declaring seed-complete. Set by run.go during config construction.
	ExpectedSourceCount int `json:"expected_source_count" schema:"type:int,description:Number of source components to expect,category:status"`

	// SeedTimeout is how long to wait for all sources to report before
	// marking status as degraded. Defaults to "120s".
	SeedTimeout string `json:"seed_timeout,omitempty" schema:"type:string,description:Timeout waiting for all sources to report,category:status"`

	// HeartbeatInterval is how often to re-publish status. Defaults to "30s".
	HeartbeatInterval string `json:"heartbeat_interval,omitempty" schema:"type:string,description:Status heartbeat interval,category:status"`
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
		{
			Name:        "graph.ingest.status",
			Type:        "jetstream",
			Subject:     "graph.ingest.status",
			StreamName:  "GRAPH",
			Required:    false,
			Description: "Ingestion status broadcast for downstream consumers",
		},
		{
			Name:        "graph.ingest.predicates",
			Type:        "jetstream",
			Subject:     "graph.ingest.predicates",
			StreamName:  "GRAPH",
			Required:    false,
			Description: "Predicate schema broadcast for downstream consumers",
		},
	}

	return Config{
		Ports: &component.PortConfig{
			Outputs: outputDefs,
		},
		SeedTimeout:       "120s",
		HeartbeatInterval: "30s",
	}
}
