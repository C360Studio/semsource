package vision

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/c360studio/semsource/handler"
	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/storage"
)

// Config holds VisionProcessor configuration.
type Config struct {
	// Enabled controls whether vision analysis runs. Default: true.
	// When false, Process returns entities unchanged without contacting the provider.
	Enabled bool

	// MaxFileSize is the maximum binary size in bytes that will be analysed.
	// Entities whose stored binary exceeds this limit are skipped without error.
	// Default: 10 MiB.
	MaxFileSize int64

	// MediaTypes lists the media_type property values that trigger analysis.
	// The check is a case-sensitive exact match against the entity's "media_type"
	// property. Default: ["image", "keyframe"].
	MediaTypes []string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:     true,
		MaxFileSize: 10 * 1024 * 1024, // 10 MiB
		MediaTypes:  []string{"image", "keyframe"},
	}
}

// Processor enriches media entities with vision analysis results.
// It is safe for concurrent use — Process and ProcessSingle may be called
// from multiple goroutines simultaneously.
type Processor struct {
	provider Provider
	store    storage.Store
	cfg      Config
	logger   *slog.Logger
}

// Option is a functional option for configuring a Processor.
type Option func(*Processor)

// WithConfig replaces the processor configuration entirely.
func WithConfig(cfg Config) Option {
	return func(p *Processor) { p.cfg = cfg }
}

// WithLogger sets a custom structured logger on the processor.
func WithLogger(l *slog.Logger) Option {
	return func(p *Processor) { p.logger = l }
}

// New creates a VisionProcessor backed by provider and store.
// Functional options are applied in order after the defaults are set.
func New(provider Provider, store storage.Store, opts ...Option) *Processor {
	p := &Processor{
		provider: provider,
		store:    store,
		cfg:      DefaultConfig(),
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.logger == nil {
		p.logger = slog.Default()
	}
	return p
}

// Process analyses a slice of RawEntities and enriches eligible media entities
// with vision triples. Non-media entities and entities that cannot be analysed
// are returned unchanged. Errors during analysis are non-fatal — the entity is
// included in the output without vision triples, and a warning is logged.
func (p *Processor) Process(ctx context.Context, entities []handler.RawEntity) []handler.RawEntity {
	if !p.cfg.Enabled {
		return entities
	}

	out := make([]handler.RawEntity, len(entities))
	for i, e := range entities {
		out[i] = p.ProcessSingle(ctx, e)
	}
	return out
}

// ProcessSingle analyses a single RawEntity. If the entity is not eligible for
// vision analysis it is returned unchanged. Errors are non-fatal and logged.
func (p *Processor) ProcessSingle(ctx context.Context, entity handler.RawEntity) handler.RawEntity {
	if !p.cfg.Enabled {
		return entity
	}

	// Gate 1: entity must have a media_type that matches the configured list.
	mediaType, ok := stringProp(entity.Properties, "media_type")
	if !ok || !p.isConfiguredMediaType(mediaType) {
		return entity
	}

	// Gate 2: entity must have a storage_ref so we can fetch the binary.
	storageRef, ok := stringProp(entity.Properties, "storage_ref")
	if !ok || storageRef == "" {
		p.logger.Debug("vision processor: skipping entity without storage_ref",
			"entity_type", entity.EntityType,
			"instance", entity.Instance,
		)
		return entity
	}

	// Fetch binary from store.
	data, err := p.store.Get(ctx, storageRef)
	if err != nil {
		p.logger.Warn("vision processor: failed to fetch binary from store",
			"storage_ref", storageRef,
			"error", err,
		)
		return entity
	}

	// Gate 3: respect MaxFileSize limit.
	if int64(len(data)) > p.cfg.MaxFileSize {
		p.logger.Warn("vision processor: skipping entity — binary exceeds MaxFileSize",
			"storage_ref", storageRef,
			"size", len(data),
			"max_file_size", p.cfg.MaxFileSize,
		)
		return entity
	}

	// Resolve the MIME type from entity properties, falling back gracefully.
	mimeType, _ := stringProp(entity.Properties, "mime_type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// Analyse.
	result, err := p.provider.Analyze(ctx, data, mimeType)
	if err != nil {
		p.logger.Warn("vision processor: provider analysis failed",
			"storage_ref", storageRef,
			"error", err,
		)
		return entity
	}

	return p.applyResult(entity, result)
}

// applyResult appends vision triples and properties to entity based on result.
// It never modifies the original entity's slice headers — a fresh copy is built.
func (p *Processor) applyResult(entity handler.RawEntity, result *Result) handler.RawEntity {
	// Encode labels as a JSON array string.
	labelsJSON, err := json.Marshal(result.Labels)
	if err != nil {
		labelsJSON = []byte(`"` + strings.Join(result.Labels, ",") + `"`)
	}

	// Encode objects as a JSON array string.
	objectsJSON, err := json.Marshal(result.Objects)
	if err != nil {
		objectsJSON = []byte("[]")
	}

	// Store vision results in Properties — normalizer converts to triples.
	if entity.Properties == nil {
		entity.Properties = make(map[string]any)
	}
	entity.Properties[source.MediaVisionLabels] = string(labelsJSON)
	entity.Properties[source.MediaVisionDescription] = result.Description
	entity.Properties[source.MediaVisionConfidence] = fmt.Sprintf("%g", result.Confidence)
	entity.Properties[source.MediaVisionObjects] = string(objectsJSON)
	entity.Properties[source.MediaVisionModel] = result.Model
	if result.Text != "" {
		entity.Properties[source.MediaVisionText] = result.Text
	}

	return entity
}

// isConfiguredMediaType reports whether mediaType is in the configured list.
func (p *Processor) isConfiguredMediaType(mediaType string) bool {
	for _, mt := range p.cfg.MediaTypes {
		if mt == mediaType {
			return true
		}
	}
	return false
}

// stringProp retrieves a string value from a Properties map.
// Returns ("", false) when the key is absent or the value is not a string.
func stringProp(props map[string]any, key string) (string, bool) {
	if props == nil {
		return "", false
	}
	v, ok := props[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
