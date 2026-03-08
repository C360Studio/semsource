// Package config loads and validates the semsource.json configuration file.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ObjectStoreConfig configures the NATS ObjectStore connection for binary content.
type ObjectStoreConfig struct {
	// NATSUrl is the NATS server URL (e.g., "nats://localhost:4222").
	NATSUrl string `json:"nats_url"`

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

// EntityStoreConfig configures persistent graph storage via NATS KV.
// When present, entities are persisted to the shared ENTITY_STATES bucket.
// When absent, entities are stored in-memory only.
type EntityStoreConfig struct {
	// NATSUrl is the NATS server URL (e.g., "nats://localhost:4222").
	NATSUrl string `json:"nats_url"`
}

// Config is the top-level semsource configuration.
type Config struct {
	// Namespace is the org identifier used in entity ID construction (e.g., "acme").
	Namespace string `json:"namespace"`

	// Flow configures transport delivery behaviour.
	Flow FlowConfig `json:"flow"`

	// Sources lists all ingestion sources.
	Sources []SourceEntry `json:"sources"`

	// EntityStore configures persistent graph storage.
	// When set, entities are persisted to the NATS KV ENTITY_STATES bucket.
	EntityStore *EntityStoreConfig `json:"entity_store,omitempty"`

	// ObjectStore configures binary content storage.
	// Required when using media sources (image, video).
	ObjectStore *ObjectStoreConfig `json:"object_store,omitempty"`

	// WorkspaceDir is the base directory where remote git repositories are
	// cloned. Defaults to ~/.semsource/repos when empty.
	WorkspaceDir string `json:"workspace_dir,omitempty"`

	// GitToken is a personal access token or GitHub App installation token
	// for authenticating HTTPS clones of private repositories.
	// Can also be set via the SEMSOURCE_GIT_TOKEN environment variable.
	GitToken string `json:"git_token,omitempty"`

	// MediaStoreDir is the root directory used by media source components
	// (image, video, audio) to store binary content on the local filesystem.
	// When empty, media processors operate in metadata-only mode.
	MediaStoreDir string `json:"media_store_dir,omitempty"`
}

// applyDefaults fills in omitted fields with their documented defaults.
func (c *Config) applyDefaults() {
	if c.Flow.DeliveryMode == "" {
		c.Flow.DeliveryMode = "at-least-once"
	}
	if c.Flow.AckTimeout == "" {
		c.Flow.AckTimeout = "5s"
	}
	if c.WorkspaceDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			c.WorkspaceDir = filepath.Join(home, ".semsource", "repos")
		}
	}
	// Allow token to be set via environment variable (avoids putting secrets in config files).
	if c.GitToken == "" {
		c.GitToken = os.Getenv("SEMSOURCE_GIT_TOKEN")
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

	if c.ObjectStore != nil {
		if c.ObjectStore.Bucket == "" {
			return fmt.Errorf("config: object_store.bucket is required")
		}
		if c.ObjectStore.NATSUrl == "" {
			return fmt.Errorf("config: object_store.nats_url is required")
		}
	}

	return nil
}
