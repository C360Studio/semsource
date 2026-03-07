// Package parser provides document parsing functionality.
package parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/source"
	"gopkg.in/yaml.v3"
)

// MarkdownParser parses markdown documents with optional YAML frontmatter.
type MarkdownParser struct{}

// NewMarkdownParser creates a new markdown parser.
func NewMarkdownParser() *MarkdownParser {
	return &MarkdownParser{}
}

// Parse parses a markdown document, extracting frontmatter and body.
func (p *MarkdownParser) Parse(filename string, content []byte) (*source.Document, error) {
	doc := &source.Document{
		ID:       GenerateDocID("markdown", filename, content),
		Filename: filepath.Base(filename),
		Content:  string(content),
	}

	// Check for YAML frontmatter
	str := string(content)
	if strings.HasPrefix(str, "---\n") || strings.HasPrefix(str, "---\r\n") {
		frontmatter, body, err := extractFrontmatter(str)
		if err != nil {
			// If frontmatter parsing fails, treat entire content as body
			doc.Body = str
		} else {
			doc.Frontmatter = frontmatter
			doc.Body = body
		}
	} else {
		doc.Body = str
	}

	return doc, nil
}

// CanParse returns true if this parser can handle the given MIME type.
func (p *MarkdownParser) CanParse(mimeType string) bool {
	switch mimeType {
	case "text/markdown", "text/x-markdown", "text/plain":
		return true
	default:
		return false
	}
}

// MimeType returns the primary MIME type for this parser.
func (p *MarkdownParser) MimeType() string {
	return "text/markdown"
}

// extractFrontmatter parses YAML frontmatter from markdown content.
// Returns the parsed frontmatter map, the remaining body, and any error.
func extractFrontmatter(content string) (map[string]any, string, error) {
	// Find the closing delimiter
	const delimiter = "---"
	const minDelimLen = 4 // "---\n"

	// Skip the opening delimiter
	start := len(delimiter)
	if len(content) > start && content[start] == '\r' {
		start++
	}
	if len(content) > start && content[start] == '\n' {
		start++
	}

	// Find the closing delimiter
	closeIdx := strings.Index(content[start:], "\n"+delimiter)
	if closeIdx == -1 {
		closeIdx = strings.Index(content[start:], "\r\n"+delimiter)
	}
	if closeIdx == -1 {
		return nil, content, fmt.Errorf("no closing frontmatter delimiter")
	}

	yamlContent := content[start : start+closeIdx]

	// Find where the body starts (after closing delimiter and newline)
	bodyStart := start + closeIdx + 1 + len(delimiter)
	for bodyStart < len(content) && (content[bodyStart] == '\n' || content[bodyStart] == '\r') {
		bodyStart++
	}

	body := ""
	if bodyStart < len(content) {
		body = content[bodyStart:]
	}

	// Parse YAML
	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(yamlContent), &frontmatter); err != nil {
		return nil, content, fmt.Errorf("parse YAML frontmatter: %w", err)
	}

	return frontmatter, body, nil
}

// GenerateDocID creates a 6-part entity ID for a document.
// Format: c360.semspec.source.doc.{format}.{instance}
// Each part is lowercase alphanumeric only (no hyphens, underscores, etc).
func GenerateDocID(format, filename string, content []byte) string {
	base := filepath.Base(filename)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	instance := SanitizeIDPart(name)

	// Append hash suffix for uniqueness (12 hex chars)
	hash := sha256.Sum256(content)
	instance = instance + hex.EncodeToString(hash[:])[:12]

	return fmt.Sprintf("c360.semspec.source.doc.%s.%s", SanitizeIDPart(format), instance)
}

// GenerateChunkID creates a 6-part entity ID for a document chunk.
// Format: c360.semspec.source.chunk.{format}.{parenthash}{index}
func GenerateChunkID(format string, parentContent []byte, index int) string {
	hash := sha256.Sum256(parentContent)
	instance := hex.EncodeToString(hash[:])[:12] + fmt.Sprintf("%04d", index)
	return fmt.Sprintf("c360.semspec.source.chunk.%s.%s", SanitizeIDPart(format), instance)
}

// SanitizeIDPart strips characters that are not lowercase alphanumeric or hyphens.
// Dots are separators between the 6 parts, so they are stripped. Hyphens are allowed
// within parts (they are valid in NATS subjects/KV keys, just not separators).
func SanitizeIDPart(s string) string {
	var buf bytes.Buffer
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			buf.WriteRune(r)
		}
	}
	result := buf.String()
	// Trim leading/trailing hyphens
	result = strings.Trim(result, "-")
	if result == "" {
		return "unknown"
	}
	return result
}

// ContentHash computes a SHA256 hash of the content.
func ContentHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}
