// Package videosource provides the video-source processor component for semsource.
// It ingests video file directories, extracts metadata and keyframes via ffprobe/ffmpeg,
// and publishes video entity payloads to the NATS graph ingestion stream.
package videosource

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// Config holds configuration for the video-source processor component.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// Paths is the list of filesystem directories to walk for video files.
	// At least one path is required.
	Paths []string `json:"paths" schema:"type:array,description:Filesystem directories to scan for video files (.mp4 .webm .mov .avi .mkv),category:basic,required:true"`

	// Org is the organization namespace used in entity ID construction.
	Org string `json:"org" schema:"type:string,description:Organization namespace for entity IDs (e.g. acme),category:basic,required:true"`

	// WatchEnabled controls whether fsnotify watching is active after the
	// initial ingest. When false the component exits after the initial walk.
	WatchEnabled bool `json:"watch_enabled" schema:"type:bool,description:Enable fsnotify watching for live file changes,category:basic,default:true"`

	// StreamName is the JetStream stream name for publishing entities.
	StreamName string `json:"stream_name" schema:"type:string,description:JetStream stream name,category:advanced,default:GRAPH"`

	// KeyframeMode selects the keyframe extraction strategy.
	// Valid values: "interval" (default), "scene", "iframes".
	KeyframeMode string `json:"keyframe_mode" schema:"type:string,description:Keyframe extraction strategy: interval|scene|iframes,category:advanced,default:interval"`

	// KeyframeInterval is the time between extracted frames in interval mode.
	// Accepts Go duration strings: "5s", "30s", "1m". Default is "5s".
	KeyframeInterval string `json:"keyframe_interval" schema:"type:string,description:Frame extraction interval in interval mode (e.g. 5s 30s 1m),category:advanced,default:5s"`

	// SceneThreshold is the scene-change sensitivity in scene mode.
	// Valid range: (0, 1]. Default is 0.3.
	SceneThreshold float64 `json:"scene_threshold" schema:"type:float,description:Scene-change sensitivity for scene mode (0.0-1.0),category:advanced,default:0.3"`

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
	switch c.KeyframeMode {
	case "", "interval", "scene", "iframes":
		// valid
	default:
		return fmt.Errorf("keyframe_mode must be one of: interval, scene, iframes (got %q)", c.KeyframeMode)
	}
	if c.SceneThreshold < 0 || c.SceneThreshold > 1 {
		return fmt.Errorf("scene_threshold must be in range [0, 1] (got %g)", c.SceneThreshold)
	}
	return nil
}

// DefaultConfig returns the default configuration for the video-source processor.
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
		WatchEnabled:     true,
		StreamName:       "GRAPH",
		KeyframeMode:     "interval",
		KeyframeInterval: "5s",
		SceneThreshold:   0.3,
	}
}
