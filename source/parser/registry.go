package parser

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/c360studio/semsource/source"
)

// Parser defines the interface for document parsers.
type Parser interface {
	// Parse parses a document and returns structured data.
	Parse(filename string, content []byte) (*source.Document, error)

	// CanParse returns true if this parser handles the given MIME type.
	CanParse(mimeType string) bool

	// MimeType returns the primary MIME type for this parser.
	MimeType() string
}

// Registry manages document parsers.
type Registry struct {
	mu      sync.RWMutex
	parsers map[string]Parser // keyed by primary MIME type
}

// DefaultRegistry is the global parser registry with default parsers.
var DefaultRegistry = NewRegistry()

// NewRegistry creates a new parser registry with default parsers.
func NewRegistry() *Registry {
	r := &Registry{
		parsers: make(map[string]Parser),
	}

	// Register default parsers
	r.Register(NewMarkdownParser())
	r.Register(NewPDFParser())
	r.Register(NewRSTParser())
	r.Register(NewASCIIDocParser())
	r.Register(NewOpenSpecParser())

	return r
}

// Register adds a parser to the registry.
func (r *Registry) Register(p Parser) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.parsers[p.MimeType()] = p
}

// GetByMimeType returns a parser for the given MIME type.
func (r *Registry) GetByMimeType(mimeType string) Parser {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Direct match
	if p, ok := r.parsers[mimeType]; ok {
		return p
	}

	// Check if any parser can handle this type
	for _, p := range r.parsers {
		if p.CanParse(mimeType) {
			return p
		}
	}

	return nil
}

// GetByExtension returns a parser for a file based on its extension.
// It first checks for OpenSpec files based on path patterns.
func (r *Registry) GetByExtension(filename string) Parser {
	// Check for OpenSpec files first (path-based detection)
	if IsOpenSpecFile(filename) {
		return r.GetByMimeType("text/x-openspec")
	}

	mimeType := MimeTypeFromExtension(filepath.Ext(filename))
	return r.GetByMimeType(mimeType)
}

// Parse parses a document using the appropriate parser.
func (r *Registry) Parse(filename string, content []byte) (*source.Document, error) {
	parser := r.GetByExtension(filename)
	if parser == nil {
		return nil, fmt.Errorf("no parser for file type: %s", filepath.Ext(filename))
	}
	return parser.Parse(filename, content)
}

// ListMimeTypes returns all registered MIME types.
func (r *Registry) ListMimeTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.parsers))
	for t := range r.parsers {
		types = append(types, t)
	}
	return types
}

// MimeTypeFromExtension returns the MIME type for a file extension.
// Note: OpenSpec files (.spec.md or files in openspec/ directories) are
// detected separately in GetByExtension using IsOpenSpecFile.
func MimeTypeFromExtension(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	case ".md", ".markdown":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".pdf":
		return "application/pdf"
	case ".rst":
		return "text/x-rst"
	case ".adoc", ".asciidoc", ".asc":
		return "text/asciidoc"
	default:
		return "application/octet-stream"
	}
}

// FormatFromExtension returns a short format name for a file extension.
// Used to generate the "type" part of 6-part entity IDs.
func FormatFromExtension(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	case ".md", ".markdown":
		return "markdown"
	case ".txt":
		return "text"
	case ".html", ".htm":
		return "html"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".pdf":
		return "pdf"
	case ".rst":
		return "rst"
	case ".adoc", ".asciidoc", ".asc":
		return "asciidoc"
	default:
		return "unknown"
	}
}

// ExtensionFromMimeType returns a typical file extension for a MIME type.
func ExtensionFromMimeType(mimeType string) string {
	switch mimeType {
	case "text/markdown", "text/x-markdown":
		return ".md"
	case "text/plain":
		return ".txt"
	case "text/html":
		return ".html"
	case "application/json":
		return ".json"
	case "application/yaml":
		return ".yaml"
	case "application/pdf":
		return ".pdf"
	case "text/x-rst", "text/rst":
		return ".rst"
	case "text/asciidoc", "text/x-asciidoc":
		return ".adoc"
	case "text/x-openspec":
		return ".spec.md"
	default:
		return ""
	}
}
