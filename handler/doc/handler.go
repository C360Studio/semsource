// Package doc implements the doc Handler for markdown and plain-text document sources.
package doc

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
)

// docExtensions lists the file extensions Handler will process.
var docExtensions = map[string]bool{
	".md":  true,
	".mdx": true,
	".txt": true,
}

// Handler handles document sources (markdown, plain text).
// It implements handler.SourceHandler.
type Handler struct {
	// org is the organisation namespace used when building typed EntityState
	// values via IngestEntityStates and enrichEventEntityStates. When empty,
	// EntityStates are not populated on watch events.
	org string
}

// New returns a ready-to-use Handler.
func New() *Handler {
	return &Handler{}
}

// NewWithOrg returns a Handler that will populate EntityStates on watch
// events using the given org namespace.
func NewWithOrg(org string) *Handler {
	return &Handler{org: org}
}

// sourceTypeKey is the config source type key for doc sources.
// The config uses "docs" (plural) while the handler.SourceTypeDoc constant
// is "doc" (singular) — used for RawEntity.SourceType on emitted entities.
const sourceTypeKey = "docs"

// SourceType returns the handler type identifier as used in semsource.yaml.
func (h *Handler) SourceType() string {
	return sourceTypeKey
}

// Supports returns true when cfg describes a "docs" source.
func (h *Handler) Supports(cfg handler.SourceConfig) bool {
	return cfg.GetType() == sourceTypeKey
}

// resolvePaths returns the set of root paths to process for cfg.
// If GetPaths() is non-empty those are used; otherwise GetPath() is the
// single-element fallback so that existing single-path configs keep working.
// Returns an error when both sources are empty.
func resolvePaths(cfg handler.SourceConfig) ([]string, error) {
	if paths := cfg.GetPaths(); len(paths) > 0 {
		return paths, nil
	}
	p := cfg.GetPath()
	if p == "" {
		return nil, fmt.Errorf("doc handler: no paths configured (GetPaths is empty and GetPath is empty)")
	}
	return []string{p}, nil
}

// Ingest walks all configured path(s) in cfg, reads each supported document
// file, and returns a RawEntity per file. It respects ctx cancellation.
func (h *Handler) Ingest(ctx context.Context, cfg handler.SourceConfig) ([]handler.RawEntity, error) {
	roots, err := resolvePaths(cfg)
	if err != nil {
		return nil, err
	}

	var entities []handler.RawEntity

	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		walkErr := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
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
			if !docExtensions[ext] {
				return nil
			}

			entity, err := ingestFile(path, root)
			if err != nil {
				// Non-fatal: log and continue.
				return nil
			}
			entities = append(entities, entity)
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("doc handler: walk %q: %w", root, walkErr)
		}
	}

	return entities, nil
}

// ingestFile reads a single document file and constructs its RawEntity.
func ingestFile(path, root string) (handler.RawEntity, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return handler.RawEntity{}, fmt.Errorf("read %q: %w", path, err)
	}

	hash := contentHash(content)
	instance := hash[:6]

	relPath, _ := filepath.Rel(root, path)
	title := extractTitle(content, filepath.Base(path))
	mimeType := mimeForExt(filepath.Ext(path))

	return handler.RawEntity{
		SourceType: handler.SourceTypeDoc,
		Domain:     handler.DomainWeb,
		// System is the relative directory path of the doc root, slugified.
		// Left to the caller (Normalizer) to further qualify; we set it to
		// the root path base so the normalizer has something to work with.
		System:     entityid.SystemSlug(root),
		EntityType: "doc",
		Instance:   instance,
		Properties: map[string]any{
			"title":        title,
			"file_path":    relPath,
			"mime_type":    mimeType,
			"content_hash": hash,
			"content":      string(content),
		},
	}, nil
}

// extractTitle returns the text of the first markdown H1 heading in content,
// falling back to the filename (without extension) if none is found.
func extractTitle(content []byte, filename string) string {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	// Fallback: strip extension from filename.
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}

// contentHash returns the hex-encoded SHA-256 of b.
func contentHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// mimeForExt returns the MIME type for known document extensions.
func mimeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".md", ".mdx":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

// slugify converts a filesystem path into a slug safe for use in entity IDs:
// slashes become hyphens.
func slugify(path string) string {
	s := filepath.ToSlash(path)
	s = strings.TrimPrefix(s, "/")
	return strings.ReplaceAll(s, "/", "-")
}
