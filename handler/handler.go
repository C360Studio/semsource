// Package handler defines the SourceHandler interface and supporting types
// for the SemSource ingestion pipeline.
//
// All source handlers implement SourceHandler. The engine uses Supports to
// route each configured source to the correct handler, then calls Ingest for
// initial seeding and Watch for real-time change tracking.
package handler

import "context"

// SourceConfig is the minimal interface a source configuration must satisfy
// for handler dispatch. Concrete implementations live in the config package —
// this interface keeps the handler package free of config dependencies.
type SourceConfig interface {
	// GetType returns the source type string (git, ast, doc, config, url, image, video).
	GetType() string

	// GetPath returns the primary filesystem path for the source, if applicable.
	// Returns empty string for URL-based sources.
	GetPath() string

	// GetPaths returns all configured filesystem paths for multi-path sources
	// (docs, config, image, video). Returns nil for single-path sources.
	GetPaths() []string

	// GetURL returns the primary URL for the source, if applicable.
	// Returns empty string for filesystem-based sources.
	GetURL() string

	// GetBranch returns the git branch to track.
	// Returns empty string when not configured (use remote default).
	GetBranch() string

	// IsWatchEnabled reports whether real-time watching is enabled for this source.
	IsWatchEnabled() bool

	// GetKeyframeMode returns the keyframe extraction mode for video sources.
	// Valid values: "interval", "scene", "iframes". Returns "" for non-video sources.
	GetKeyframeMode() string

	// GetKeyframeInterval returns the keyframe extraction interval for video sources.
	// Returns "" when not configured (handler uses its own default).
	GetKeyframeInterval() string

	// GetSceneThreshold returns the scene-change sensitivity for video sources
	// using scene mode. Returns 0 when not configured.
	GetSceneThreshold() float64
}

// CoalesceProvider is an optional interface that SourceConfig implementations
// may satisfy to provide a custom coalescing/debounce window for file watchers.
// When not implemented, handlers use their built-in defaults.
type CoalesceProvider interface {
	GetCoalesceMs() int
}

// SourceHandler is the core interface implemented by every source handler.
// Each handler is responsible for one category of source material.
//
// Implementations must be safe for concurrent use — the engine may call
// Ingest and Watch from different goroutines.
type SourceHandler interface {
	// SourceType returns the handler identifier string (git, ast, doc, config, url).
	// Must match the type values used in SourceConfig.GetType().
	SourceType() string

	// Ingest processes the source described by cfg and returns all raw entities
	// extracted from it, before ID normalization. Called on initial seed and
	// on explicit refresh. Implementations must respect ctx cancellation.
	Ingest(ctx context.Context, cfg SourceConfig) ([]RawEntity, error)

	// Watch returns a channel of ChangeEvents for real-time source monitoring.
	// The channel is closed when ctx is cancelled or the watch is otherwise
	// terminated. Returns nil, nil if this handler does not support watching.
	// Implementations must start the watcher before returning the channel.
	Watch(ctx context.Context, cfg SourceConfig) (<-chan ChangeEvent, error)

	// Supports returns true if this handler can process the given config.
	// Used by the engine for handler dispatch — a handler should return true
	// only when it is certain it can handle the source without error.
	Supports(cfg SourceConfig) bool
}
