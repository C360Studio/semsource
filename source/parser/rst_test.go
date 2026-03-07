package parser

import (
	"strings"
	"testing"
)

func TestRSTParser_MimeType(t *testing.T) {
	p := NewRSTParser()
	if p.MimeType() != "text/x-rst" {
		t.Errorf("expected text/x-rst, got %s", p.MimeType())
	}
}

func TestRSTParser_CanParse(t *testing.T) {
	p := NewRSTParser()

	tests := []struct {
		mimeType string
		want     bool
	}{
		{"text/x-rst", true},
		{"text/rst", true},
		{"text/restructuredtext", true},
		{"text/plain", false},
		{"text/markdown", false},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			got := p.CanParse(tt.mimeType)
			if got != tt.want {
				t.Errorf("CanParse(%s) = %v, want %v", tt.mimeType, got, tt.want)
			}
		})
	}
}

func TestRSTParser_ParseBasicDocument(t *testing.T) {
	p := NewRSTParser()

	content := `Title
=====

This is a paragraph.

Subtitle
--------

Another paragraph here.
`

	doc, err := p.Parse("test.rst", []byte(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if doc.ID == "" {
		t.Error("expected document ID to be set")
	}

	if doc.Filename != "test.rst" {
		t.Errorf("expected filename test.rst, got %s", doc.Filename)
	}

	// Check that sections were converted to markdown headings
	if !strings.Contains(doc.Body, "# Title") {
		t.Error("expected # Title heading")
	}

	if !strings.Contains(doc.Body, "## Subtitle") {
		t.Error("expected ## Subtitle heading")
	}
}

func TestRSTParser_ParseWithFieldList(t *testing.T) {
	p := NewRSTParser()

	content := `:author: John Doe
:version: 1.0
:date: 2024-01-01

Title
=====

Content here.
`

	doc, err := p.Parse("test.rst", []byte(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if doc.Frontmatter == nil {
		t.Fatal("expected frontmatter to be extracted")
	}

	if doc.Frontmatter["author"] != "John Doe" {
		t.Errorf("expected author John Doe, got %v", doc.Frontmatter["author"])
	}

	if doc.Frontmatter["version"] != "1.0" {
		t.Errorf("expected version 1.0, got %v", doc.Frontmatter["version"])
	}
}

func TestRSTParser_ParseCodeBlock(t *testing.T) {
	p := NewRSTParser()

	content := `Code Example
============

Here is some code::

    def hello():
        print("Hello, World!")

More text after.
`

	doc, err := p.Parse("test.rst", []byte(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should have code blocks converted to markdown fences
	if !strings.Contains(doc.Body, "```") {
		t.Error("expected code block fences")
	}
}

func TestRSTParser_ParseDirectiveCodeBlock(t *testing.T) {
	p := NewRSTParser()

	content := `Example
=======

.. code-block:: python

    def hello():
        print("Hello")
`

	doc, err := p.Parse("test.rst", []byte(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !strings.Contains(doc.Body, "```") {
		t.Error("expected code block fence")
	}
}

func TestRSTParser_ConvertFieldListsInBody(t *testing.T) {
	p := NewRSTParser()

	content := `Document
========

:param name: The name parameter
:returns: A string value
`

	doc, err := p.Parse("test.rst", []byte(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Field lists in body should be converted to bold labels
	if !strings.Contains(doc.Body, "**param name:**") {
		t.Error("expected field list to be converted to bold label")
	}
}
