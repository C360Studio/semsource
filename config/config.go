// Package config loads and validates the semsource.yaml configuration file.
package config

import "fmt"

// OutputConfig describes a single output endpoint for the flow.
type OutputConfig struct {
	// Name is a human-readable identifier for this output.
	Name string `yaml:"name"`

	// Type describes the output mechanism (e.g., "network").
	Type string `yaml:"type"`

	// Subject is the endpoint address (e.g., "http://0.0.0.0:7890/graph").
	Subject string `yaml:"subject"`
}

// FlowConfig holds transport and delivery settings.
type FlowConfig struct {
	// Outputs lists the downstream endpoints this flow writes to.
	Outputs []OutputConfig `yaml:"outputs"`

	// DeliveryMode controls acknowledgement semantics.
	// Defaults to "at-least-once".
	DeliveryMode string `yaml:"delivery_mode"`

	// AckTimeout is the maximum time to wait for an acknowledgement.
	// Must be a valid Go duration string. Defaults to "5s".
	AckTimeout string `yaml:"ack_timeout"`
}

// Config is the top-level semsource configuration.
type Config struct {
	// Namespace is the org identifier used in entity ID construction (e.g., "acme").
	Namespace string `yaml:"namespace"`

	// Flow configures transport delivery behaviour.
	Flow FlowConfig `yaml:"flow"`

	// Sources lists all ingestion sources.
	Sources []SourceEntry `yaml:"sources"`
}

// applyDefaults fills in omitted fields with their documented defaults.
func (c *Config) applyDefaults() {
	if c.Flow.DeliveryMode == "" {
		c.Flow.DeliveryMode = "at-least-once"
	}
	if c.Flow.AckTimeout == "" {
		c.Flow.AckTimeout = "5s"
	}
}

// Validate checks that all required fields are present and each source is valid.
func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("config: namespace is required")
	}
	if len(c.Flow.Outputs) == 0 {
		return fmt.Errorf("config: flow.outputs must contain at least one output")
	}
	if len(c.Sources) == 0 {
		return fmt.Errorf("config: sources must contain at least one source")
	}

	for i, src := range c.Sources {
		if err := src.Validate(); err != nil {
			return fmt.Errorf("config: sources[%d]: %w", i, err)
		}
	}

	return nil
}
