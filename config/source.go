package config

import (
	"fmt"
	"math"
	"time"
)

// validSourceTypes lists all supported source handler types.
var validSourceTypes = map[string]bool{
	"git":    true,
	"repo":   true,
	"ast":    true,
	"docs":   true,
	"config": true,
	"url":    true,
	"image":  true,
	"video":  true,
	"audio":  true,
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
	// Superseded by Languages when both are set; kept for back-compat.
	Language string `json:"language,omitempty"`

	// Languages lists the programming languages for ast (and expanded repo)
	// sources — registered parser names: go, typescript, javascript, java,
	// python, svelte. When set it wins over the singular Language, whose value
	// (if also set) must appear in the list (compose-packaging-hardening D4: a
	// polyglot workspace must not get a silently partial graph).
	Languages []string `json:"languages,omitempty"`

	// Version is an optional explicit version for ast (and expanded repo)
	// sources. Non-empty, it flows to the ast component's version-scoped entity
	// IDs and code.artifact.version triples — the input the supersession pass
	// and code_changes relate (version-registration-surface D1). Empty keeps
	// version-independent IDs byte-for-byte. Never defaulted from a ref: branch
	// identity already scopes IDs, and an implicit version would re-key
	// existing multi-branch deployments.
	Version string `json:"version,omitempty"`

	// Project optionally overrides the path-derived project identity for ast
	// (and expanded repo) sources. Supersession corresponds entities BY
	// project, so registering two versions of one dependency at two paths
	// requires declaring the shared project explicitly — path slugs differ per
	// version directory (D1 amendment). Slugified for ID-safety; empty keeps
	// the path-derived project byte-for-byte.
	Project string `json:"project,omitempty"`

	// URLs is a list of HTTP/S URLs for url sources.
	URLs []string `json:"urls,omitempty"`

	// PollInterval controls how often poll-based sources check their origin
	// for changes. Must be parseable as a Go time.Duration.
	// Applies to: git (default "60s"), url (default "300s").
	// Ignored by fsnotify-based sources (docs, config, ast).
	PollInterval string `json:"poll_interval,omitempty"`

	// IndexInterval controls how often ast sources perform a full reindex
	// sweep on top of fsnotify. Must be parseable as a Go time.Duration.
	// Default "60s". Empty string or "0s" disables periodic reindex.
	// Only applicable to "ast" source type.
	IndexInterval string `json:"index_interval,omitempty"`

	// Branches is a list of branch name patterns to track in multi-branch mode.
	// Supports glob patterns: ["*"], ["main", "scenario/*"].
	// When empty, single-branch mode is used (tracks Branch field only).
	// Only applicable to "repo" and "git" source types.
	Branches []string `json:"branches,omitempty"`

	// BranchPollInterval is how often to check for new branches.
	// Must be parseable as a Go time.Duration (e.g., "15s"). Default: "15s".
	// Only used when Branches is set.
	BranchPollInterval string `json:"branch_poll_interval,omitempty"`

	// MaxBranches is the safety cap on concurrent branch tracking.
	// Default: 50. Only used when Branches is set.
	MaxBranches int `json:"max_branches,omitempty"`

	// BranchSlug is set internally by multi-branch expansion to scope
	// entity IDs per branch. Not user-configurable.
	BranchSlug string `json:"-"`

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

	// KeyframeMode controls how keyframes are extracted from video sources.
	// Valid values: "interval", "scene", "iframes". Default: "interval".
	KeyframeMode string `json:"keyframe_mode,omitempty"`

	// KeyframeInterval is the time between keyframe extractions for interval mode.
	// Must be parseable as a Go time.Duration (e.g., "30s"). Used by video sources.
	KeyframeInterval string `json:"keyframe_interval,omitempty"`

	// SceneThreshold is the scene-change sensitivity for scene-based keyframe extraction.
	// Range: 0.0 (detect every frame) to 1.0 (detect only major scene changes).
	// Used by video sources in scene mode.
	SceneThreshold float64 `json:"scene_threshold,omitempty"`
}

