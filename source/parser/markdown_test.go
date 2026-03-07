package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkdownParser_Parse_NoFrontmatter(t *testing.T) {
	p := NewMarkdownParser()

	content := `# Hello World

This is a test document.

## Section 1

Some content here.
`

	doc, err := p.Parse("test.md", []byte(content))
	require.NoError(t, err)

	assert.NotEmpty(t, doc.ID)
	assert.Equal(t, "test.md", doc.Filename)
	assert.Equal(t, content, doc.Content)
	assert.Equal(t, content, doc.Body)
	assert.False(t, doc.HasFrontmatter())
}

func TestMarkdownParser_Parse_WithFrontmatter(t *testing.T) {
	p := NewMarkdownParser()

	content := `---
category: sop
applies_to:
  - "*.go"
  - "handlers/*.go"
severity: error
summary: Go error handling guidelines
requirements:
  - Always wrap errors with context
  - Never ignore errors
---
# Error Handling SOP

All Go code must follow these error handling guidelines.
`

	doc, err := p.Parse("error-handling.md", []byte(content))
	require.NoError(t, err)

	assert.NotEmpty(t, doc.ID)
	assert.Equal(t, "error-handling.md", doc.Filename)
	assert.True(t, doc.HasFrontmatter())

	// Check frontmatter fields
	assert.Equal(t, "sop", doc.Frontmatter["category"])
	assert.Equal(t, "error", doc.Frontmatter["severity"])
	assert.Equal(t, "Go error handling guidelines", doc.Frontmatter["summary"])

	// Check applies_to
	appliesTo, ok := doc.Frontmatter["applies_to"].([]any)
	require.True(t, ok)
	assert.Len(t, appliesTo, 2)
	assert.Equal(t, "*.go", appliesTo[0])
	assert.Equal(t, "handlers/*.go", appliesTo[1])

	// Check requirements
	reqs, ok := doc.Frontmatter["requirements"].([]any)
	require.True(t, ok)
	assert.Len(t, reqs, 2)

	// Check body doesn't include frontmatter
	assert.True(t, len(doc.Body) < len(doc.Content))
	assert.Contains(t, doc.Body, "# Error Handling SOP")
	assert.NotContains(t, doc.Body, "---")
}

func TestMarkdownParser_Parse_InvalidFrontmatter(t *testing.T) {
	p := NewMarkdownParser()

	// Missing closing delimiter - should treat as body
	content := `---
category: sop

# No closing delimiter

Content here.
`

	doc, err := p.Parse("test.md", []byte(content))
	require.NoError(t, err)

	// Should not have frontmatter since delimiter wasn't closed
	assert.False(t, doc.HasFrontmatter())
	assert.Equal(t, content, doc.Body)
}

func TestMarkdownParser_Parse_MalformedYAML(t *testing.T) {
	p := NewMarkdownParser()

	content := `---
category: [unclosed array
---
# Test

Content.
`

	doc, err := p.Parse("test.md", []byte(content))
	require.NoError(t, err)

	// Malformed YAML means no frontmatter parsed
	assert.False(t, doc.HasFrontmatter())
	assert.Equal(t, content, doc.Body)
}

func TestMarkdownParser_Parse_WindowsLineEndings(t *testing.T) {
	p := NewMarkdownParser()

	content := "---\r\ncategory: sop\r\n---\r\n# Title\r\n"

	doc, err := p.Parse("test.md", []byte(content))
	require.NoError(t, err)

	assert.True(t, doc.HasFrontmatter())
	assert.Equal(t, "sop", doc.Frontmatter["category"])
}

func TestMarkdownParser_CanParse(t *testing.T) {
	p := NewMarkdownParser()

	tests := []struct {
		mimeType string
		want     bool
	}{
		{"text/markdown", true},
		{"text/x-markdown", true},
		{"text/plain", true},
		{"text/html", false},
		{"application/pdf", false},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			assert.Equal(t, tt.want, p.CanParse(tt.mimeType))
		})
	}
}

