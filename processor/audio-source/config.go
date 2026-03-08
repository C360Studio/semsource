// Package audiosource provides the audio-source processor component for semsource.
// It ingests audio file directories and publishes audio entity payloads to the
// NATS graph ingestion stream. Metadata is extracted via ffprobe; binary storage
// is not used by default (metadata-only mode).
package audiosource

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// Config holds configuration for the audio-source processor component.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// Paths is the list of filesystem directories to walk for audio files.
	// At least one path is required.
	Paths []string `json:"paths" schema:"type:array,description:Filesystem directories to scan for audio files (.mp3 .wav .flac .aac .ogg .m4a .wma),category:basic,required:true"`

	// Org is the organization namespace used in entity ID construction.
	Org string `json:"org" schema:"type:string,description:Organization namespace for entity IDs (e.g. acme),category:basic,required:true"`

	// WatchEnabled controls whether fsnotify watching is active after the
	// initial ingest. When false the component exits after the initial walk.
	WatchEnabled bool `json:"watch_enabled" schema:"type:bool,description:Enable fsnotify watching for live file changes,category:basic,default:true"`

	// StreamName is the JetStream stream name for publishing entities.
	StreamName string `json:"stream_name" schema:"type:string,description:JetStream stream name,category:advanced,default:GRAPH"`

	// FileStoreRoot is the root directory for local filesystem binary storage.
	// When empty, the handler operates in metadata-only mode (no binary storage).
	FileStoreRoot string `json:"file_store_root" schema:"type:string,description:Root directory for local filesystem binary storage (empty = metadata-only),category:advanced"`
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if len(c.Paths) == 0 {
		return fmt.Errorf("at least one path is required")
	}
	for i, p := range c.Paths {
		if p == "" {
			return fmt.Errorf("paths[%d] must not be empty", i)
		}
	}
	if c.Org == "" {
		return fmt.Errorf("org is required")
	}
	return nil
}

// DefaultConfig returns the default configuration for the audio-source processor.
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
		WatchEnabled: true,
		StreamName:   "GRAPH",
	}
}
