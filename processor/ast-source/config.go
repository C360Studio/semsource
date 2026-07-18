// Package astsource provides the ast-source processor component for semsource.
// It watches source files, parses them with language-specific parsers, and
// publishes entity payloads to the NATS graph ingestion stream.
package astsource

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/component"

	semsourceast "github.com/c360studio/semsource/source/ast"
	// Import language parsers to trigger init() registration.
	_ "github.com/c360studio/semsource/source/ast/golang"
	_ "github.com/c360studio/semsource/source/ast/java"
	_ "github.com/c360studio/semsource/source/ast/python"
	_ "github.com/c360studio/semsource/source/ast/svelte"
	_ "github.com/c360studio/semsource/source/ast/ts"
)

// WatchPathConfig configures a single watch path with its parsing options.
type WatchPathConfig struct {
	// Path supports glob patterns: "./services/*", "./libs/**"
	Path string `json:"path" schema:"type:string,description:Path or glob pattern to watch,required:true"`

	// Org is the organization for entity IDs
	Org string `json:"org" schema:"type:string,description:Organization for entity IDs,required:true"`

	// Project is the project name for entity IDs
	Project string `json:"project" schema:"type:string,description:Project name for entity IDs,required:true"`

	// Version is an optional version/ref qualifier for entity-ID scoping (e.g. a
	// dependency version such as "v1.9.0"). When empty the system segment is
	// version-independent, preserving today's ID byte-for-byte. When set it is
	// slugified and appended to the system segment so that code from different
	// versions of the same project receives distinct, non-colliding entity IDs.
	Version string `json:"version,omitempty" schema:"type:string,description:Version/ref qualifier for entity-ID scoping (e.g. a dependency version); empty = version-independent"`

	// Languages to parse (registered parser names: go, typescript, javascript, java, python, svelte)
	Languages []string `json:"languages" schema:"type:array,description:Languages to parse,default:[go]"`

	// Excludes are directory names to skip
	Excludes []string `json:"excludes" schema:"type:array,description:Directory names to skip"`
}

// Validate checks the WatchPathConfig for errors.
func (w *WatchPathConfig) Validate() error {
	if w.Path == "" {
		return fmt.Errorf("path is required")
	}
	if w.Org == "" {
		return fmt.Errorf("org is required")
	}
	if w.Project == "" {
		return fmt.Errorf("project is required")
	}

	// Validate that all specified languages are registered
	for _, lang := range w.Languages {
		if !semsourceast.DefaultRegistry.HasParser(lang) {
			return fmt.Errorf("unknown language: %s (registered: %v)", lang, semsourceast.DefaultRegistry.ListParsers())
		}
	}

	return nil
}

// GetFileExtensions returns the file extensions for this watch path based on languages.
func (w *WatchPathConfig) GetFileExtensions() []string {
	var extensions []string
	seen := make(map[string]bool)

	languages := w.Languages
	if len(languages) == 0 {
		languages = []string{"go"}
	}

	for _, lang := range languages {
		for _, ext := range semsourceast.DefaultRegistry.GetExtensionsForParser(lang) {
			if !seen[ext] {
				seen[ext] = true
				extensions = append(extensions, ext)
			}
		}
	}

	return extensions
}

// Config holds configuration for the ast-source processor component.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`

	// WatchPaths defines multiple paths to watch with per-path configuration.
	WatchPaths []WatchPathConfig `json:"watch_paths" schema:"type:array,description:Watch paths with per-path configuration,category:basic"`

	// InstanceName is the unique component instance name for status tracking.
	// Set automatically by run.go to match the component map key.
	InstanceName string `json:"instance_name,omitempty" schema:"type:string,description:Unique component instance name for status tracking,category:internal"`

	// Global settings
	WatchEnabled  bool   `json:"watch_enabled"  schema:"type:bool,description:Enable file watcher for real-time updates,category:basic,default:true"`
	CoalesceMs    int    `json:"coalesce_ms,omitempty" schema:"type:int,description:Debounce window for file watcher events in ms. 0 uses built-in default (100ms),category:advanced"`
	IndexInterval string `json:"index_interval" schema:"type:string,description:Full reindex interval (e.g. 60s). Empty string disables periodic reindex.,category:advanced,default:60s"`
	StreamName    string `json:"stream_name"    schema:"type:string,description:JetStream stream name,category:advanced,default:GRAPH"`
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if len(c.WatchPaths) == 0 {
		return fmt.Errorf("watch_paths must contain at least one path")
	}
	for i, wp := range c.WatchPaths {
		if err := wp.Validate(); err != nil {
			return fmt.Errorf("watch_paths[%d]: %w", i, err)
		}
	}

	// Validate index interval if provided
	if c.IndexInterval != "" {
		d, err := time.ParseDuration(c.IndexInterval)
		if err != nil {
			return fmt.Errorf("invalid index_interval format: %w", err)
		}
		if d <= 0 {
			return fmt.Errorf("index_interval must be positive")
		}
	}

	return nil
}

// DefaultConfig returns the default configuration for the ast-source processor.
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
		WatchEnabled:  true,
		IndexInterval: "60s",
		StreamName:    "GRAPH",
	}
}
