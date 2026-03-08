// Package image implements the ImageHandler for image file sources.
package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semstreams/storage"
)

// sourceTypeKey is the config source type key for image sources.
const sourceTypeKey = "image"

// defaultExtensions lists the file extensions ImageHandler will process.
var defaultExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".svg":  true,
}

// ImageHandler handles image file sources.
// It implements handler.SourceHandler.
type ImageHandler struct {
	store  storage.Store // nil = no binary storage (metadata only)
	logger *slog.Logger
}

// Option is a functional option for configuring an ImageHandler.
type Option func(*ImageHandler)

// WithStore sets the binary storage backend. When nil (the default), the
// handler records metadata triples only and skips binary storage.
func WithStore(s storage.Store) Option {
	return func(h *ImageHandler) { h.store = s }
}

// WithLogger sets a custom structured logger on the handler.
func WithLogger(l *slog.Logger) Option {
	return func(h *ImageHandler) { h.logger = l }
}

// New returns a ready-to-use ImageHandler configured by the provided options.
// Calling New() with no options is equivalent to the former New() behaviour:
// metadata-only mode with the default slog logger.
func New(opts ...Option) *ImageHandler {
	h := &ImageHandler{}
	for _, opt := range opts {
		opt(h)
	}
	if h.logger == nil {
		h.logger = slog.Default()
	}
	return h
}

// SourceType returns the handler type identifier as used in semsource.json.
func (h *ImageHandler) SourceType() string {
	return sourceTypeKey
}

// Supports returns true when cfg describes an "image" source.
func (h *ImageHandler) Supports(cfg handler.SourceConfig) bool {
	return cfg.GetType() == sourceTypeKey
}

// resolvePaths returns the effective set of root paths to process for cfg.
// When GetPaths() is non-empty it is used directly; otherwise the single
// GetPath() value is returned as a one-element slice. An error is returned
// when both are empty so callers receive a clear diagnostic.
func resolvePaths(cfg handler.SourceConfig) ([]string, error) {
	if paths := cfg.GetPaths(); len(paths) > 0 {
		return paths, nil
	}
	p := cfg.GetPath()
	if p == "" {
		return nil, fmt.Errorf("image handler: no paths configured (GetPaths is empty and GetPath is empty)")
	}
	return []string{p}, nil
}

// Ingest walks every path in cfg, reads each supported image file, and
// returns a RawEntity per file. It respects ctx cancellation.
func (h *ImageHandler) Ingest(ctx context.Context, cfg handler.SourceConfig) ([]handler.RawEntity, error) {
	roots, err := resolvePaths(cfg)
	if err != nil {
		return nil, err
	}

	var entities []handler.RawEntity

	for _, root := range roots {
		if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if !defaultExtensions[ext] {
				return nil
			}

			entity, err := h.ingestFile(ctx, path, root)
			if err != nil {
				// Non-fatal: skip unreadable or malformed files and continue.
				return nil
			}
			entities = append(entities, entity)
			return nil
		}); err != nil {
			return nil, fmt.Errorf("image handler: walk %q: %w", root, err)
		}
	}

	return entities, nil
}

// ingestFile reads a single image file and constructs its RawEntity.
// When the handler has a store configured, it also persists the binary content
// and attempts to generate and store a thumbnail.
func (h *ImageHandler) ingestFile(ctx context.Context, path, root string) (handler.RawEntity, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return handler.RawEntity{}, fmt.Errorf("read %q: %w", path, err)
	}

	hash := contentHash(content)
	instance := hash[:6]

	relPath, _ := filepath.Rel(root, path)
	ext := filepath.Ext(path)
	mimeType := mimeForExt(ext)
	format := formatForExt(ext)
	fileSize := int64(len(content))

	width, height := imageDimensions(path)

	entity := handler.RawEntity{
		SourceType: handler.SourceTypeImage,
		Domain:     handler.DomainMedia,
		System:     slugify(root),
		EntityType: "image",
		Instance:   instance,
		Properties: map[string]any{
			"media_type": "image",
			"file_path":  relPath,
			"mime_type":  mimeType,
			"file_hash":  hash,
			"width":      width,
			"height":     height,
			"file_size":  fileSize,
			"format":     format,
		},
	}

	// When a store is configured, persist the binary content and attempt
	// thumbnail generation. Both operations are non-fatal: failures are logged
	// and the entity is still returned with its metadata properties intact.
	if h.store != nil {
		storageKey := fmt.Sprintf("images/%s/%s/original", slugify(root), instance)
		if err := h.store.Put(ctx, storageKey, content); err != nil {
			h.logger.Warn("failed to store image binary", "path", path, "error", err)
		} else {
			entity.Properties["storage_ref"] = storageKey

			thumbKey, err := h.generateAndStoreThumbnail(ctx, content, path, root, instance)
			if err == nil && thumbKey != "" {
				entity.Properties["thumbnail_ref"] = thumbKey
			}
		}
	}

	return entity, nil
}

// imageDimensions returns the pixel width and height of the image at path.
// For SVG and WebP files, which require special decoders not in the standard
// library, it returns (0, 0). Returns (0, 0) on any decode error.
func imageDimensions(path string) (width, height int) {
	ext := strings.ToLower(filepath.Ext(path))
	// SVG is XML-based; WebP is not supported by the standard library decoders.
	if ext == ".svg" || ext == ".webp" {
		return 0, 0
	}

	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// contentHash returns the hex-encoded SHA-256 of b.
func contentHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// mimeForExt returns the MIME type for known image extensions.
func mimeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

// formatForExt returns the format name for known image extensions.
func formatForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "jpeg"
	default:
		return strings.TrimPrefix(strings.ToLower(ext), ".")
	}
}

// slugify converts a filesystem path into a slug safe for use in entity IDs:
// slashes become hyphens.
func slugify(path string) string {
	s := filepath.ToSlash(path)
	s = strings.TrimPrefix(s, "/")
	return strings.ReplaceAll(s, "/", "-")
}