// EffectiveLanguages resolves the plural/singular language fields into the
// language list a spawned ast component should parse: Languages when set,
// else the singular Language, else nil (the spawner applies its own default).
func (s SourceEntry) EffectiveLanguages() []string {
	if len(s.Languages) > 0 {
		return s.Languages
	}
	if s.Language != "" {
		return []string{s.Language}
	}
	return nil
}

// validateLanguages enforces plural/singular consistency: entries must be
// non-empty, and a singular Language set alongside Languages must appear in
// the list (a contradiction would silently drop the singular's intent).
func (s SourceEntry) validateLanguages() error {
	for _, lang := range s.Languages {
		if lang == "" {
			return fmt.Errorf("source type %q: languages must not contain empty strings", s.Type)
		}
	}
	if s.Language == "" || len(s.Languages) == 0 {
		return nil
	}
	for _, lang := range s.Languages {
		if lang == s.Language {
			return nil
		}
	}
	return fmt.Errorf("source type %q: language %q is not in languages %v (set one or make them consistent)", s.Type, s.Language, s.Languages)
}

// validatePositiveDuration checks that an optional duration-string field is
// either empty (use default) or a positive Go duration. Zero-valued durations
// would disable the associated ticker entirely, which is almost never what a
// user intends when they set the field.
func validatePositiveDuration(field, value string) error {
	if value == "" {
		return nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("source: invalid %s %q: %w", field, value, err)
	}
	if d <= 0 {
		return fmt.Errorf("source: %s must be positive, got %q", field, value)
	}
	return nil
}

// Validate checks that the SourceEntry has the required fields for its type.
func (s SourceEntry) Validate() error {
	if !validSourceTypes[s.Type] {
		return fmt.Errorf("source: unknown type %q (valid: git, repo, ast, docs, config, url, image, video, audio)", s.Type)
	}

	if err := validatePositiveDuration("poll_interval", s.PollInterval); err != nil {
		return err
	}
	if err := validatePositiveDuration("index_interval", s.IndexInterval); err != nil {
		return err
	}

	switch s.Type {
	case "git":
		// Accept a remote url OR a local path. A path-only git source is read in
		// place (handler resolveRepoPath prefers Path; sourcespawn passes it as
		// repo_path) — required to index a mounted/agent worktree without cloning
		// (issue #1 / ADR-0007 sidecar). Mirrors the "repo" rule below.
		if s.URL == "" && s.Path == "" {
			return fmt.Errorf("source type %q: url or path is required", s.Type)
		}
	case "repo":
		if s.URL == "" && s.Path == "" {
			return fmt.Errorf("source type %q: url or path is required", s.Type)
		}
		if len(s.Branches) > 0 && s.Branch != "" {
			return fmt.Errorf("source type %q: cannot set both branch and branches", s.Type)
		}
		if err := s.validateLanguages(); err != nil {
			return err
		}
	case "ast":
		if s.Path == "" {
			return fmt.Errorf("source type %q: path is required", s.Type)
		}
		if err := s.validateLanguages(); err != nil {
			return err
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
	case "video":
		if len(s.Paths) == 0 {
			return fmt.Errorf("source type %q: at least one path is required", s.Type)
		}
		if s.KeyframeMode != "" && s.KeyframeMode != "interval" && s.KeyframeMode != "scene" && s.KeyframeMode != "iframes" {
			return fmt.Errorf("source type %q: keyframe_mode must be interval, scene, or iframes", s.Type)
		}
		if math.IsNaN(s.SceneThreshold) || math.IsInf(s.SceneThreshold, 0) {
			return fmt.Errorf("source type %q: scene_threshold must be a finite number", s.Type)
		}
		if s.SceneThreshold < 0 || s.SceneThreshold > 1 {
			return fmt.Errorf("source type %q: scene_threshold must be between 0 and 1", s.Type)
		}
	case "audio":
		if len(s.Paths) == 0 {
			return fmt.Errorf("source type %q: at least one path is required", s.Type)
		}
	}

	return nil
}
