package config

import "fmt"

// validSourceTypes lists all supported source handler types.
var validSourceTypes = map[string]bool{
	"git":    true,
	"ast":    true,
	"docs":   true,
	"config": true,
	"url":    true,
	"image":  true,
}

// SourceEntry represents a single source to be ingested.
// Fields are selectively populated based on the source Type.
type SourceEntry struct {
	// Type identifies the handler: git, ast, docs, config, url.
	Type string `json:"type"`

	// URL is the remote endpoint for git and url source types.
	URL string `json:"url,omitempty"`

	// Branch specifies the git branch to track.
	Branch string `json:"branch,omitempty"`

	// Path is the local filesystem path for ast sources.
	Path string `json:"path,omitempty"`

	// Paths is a list of filesystem paths for docs and config sources.
	Paths []string `json:"paths,omitempty"`

	// Language specifies the programming language for ast sources (e.g., "go").
	Language string `json:"language,omitempty"`

	// URLs is a list of HTTP/S URLs for url sources.
	URLs []string `json:"urls,omitempty"`

	// PollInterval is how often to re-fetch url sources (e.g., "300s").
	// Must be parseable as a Go time.Duration.
	PollInterval string `json:"poll_interval,omitempty"`

	// Watch enables continuous file-system or network watching for this source.
	Watch bool `json:"watch,omitempty"`

	// Extensions filters which file extensions to scan (e.g., ["png", "jpg"]).
	// Used by image and video sources. Empty means use handler defaults.
	Extensions []string `json:"extensions,omitempty"`

	// MaxFileSize is the maximum file size to ingest (e.g., "50MB").
	// Used by image and video sources. Empty means use handler default.
	MaxFileSize string `json:"max_file_size,omitempty"`

	// GenerateThumbnails controls whether thumbnails are generated for large images.
	// Used by image sources.
	GenerateThumbnails *bool `json:"generate_thumbnails,omitempty"`

	// ThumbnailMaxDim is the maximum dimension (width or height) for generated thumbnails.
	// Used by image sources. Default: 512.
	ThumbnailMaxDim int `json:"thumbnail_max_dim,omitempty"`
}

// Validate checks that the SourceEntry has the required fields for its type.
func (s SourceEntry) Validate() error {
	if !validSourceTypes[s.Type] {
		return fmt.Errorf("source: unknown type %q (valid: git, ast, docs, config, url, image)", s.Type)
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
	case "image":
		if len(s.Paths) == 0 {
			return fmt.Errorf("source type %q: at least one path is required", s.Type)
		}
		for _, ext := range s.Extensions {
			if ext == "" {
				return fmt.Errorf("source type %q: extensions must not contain empty strings", s.Type)
			}
		}
	}

	return nil
}
