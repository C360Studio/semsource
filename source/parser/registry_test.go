package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_GetByMimeType(t *testing.T) {
	r := NewRegistry()

	t.Run("direct match", func(t *testing.T) {
		p := r.GetByMimeType("text/markdown")
		assert.NotNil(t, p)
		assert.Equal(t, "text/markdown", p.MimeType())
	})

	t.Run("CanParse fallback", func(t *testing.T) {
		p := r.GetByMimeType("text/x-markdown")
		assert.NotNil(t, p)
	})

	t.Run("text/plain handled by markdown parser", func(t *testing.T) {
		p := r.GetByMimeType("text/plain")
		assert.NotNil(t, p)
	})

	t.Run("has parser for PDF", func(t *testing.T) {
		p := r.GetByMimeType("application/pdf")
		assert.NotNil(t, p)
		assert.Equal(t, "application/pdf", p.MimeType())
	})

	t.Run("no parser for unknown type", func(t *testing.T) {
		p := r.GetByMimeType("application/octet-stream")
		assert.Nil(t, p)
	})
}

func TestRegistry_GetByExtension(t *testing.T) {
	r := NewRegistry()

	tests := []struct {
		filename string
		wantNil  bool
	}{
		{"test.md", false},
		{"test.markdown", false},
		{"test.txt", false},
		{"test.pdf", false},
		{"test.rst", false},
		{"test.adoc", false},
		{"test.docx", true},
		{"noextension", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			p := r.GetByExtension(tt.filename)
			if tt.wantNil {
				assert.Nil(t, p)
			} else {
				assert.NotNil(t, p)
			}
		})
	}
}

func TestRegistry_Parse(t *testing.T) {
	r := NewRegistry()

	t.Run("success with markdown", func(t *testing.T) {
		doc, err := r.Parse("test.md", []byte("# Hello"))
		require.NoError(t, err)
		assert.Equal(t, "test.md", doc.Filename)
		assert.Contains(t, doc.Body, "# Hello")
	})

	t.Run("error when no parser", func(t *testing.T) {
		_, err := r.Parse("test.docx", []byte("content"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no parser for file type")
		assert.Contains(t, err.Error(), ".docx")
	})
}

func TestRegistry_ListMimeTypes(t *testing.T) {
	r := NewRegistry()

	types := r.ListMimeTypes()
	assert.Contains(t, types, "text/markdown")
}

func TestMimeTypeFromExtension(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".md", "text/markdown"},
		{".markdown", "text/markdown"},
		{".MD", "text/markdown"}, // case insensitive
		{".txt", "text/plain"},
		{".html", "text/html"},
		{".htm", "text/html"},
		{".json", "application/json"},
		{".yaml", "application/yaml"},
		{".yml", "application/yaml"},
		{".pdf", "application/pdf"},
		{".unknown", "application/octet-stream"},
		{"", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := MimeTypeFromExtension(tt.ext)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtensionFromMimeType(t *testing.T) {
	tests := []struct {
		mimeType string
		want     string
	}{
		{"text/markdown", ".md"},
		{"text/x-markdown", ".md"},
		{"text/plain", ".txt"},
		{"text/html", ".html"},
		{"application/json", ".json"},
		{"application/yaml", ".yaml"},
		{"application/pdf", ".pdf"},
		{"application/octet-stream", ""},
		{"unknown/type", ""},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			got := ExtensionFromMimeType(tt.mimeType)
			assert.Equal(t, tt.want, got)
		})
	}
}
