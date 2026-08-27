// Package objectstoresource provides the objectstore-source processor
// component for semsource. It ingests document artifacts out of an
// S3-compatible bucket and publishes document entity payloads to the NATS
// graph ingestion stream.
package objectstoresource

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/component"

	"github.com/c360studio/semsource/storage/s3store"
)

// DefaultPollInterval is how often the source re-lists its prefix when the
// configuration does not choose. An object store announces nothing, so this is
// the whole latency between an artifact appearing and the graph holding it.
const DefaultPollInterval = time.Minute

// Config holds configuration for the objectstore-source processor component.
//
// It carries no credentials, and must not grow any: this document is watched
// and replicated through KV, so an access key placed here would be distributed
// well beyond the process that needs it. Credentials come from the process
// environment — see s3store.EnvAccessKeyID.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// Bucket is the object store bucket to ingest from. Required.
	Bucket string `json:"bucket" schema:"type:string,description:Bucket holding the document artifacts,category:basic,required:true"`

	// Prefix scopes ingestion to part of the bucket. Empty means the whole
	// bucket, which is only safe where the bucket holds documents and nothing
	// else.
	Prefix string `json:"prefix,omitempty" schema:"type:string,description:Key prefix to scope ingestion to,category:basic"`

	// Endpoint is the store's base URL, scheme included. Empty means AWS.
	Endpoint string `json:"endpoint,omitempty" schema:"type:string,description:S3-compatible endpoint URL (defaults to AWS),category:basic"`

	// Region is forwarded to the store for request signing.
	Region string `json:"region,omitempty" schema:"type:string,description:Region forwarded for request signing,category:basic"`

	// PathStyle selects path-style bucket addressing, which self-hosted stores
	// reached by IP or by a name with no wildcard DNS require.
	PathStyle bool `json:"path_style,omitempty" schema:"type:bool,description:Use path-style bucket addressing,category:basic"`

	// Org is the organization namespace used in entity ID construction.
	Org string `json:"org" schema:"type:string,description:Organization namespace for entity IDs (e.g. acme),category:basic,required:true"`

	// Project, when non-empty, overrides the bucket-derived entity-ID system
	// slug. Two prefixes of one bucket meant to be separate projects say so
	// this way.
	Project string `json:"project,omitempty" schema:"type:string,description:Explicit project identity overriding the bucket-derived entity-ID system slug"`

	// Version, when non-empty, scopes entity identity so two snapshots of one
	// corpus can coexist. Empty keeps identifiers byte-for-byte.
	Version string `json:"version,omitempty" schema:"type:string,description:Explicit version scoping this registration's entity IDs"`

	// WatchEnabled controls whether the source keeps re-listing after the
	// initial ingest. When false the component ingests once and stops.
	WatchEnabled bool `json:"watch_enabled" schema:"type:bool,description:Re-list the prefix on an interval to pick up changes,category:basic,default:true"`

	// PollIntervalMs is how often to re-list. 0 uses DefaultPollInterval.
	PollIntervalMs int `json:"poll_interval_ms,omitempty" schema:"type:int,description:Re-listing interval in ms. 0 uses the built-in default (60000),category:advanced"`

	// BodyStoreRoot is the local directory holding verbatim passage bodies.
	// Bodies are stored locally by design: the bucket is read-only, and where
	// a body lands has no bearing on identity, which comes from the object key.
	BodyStoreRoot string `json:"body_store_root,omitempty" schema:"type:string,description:Local directory for verbatim passage bodies,category:advanced"`

	// InstanceName is the unique component instance name for status tracking.
	// Set automatically by run.go to match the component map key.
	InstanceName string `json:"instance_name,omitempty" schema:"type:string,description:Unique component instance name for status tracking,category:internal"`
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if c.Org == "" {
		return fmt.Errorf("org is required")
	}
	if c.PollIntervalMs < 0 {
		return fmt.Errorf("poll_interval_ms must not be negative")
	}
	// The store owns endpoint and addressing validation, so there is one
	// definition of a well-formed endpoint rather than two that can disagree.
	store := s3store.Config{
		Bucket:    c.Bucket,
		Endpoint:  c.Endpoint,
		Region:    c.Region,
		PathStyle: c.PathStyle,
	}
	return store.Validate()
}

// StoreConfig returns the object-store configuration this source implies.
func (c *Config) StoreConfig() s3store.Config {
	return s3store.Config{
		Bucket:    c.Bucket,
		Endpoint:  c.Endpoint,
		Region:    c.Region,
		PathStyle: c.PathStyle,
	}
}

// PollInterval returns the configured re-listing interval, or the default.
func (c *Config) PollInterval() time.Duration {
	if c.PollIntervalMs > 0 {
		return time.Duration(c.PollIntervalMs) * time.Millisecond
	}
	return DefaultPollInterval
}

// DefaultConfig returns the default configuration for the objectstore-source
// processor.
func DefaultConfig() Config {
	outputDefs := []component.PortDefinition{
		{
			Name: "graph.ingest",
			Config: component.JetStreamPort{
				StreamName: "GRAPH",
				Subjects:   []string{"graph.ingest.entity"},
			},
			Required:    true,
			Description: "Entity state updates for graph ingestion",
		},
	}

	return Config{
		Ports: &component.PortConfig{
			Outputs: outputDefs,
		},
		WatchEnabled: true,
	}
}
