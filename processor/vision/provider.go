// Package vision implements the VisionProcessor, which enriches media entities
// with ML-generated vision labels by sending binary content to a pluggable
// vision model backend.
//
// The processor sits in the consumer flow between WebSocket input and graph
// ingestion. It operates on handler.RawEntity slices pre-normalization, so
// entity IDs are not yet assigned when Process is called.
//
// Usage:
//
//	provider := myclaudeprovider.New(apiKey)
//	store := objectstore.NewStore(ctx, natsConn, cfg)
//	proc := vision.New(provider, store, vision.WithConfig(vision.DefaultConfig()))
//
//	enriched := proc.Process(ctx, entities)
package vision

import "context"

// Result holds the output from a vision model analysis.
type Result struct {
	// Labels are the ML-detected labels or tags for the image.
	Labels []string

	// Description is a natural-language description of the image.
	Description string

	// Confidence is the overall analysis confidence score (0.0–1.0).
	Confidence float64

	// Objects are structured detections with optional bounding boxes.
	Objects []DetectedObject

	// Text is OCR-extracted text found in the image.
	// Empty string when no text was detected.
	Text string

	// Model is the model identifier that produced this result
	// (e.g. "claude-3-5-sonnet-20241022").
	Model string
}

// DetectedObject represents a detected object with an optional bounding box.
type DetectedObject struct {
	Label       string       `json:"label"`
	Confidence  float64      `json:"confidence"`
	BoundingBox *BoundingBox `json:"bounding_box,omitempty"`
}

// BoundingBox is a normalised (0.0–1.0) bounding box within the image plane.
// The origin (0,0) is the top-left corner.
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Provider is the interface for pluggable vision model backends.
// Implementations might wrap Claude Vision, OpenAI Vision, local models, etc.
type Provider interface {
	// Analyze sends image data to the vision model and returns the result.
	// mimeType indicates the binary format of data (e.g. "image/png").
	// Implementations must honour context cancellation.
	Analyze(ctx context.Context, data []byte, mimeType string) (*Result, error)
}