func TestDocument_FrontmatterAsAnalysis(t *testing.T) {
	p := NewMarkdownParser()

	content := `---
category: sop
applies_to:
  - "*.go"
severity: error
summary: Test summary
requirements:
  - Rule 1
  - Rule 2
---
# Content
`

	doc, err := p.Parse("test.md", []byte(content))
	require.NoError(t, err)

	analysis := doc.FrontmatterAsAnalysis()
	require.NotNil(t, analysis)

	assert.Equal(t, "sop", analysis.Category)
	assert.Equal(t, []string{"*.go"}, analysis.AppliesTo)
	assert.Equal(t, "error", analysis.Severity)
	assert.Equal(t, "Test summary", analysis.Summary)
	assert.Equal(t, []string{"Rule 1", "Rule 2"}, analysis.Requirements)
}

func TestDocument_FrontmatterAsAnalysis_Empty(t *testing.T) {
	p := NewMarkdownParser()

	content := `# No frontmatter
Content here.
`

	doc, err := p.Parse("test.md", []byte(content))
	require.NoError(t, err)

	analysis := doc.FrontmatterAsAnalysis()
	assert.Nil(t, analysis)
}

func TestDocument_FrontmatterAsAnalysis_IncompleteFields(t *testing.T) {
	p := NewMarkdownParser()

	// Frontmatter with unrelated fields
	content := `---
author: john
date: 2024-01-01
---
# Content
`

	doc, err := p.Parse("test.md", []byte(content))
	require.NoError(t, err)

	// Should return nil since no useful analysis fields
	analysis := doc.FrontmatterAsAnalysis()
	assert.Nil(t, analysis)
}

func TestGenerateDocID_Stability(t *testing.T) {
	// Same content should produce same ID
	content := []byte("# Test\n\nContent here.")

	id1 := GenerateDocID("markdown", "test.md", content)
	id2 := GenerateDocID("markdown", "test.md", content)

	assert.Equal(t, id1, id2)
}

func TestGenerateDocID_SixParts(t *testing.T) {
	content := []byte("# Test")
	id := GenerateDocID("markdown", "test.md", content)

	parts := strings.Split(id, ".")
	assert.Len(t, parts, 6, "entity ID must have 6 dot-separated parts, got: %s", id)
	assert.Equal(t, "c360", parts[0])
	assert.Equal(t, "semspec", parts[1])
	assert.Equal(t, "source", parts[2])
	assert.Equal(t, "doc", parts[3])
	assert.Equal(t, "markdown", parts[4])
}

func TestGenerateDocID_Uniqueness(t *testing.T) {
	// Different content should produce different IDs
	content1 := []byte("# Test 1")
	content2 := []byte("# Test 2")

	id1 := GenerateDocID("markdown", "test.md", content1)
	id2 := GenerateDocID("markdown", "test.md", content2)

	assert.NotEqual(t, id1, id2)
}

func TestGenerateChunkID_SixParts(t *testing.T) {
	content := []byte("# Test")
	id := GenerateChunkID("markdown", content, 1)

	parts := strings.Split(id, ".")
	assert.Len(t, parts, 6, "chunk ID must have 6 dot-separated parts, got: %s", id)
	assert.Equal(t, "c360", parts[0])
	assert.Equal(t, "semspec", parts[1])
	assert.Equal(t, "source", parts[2])
	assert.Equal(t, "chunk", parts[3])
	assert.Equal(t, "markdown", parts[4])
}

func TestSanitizeIDPart(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello-world", "hello-world"},
		{"Hello World", "helloworld"},
		{"test_file", "testfile"},
		{"special!@#chars", "specialchars"},
		{"123-test", "123-test"},
		{"", "unknown"},
		{"-leading-trailing-", "leading-trailing"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeIDPart(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContentHash(t *testing.T) {
	content := []byte("test content")
	hash := ContentHash(content)

	// SHA256 produces 64 hex chars
	assert.Len(t, hash, 64)

	// Same content produces same hash
	hash2 := ContentHash(content)
	assert.Equal(t, hash, hash2)

	// Different content produces different hash
	hash3 := ContentHash([]byte("different content"))
	assert.NotEqual(t, hash, hash3)
}
