// Package urlsource provides the url-source processor component for semsource.
// It polls one or more HTTP/S URLs at a configurable interval and publishes
// page entity payloads to the NATS graph ingestion stream.
package urlsource

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/component"
)

// Config holds configuration for the url-source processor component.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// URLs is the list of HTTP/S URLs to ingest and poll.
	// At least one URL is required.
	URLs []string `json:"urls" schema:"type:array,description:HTTP/S URLs to ingest and poll,category:basic,required:true"`

	// Org is the organization namespace used in entity ID construction.
	Org string `json:"org" schema:"type:string,description:Organization namespace for entity IDs (e.g. acme),category:basic,required:true"`

	// PollInterval controls how often each URL is re-fetched to detect content
	// changes. Accepts Go duration strings (e.g. "60s", "5m"). Default: "300s".
	PollInterval string `json:"poll_interval" schema:"type:string,description:Polling interval for URL content changes (e.g. 60s 5m),category:advanced,default:300s"`

	// StreamName is the JetStream stream name for publishing entities.
	StreamName string `json:"stream_name" schema:"type:string,description:JetStream stream name,category:advanced,default:GRAPH"`

	// InstanceName is the unique component instance name for status tracking.
	// Set automatically by run.go to match the component map key.
	InstanceName string `json:"instance_name,omitempty" schema:"type:string,description:Unique component instance name for status tracking,category:internal"`
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if len(c.URLs) == 0 {
		return fmt.Errorf("at least one URL is required")
	}
	if c.Org == "" {
		return fmt.Errorf("org is required")
	}
	if c.PollInterval != "" {
		d, err := time.ParseDuration(c.PollInterval)
		if err != nil {
			return fmt.Errorf("invalid poll_interval format: %w", err)
		}
		if d <= 0 {
			return fmt.Errorf("poll_interval must be positive")
		}
	}
	return nil
}

// DefaultConfig returns the default configuration for the url-source processor.
func DefaultConfig() Config {
	outputDefs := []component.PortDefinition{
		{
			Name:        "graph.ingest",
			Type:        "jetstream",
			Subject:     "graph.ingest.entity",
			StreamName:  "GRAPH",
			Required:    true,
			Description: "Entity state updates for graph ingestion",
		},
	}

	return Config{
		Ports: &component.PortConfig{
			Outputs: outputDefs,
		},
		PollInterval: "300s",
		StreamName:   "GRAPH",
	}
}
