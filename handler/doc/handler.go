// Package doc implements the DocHandler for markdown and plain-text document sources.
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
	"time"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semstreams/message"
)

// docExtensions lists the file extensions DocHandler will process.
var docExtensions = map[string]bool{
	".md":  true,
	".txt": true,
}

// DocHandler handles document sources (markdown, plain text).
// It implements handler.SourceHandler.
type DocHandler struct{}

// New returns a ready-to-use DocHandler.
func New() *DocHandler {
	return &DocHandler{}
}

// sourceTypeKey is the config source type key for doc sources.
// The config uses "docs" (plural) while the handler.SourceTypeDoc constant
// is "doc" (singular) — used for RawEntity.SourceType on emitted entities.
const sourceTypeKey = "docs"

// SourceType returns the handler type identifier as used in semsource.yaml.
func (h *DocHandler) SourceType() string {
	return sourceTypeKey
}

// Supports returns true when cfg describes a "docs" source.
func (h *DocHandler) Supports(cfg handler.SourceConfig) bool {
	return cfg.GetType() == sourceTypeKey
}

// Ingest walks the path(s) in cfg, reads each supported document file, and
// returns a RawEntity per file. It respects ctx cancellation.
func (h *DocHandler) Ingest(ctx context.Context, cfg handler.SourceConfig) ([]handler.RawEntity, error) {
	root := cfg.GetPath()
	if root == "" {
		return nil, fmt.Errorf("doc handler: GetPath() returned empty string")
	}

	var entities []handler.RawEntity

	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
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
	if err != nil {
		return nil, fmt.Errorf("doc handler: walk %q: %w", root, err)
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

	now := time.Now().UTC()

	triples := []message.Triple{
		{
			Subject:    "", // filled by normalizer after ID assignment
			Predicate:  handler.DomainWeb + ".doc.title",
			Object:     title,
			Source:     handler.SourceTypeDoc,
			Timestamp:  now,
			Confidence: 1.0,
		},
		{
			Subject:    "",
			Predicate:  handler.DomainWeb + ".doc.file_path",
			Object:     relPath,
			Source:     handler.SourceTypeDoc,
			Timestamp:  now,
			Confidence: 1.0,
		},
		{
			Subject:    "",
			Predicate:  handler.DomainWeb + ".doc.mime_type",
			Object:     mimeType,
			Source:     handler.SourceTypeDoc,
			Timestamp:  now,
			Confidence: 1.0,
		},
		{
			Subject:    "",
			Predicate:  handler.DomainWeb + ".doc.content_hash",
			Object:     hash,
			Source:     handler.SourceTypeDoc,
			Timestamp:  now,
			Confidence: 1.0,
		},
		{
			Subject:    "",
			Predicate:  handler.DomainWeb + ".doc.content",
			Object:     string(content),
			Source:     handler.SourceTypeDoc,
			Timestamp:  now,
			Confidence: 1.0,
		},
	}

	return handler.RawEntity{
		SourceType: handler.SourceTypeDoc,
		Domain:     handler.DomainWeb,
		// System is the relative directory path of the doc root, slugified.
		// Left to the caller (Normalizer) to further qualify; we set it to
		// the root path base so the normalizer has something to work with.
		System:     slugify(root),
		EntityType: "doc",
		Instance:   instance,
		Properties: map[string]any{
			"title":        title,
			"file_path":    relPath,
			"mime_type":    mimeType,
			"content_hash": hash,
		},
		Triples: triples,
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
	case ".md":
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
