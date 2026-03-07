package parser

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/c360studio/semsource/source"
)

// AsciiDoc patterns
var (
	// Section titles: = Title, == Subtitle, etc.
	adocSectionTitle = regexp.MustCompile(`^(={1,6})\s+(.+)$`)

	// Attribute definitions: :name: value
	adocAttribute = regexp.MustCompile(`^:([^:]+):\s*(.*)$`)

	// Source code block: [source,lang] or ---- block
	adocSourceBlock  = regexp.MustCompile(`^\[source(?:,\s*([^\]]+))?\]`)
	adocListingBlock = regexp.MustCompile(`^----$`)

	// Passthrough block: ++++
	adocPassthroughBlock = regexp.MustCompile(`^\+\+\+\+$`)

	// Literal block: ....
	adocLiteralBlock = regexp.MustCompile(`^\.\.\.\.+$`)

	// Sidebar block: ****
	adocSidebarBlock = regexp.MustCompile(`^\*\*\*\*+$`)

	// Example block: ====
	adocExampleBlock = regexp.MustCompile(`^====+$`)

	// Admonition blocks: NOTE:, TIP:, WARNING:, etc.
	adocAdmonition = regexp.MustCompile(`^(NOTE|TIP|IMPORTANT|WARNING|CAUTION):\s*(.*)$`)

	// Block macro: name::target[attributes]
	adocBlockMacro = regexp.MustCompile(`^([a-z]+)::([^\[]*)\[([^\]]*)\]$`)
)

// ASCIIDocParser parses AsciiDoc documents.
type ASCIIDocParser struct{}

// NewASCIIDocParser creates a new AsciiDoc parser.
func NewASCIIDocParser() *ASCIIDocParser {
	return &ASCIIDocParser{}
}

// Parse parses an AsciiDoc document.
func (p *ASCIIDocParser) Parse(filename string, content []byte) (*source.Document, error) {
	str := string(content)

	// Extract document attributes (similar to frontmatter)
	attributes, body := p.extractAttributes(str)

	// Convert to markdown-style for compatibility
	converted := p.convertToMarkdownStyle(body)

	doc := &source.Document{
		ID:          GenerateDocID("asciidoc", filename, content),
		Filename:    filepath.Base(filename),
		Content:     str,
		Body:        converted,
		Frontmatter: attributes,
	}

	return doc, nil
}

// CanParse returns true if this parser can handle the given MIME type.
func (p *ASCIIDocParser) CanParse(mimeType string) bool {
	switch mimeType {
	case "text/asciidoc", "text/x-asciidoc":
		return true
	default:
		return false
	}
}

// MimeType returns the primary MIME type for this parser.
func (p *ASCIIDocParser) MimeType() string {
	return "text/asciidoc"
}

// extractAttributes extracts document attributes from the header.
func (p *ASCIIDocParser) extractAttributes(content string) (map[string]any, string) {
	lines := strings.Split(content, "\n")
	attributes := make(map[string]any)
	bodyStart := 0

	// Skip any leading title (= Document Title)
	inHeader := true

	for i, line := range lines {
		// Check for document title (must be first non-blank, non-attribute line)
		if inHeader && strings.HasPrefix(line, "= ") && !strings.HasPrefix(line, "==") {
			// Document title - extract it
			attributes["title"] = strings.TrimPrefix(line, "= ")
			bodyStart = i + 1
			continue
		}

		// Check for attribute
		if match := adocAttribute.FindStringSubmatch(line); match != nil {
			key := strings.ToLower(strings.TrimSpace(match[1]))
			value := strings.TrimSpace(match[2])
			if value == "" {
				// Unset attribute or boolean flag
				attributes[key] = true
			} else {
				attributes[key] = value
			}
			bodyStart = i + 1
			continue
		}

		// Empty line in header is allowed
		if strings.TrimSpace(line) == "" && inHeader {
			continue
		}

		// Any other content ends the header
		inHeader = false
		if bodyStart == 0 {
			bodyStart = i
		}
	}

	if len(attributes) == 0 {
		return nil, content
	}

	// Return the body without the header
	body := strings.Join(lines[bodyStart:], "\n")
	return attributes, strings.TrimLeft(body, "\n")
}
