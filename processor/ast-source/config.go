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
	// Takes precedence over legacy single-path fields (RepoPath, Org, Project, Languages, ExcludePatterns).
	WatchPaths []WatchPathConfig `json:"watch_paths" schema:"type:array,description:Watch paths with per-path configuration,category:basic"`

	// Legacy single-path configuration (deprecated, use WatchPaths instead)
	RepoPath        string   `json:"repo_path"        schema:"type:string,description:Repository path to index (deprecated: use watch_paths),category:basic,default:."`
	Org             string   `json:"org"              schema:"type:string,description:Organization for entity IDs (deprecated: use watch_paths),category:basic"`
	Project         string   `json:"project"          schema:"type:string,description:Project name for entity IDs (deprecated: use watch_paths),category:basic"`
	Languages       []string `json:"languages"        schema:"type:array,description:Languages to index (deprecated: use watch_paths),category:basic,default:[go]"`
	ExcludePatterns []string `json:"exclude_patterns" schema:"type:array,description:Directory patterns to exclude (deprecated: use watch_paths),category:advanced"`

	// Global settings
	WatchEnabled  bool   `json:"watch_enabled"  schema:"type:bool,description:Enable file watcher for real-time updates,category:basic,default:true"`
	IndexInterval string `json:"index_interval" schema:"type:string,description:Full reindex interval (e.g. 60s). Empty string disables periodic reindex.,category:advanced,default:60s"`
	StreamName    string `json:"stream_name"    schema:"type:string,description:JetStream stream name,category:advanced,default:GRAPH"`
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	// Normalize to WatchPathConfig for uniform validation (handles legacy too).
	watchPaths := c.GetWatchPaths()
	for i, wp := range watchPaths {
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

// GetWatchPaths returns the effective watch paths.
// If WatchPaths is configured, returns it directly.
// Otherwise, converts legacy single-path config to WatchPathConfig.
func (c *Config) GetWatchPaths() []WatchPathConfig {
	if len(c.WatchPaths) > 0 {
		return c.WatchPaths
	}

	// Convert legacy config to WatchPathConfig
	languages := c.Languages
	if len(languages) == 0 {
		languages = []string{"go"}
	}

	return []WatchPathConfig{
		{
			Path:      c.RepoPath,
			Org:       c.Org,
			Project:   c.Project,
			Languages: languages,
			Excludes:  c.ExcludePatterns,
		},
	}
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
		RepoPath:        ".",
		Languages:       []string{"go"},
		WatchEnabled:    true,
		IndexInterval:   "60s",
		StreamName:      "GRAPH",
		ExcludePatterns: []string{"vendor", "node_modules", "dist", ".next", "build", "coverage"},
	}
}
