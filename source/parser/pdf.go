package parser

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/source"
	"github.com/ledongthuc/pdf"
)

// PDFParser parses PDF documents by extracting text content.
type PDFParser struct{}

// NewPDFParser creates a new PDF parser.
func NewPDFParser() *PDFParser {
	return &PDFParser{}
}

// Parse parses a PDF document and extracts text content.
func (p *PDFParser) Parse(filename string, content []byte) (*source.Document, error) {
	// Write content to a temporary file for the PDF library
	// The pdf library requires a file path, not bytes
	// We'll use a ReaderAt approach instead

	reader, err := pdf.NewReader(newBytesReaderAt(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("open PDF: %w", err)
	}

	var textBuilder strings.Builder

	// Extract text from each page
	numPages := reader.NumPage()
	for i := 1; i <= numPages; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			// Log but continue - some pages may fail to parse
			continue
		}

		if text != "" {
			if textBuilder.Len() > 0 {
				textBuilder.WriteString("\n\n---\n\n") // Page separator
			}
			textBuilder.WriteString(text)
		}
	}

	extractedText := textBuilder.String()
	if extractedText == "" {
		// If no text extracted, the PDF might be image-based
		extractedText = fmt.Sprintf("[PDF document with %d pages - no text content extracted]", numPages)
	}

	doc := &source.Document{
		ID:       GenerateDocID("pdf", filename, content),
		Filename: filepath.Base(filename),
		Content:  extractedText,
		Body:     extractedText,
	}

	return doc, nil
}

// CanParse returns true if this parser can handle the given MIME type.
func (p *PDFParser) CanParse(mimeType string) bool {
	return mimeType == "application/pdf"
}

// MimeType returns the primary MIME type for this parser.
func (p *PDFParser) MimeType() string {
	return "application/pdf"
}

// bytesReaderAt implements io.ReaderAt for a byte slice.
type bytesReaderAt struct {
	data []byte
}

func newBytesReaderAt(data []byte) *bytesReaderAt {
	return &bytesReaderAt{data: data}
}

func (r *bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, fmt.Errorf("negative offset")
	}
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n = copy(p, r.data[off:])
	if n < len(p) {
		err = io.EOF
	}
	return n, err
}
