// Package doc implements the doc Handler for markdown and plain-text document sources.
package doc

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semstreams/storage"
)

// Handler handles document sources (markdown, plain text).
// It implements handler.SourceHandler.
type Handler struct {
	// org is the organisation namespace used when building typed EntityState
	// values via IngestEntityStates and enrichEventEntityStates. When empty,
	// EntityStates are not populated on watch events.
	org string

	// bodyStore / bodyInstance are the fusion verbatim-body store (ADR-062). Every
	// doc's passage is offloaded here (content-addressed) and wired to a single
	// CONTENT blob two ways: DocBodyStore/DocBodyKey triples (the fusion docs lens
	// hydrates by handle) and EntityState.StorageRef (graph-embedding fetches the
	// body via the shared StoreRegistry — ADR-063). When nil, content stays inline.
	bodyStore    storage.Store
	bodyInstance string
}

// Option is a functional option for configuring a Handler.
type Option func(*Handler)

// WithBodyStore sets the verbatim-body store for fusion hydration (ADR-062).
// When set, every document's passage is offloaded and the entity carries
// DocBodyStore/DocBodyKey triples plus a StorageRef so the fusion docs lens
// hydrates by handle and graph-embedding embeds the offloaded body. instance is
// the StorageReference.StorageInstance the resolver maps back to this store (the
// storage component instance name, e.g. "objectstore").
func WithBodyStore(s storage.Store, instance string) Option {
	return func(h *Handler) {
		h.bodyStore = s
		h.bodyInstance = instance
	}
}

// New returns a ready-to-use Handler.
func New(opts ...Option) *Handler {
	h := &Handler{}
	for _, o := range opts {
		o(h)
	}
	return h
}

// NewWithOrg returns a Handler that will populate EntityStates on watch
// events using the given org namespace.
func NewWithOrg(org string, opts ...Option) *Handler {
	h := New(opts...)
	h.org = org
	return h
}

// sourceTypeKey is the config source type key for doc sources.
// The config uses "docs" (plural); the entity ID's type segment is "doc"
// (singular), built in entities.go.
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
	case ".adoc":
		return "text/asciidoc"
	case ".md", ".mdx":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

// isDefaultExcludedDocDir reports directories the docs corpus skips by
// default: OpenSpec planning artifacts and node_modules. Planning docs
// (proposals, deltas, archives — and the graded re-run showed even ACTIVE
// change proposals) outrank canonical docs like the README for product
// questions; they serve the dev loop, not the product doc corpus
// (search-ranking-and-reach D3). docs/adr and docs/** stay indexed; a
// deployment that wants specs indexed can add an explicit docs source.
func isDefaultExcludedDocDir(root, path string) bool {
	if handler.IsDefaultExcludedDir(filepath.Base(path)) {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return filepath.ToSlash(rel) == "openspec"
}
