// Package config loads and validates the semsource.json configuration file.
package config

import "fmt"

// ObjectStoreConfig configures the NATS ObjectStore connection for binary content.
type ObjectStoreConfig struct {
	// Bucket is the NATS JetStream ObjectStore bucket name.
	Bucket string `json:"bucket"`

	// TTL is the retention period for stored objects (Go duration string).
	// Empty means no expiration.
	TTL string `json:"ttl"`
}

// OutputConfig describes a single output endpoint for the flow.
type OutputConfig struct {
	// Name is a human-readable identifier for this output.
	Name string `json:"name"`

	// Type describes the output mechanism (e.g., "network").
	Type string `json:"type"`

	// Subject is the endpoint address (e.g., "http://0.0.0.0:7890/graph").
	Subject string `json:"subject"`
}

// FlowConfig holds transport and delivery settings.
type FlowConfig struct {
	// Outputs lists the downstream endpoints this flow writes to.
	Outputs []OutputConfig `json:"outputs"`

	// DeliveryMode controls acknowledgement semantics.
	// Defaults to "at-least-once".
	DeliveryMode string `json:"delivery_mode"`

	// AckTimeout is the maximum time to wait for an acknowledgement.
	// Must be a valid Go duration string. Defaults to "5s".
	AckTimeout string `json:"ack_timeout"`
}

// Config is the top-level semsource configuration.
type Config struct {
	// Namespace is the org identifier used in entity ID construction (e.g., "acme").
	Namespace string `json:"namespace"`

	// Flow configures transport delivery behaviour.
	Flow FlowConfig `json:"flow"`

	// Sources lists all ingestion sources.
	Sources []SourceEntry `json:"sources"`

	// ObjectStore configures binary content storage.
	// Required when using media sources (image, video).
	ObjectStore *ObjectStoreConfig `json:"object_store,omitempty"`
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

	// Media sources require an ObjectStore to be configured for binary content.
	for _, src := range c.Sources {
		if src.Type == "image" || src.Type == "video" {
			if c.ObjectStore == nil {
				return fmt.Errorf("config: object_store is required when using %q sources", src.Type)
			}
			break
		}
	}

	if c.ObjectStore != nil && c.ObjectStore.Bucket == "" {
		return fmt.Errorf("config: object_store.bucket is required")
	}

	return nil
}
