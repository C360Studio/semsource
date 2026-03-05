package config

import "fmt"

// validSourceTypes lists all supported source handler types.
var validSourceTypes = map[string]bool{
	"git":    true,
	"ast":    true,
	"docs":   true,
	"config": true,
	"url":    true,
}

// SourceEntry represents a single source to be ingested.
// Fields are selectively populated based on the source Type.
type SourceEntry struct {
	// Type identifies the handler: git, ast, docs, config, url.
	Type string `yaml:"type"`

	// URL is the remote endpoint for git and url source types.
	URL string `yaml:"url,omitempty"`

	// Branch specifies the git branch to track.
	Branch string `yaml:"branch,omitempty"`

	// Path is the local filesystem path for ast sources.
	Path string `yaml:"path,omitempty"`

	// Paths is a list of filesystem paths for docs and config sources.
	Paths []string `yaml:"paths,omitempty"`

	// Language specifies the programming language for ast sources (e.g., "go").
	Language string `yaml:"language,omitempty"`

	// URLs is a list of HTTP/S URLs for url sources.
	URLs []string `yaml:"urls,omitempty"`

	// PollInterval is how often to re-fetch url sources (e.g., "300s").
	// Must be parseable as a Go time.Duration.
	PollInterval string `yaml:"poll_interval,omitempty"`

	// Watch enables continuous file-system or network watching for this source.
	Watch bool `yaml:"watch,omitempty"`
}

// Validate checks that the SourceEntry has the required fields for its type.
func (s SourceEntry) Validate() error {
	if !validSourceTypes[s.Type] {
		return fmt.Errorf("source: unknown type %q (valid: git, ast, docs, config, url)", s.Type)
	}

	switch s.Type {
	case "git":
		if s.URL == "" {
			return fmt.Errorf("source type %q: url is required", s.Type)
		}
	case "ast":
		if s.Path == "" {
			return fmt.Errorf("source type %q: path is required", s.Type)
		}
	case "docs", "config":
		if len(s.Paths) == 0 {
			return fmt.Errorf("source type %q: at least one path is required", s.Type)
		}
	case "url":
		if len(s.URLs) == 0 {
			return fmt.Errorf("source type %q: at least one url is required", s.Type)
		}
	}

	return nil
}
