package parser

import (
	"testing"
)

func TestPDFParser_MimeType(t *testing.T) {
	p := NewPDFParser()
	if p.MimeType() != "application/pdf" {
		t.Errorf("expected application/pdf, got %s", p.MimeType())
	}
}

func TestPDFParser_CanParse(t *testing.T) {
	p := NewPDFParser()

	tests := []struct {
		mimeType string
		want     bool
	}{
		{"application/pdf", true},
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

func TestPDFParser_ParseInvalidPDF(t *testing.T) {
	p := NewPDFParser()

	// Invalid PDF content
	content := []byte("not a pdf file")

	_, err := p.Parse("test.pdf", content)
	if err == nil {
		t.Error("expected error for invalid PDF content")
	}
}

// Note: Testing actual PDF parsing requires a valid PDF file.
// The PDF library needs a properly formatted PDF document.
// Integration tests with real PDF files would go here.
