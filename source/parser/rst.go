package parser

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/c360studio/semsource/source"
)

// reStructuredText patterns
var (
	// Section underlines: ===, ---, ~~~, ^^^, etc.
	rstSectionUnderline = regexp.MustCompile(`^(={3,}|-{3,}|~{3,}|\^{3,}|\+{3,}|#{3,}|\*{3,}|_{3,})$`)

	// Code blocks: .. code-block:: or :: followed by indented content
	rstCodeBlockDirective = regexp.MustCompile(`^\.\. code(?:-block)?::`)
	rstCodeBlockShort     = regexp.MustCompile(`::$`)

	// Field list: :field-name: value
	rstFieldList = regexp.MustCompile(`^:([^:]+):(.*)$`)

	// Directive: .. directive-name::
	rstDirective = regexp.MustCompile(`^\.\. ([a-z-]+)::`)
)

// RSTParser parses reStructuredText documents.
type RSTParser struct{}

// NewRSTParser creates a new RST parser.
func NewRSTParser() *RSTParser {
	return &RSTParser{}
}

// Parse parses an RST document.
func (p *RSTParser) Parse(filename string, content []byte) (*source.Document, error) {
	str := string(content)

	// Convert RST to a markdown-like format for compatibility with the chunker
	converted := p.convertToMarkdownStyle(str)

	// Extract frontmatter-like metadata from field lists at the start
	frontmatter, body := p.extractFieldListMetadata(str)

	doc := &source.Document{
		ID:          GenerateDocID("rst", filename, content),
		Filename:    filepath.Base(filename),
		Content:     str,
		Body:        converted,
		Frontmatter: frontmatter,
	}

	if body != "" && frontmatter != nil {
		doc.Body = p.convertToMarkdownStyle(body)
	}

	return doc, nil
}

// CanParse returns true if this parser can handle the given MIME type.
func (p *RSTParser) CanParse(mimeType string) bool {
	switch mimeType {
	case "text/x-rst", "text/rst", "text/restructuredtext":
		return true
	default:
		return false
	}
}

// MimeType returns the primary MIME type for this parser.
func (p *RSTParser) MimeType() string {
	return "text/x-rst"
}

// extractFieldListMetadata extracts field list metadata from the start of an RST document.
func (p *RSTParser) extractFieldListMetadata(content string) (map[string]any, string) {
	lines := strings.Split(content, "\n")
	metadata := make(map[string]any)
	bodyStart := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		match := rstFieldList.FindStringSubmatch(trimmed)
		if match != nil {
			key := strings.ToLower(strings.TrimSpace(match[1]))
			value := strings.TrimSpace(match[2])
			metadata[key] = value
			bodyStart = i + 1
		} else {
			// First non-field-list, non-empty line marks end of metadata
			break
		}
	}

	if len(metadata) == 0 {
		return nil, content
	}

	// Return the body without the field list
	body := strings.Join(lines[bodyStart:], "\n")
	return metadata, strings.TrimLeft(body, "\n")
}
