package image_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler"
	imagehandler "github.com/c360studio/semsource/handler/image"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sourceConfig adapts test parameters to handler.SourceConfig.
type sourceConfig struct {
	typ   string
	path  string
	url   string
	watch bool
}

func (s sourceConfig) GetType() string      { return s.typ }
func (s sourceConfig) GetPath() string      { return s.path }
func (s sourceConfig) GetURL() string       { return s.url }
func (s sourceConfig) IsWatchEnabled() bool { return s.watch }

// write1x1PNG generates a minimal 1×1 pixel PNG and writes it to dir/name.
// Returns the absolute path of the written file.
func write1x1PNG(t *testing.T, dir, name string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// writeSVG writes a minimal SVG file and returns its absolute path.
func writeSVG(t *testing.T, dir, name string) string {
	t.Helper()
	content := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"></svg>`
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// ---------------------------------------------------------------------------
// SourceType / Supports
// ---------------------------------------------------------------------------

func TestImageHandler_SourceType(t *testing.T) {
	h := imagehandler.New()
	if got := h.SourceType(); got != "image" {
		t.Errorf("SourceType() = %q, want %q", got, "image")
	}
}

func TestImageHandler_Supports(t *testing.T) {
	h := imagehandler.New()

	tests := []struct {
		typ  string
		want bool
	}{
		{"image", true},
		{"doc", false},
		{"git", false},
		{"", false},
	}
	for _, tt := range tests {
		cfg := sourceConfig{typ: tt.typ}
		if got := h.Supports(cfg); got != tt.want {
			t.Errorf("Supports(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Ingest — basic entity production
// ---------------------------------------------------------------------------

func TestImageHandler_Ingest_ProducesEntity(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("entity count: got %d, want 1", len(entities))
	}
}

func TestImageHandler_Ingest_CorrectDomainAndEntityType(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities returned")
	}

	e := entities[0]
	if e.Domain != handler.DomainMedia {
		t.Errorf("Domain = %q, want %q", e.Domain, handler.DomainMedia)
	}
	if e.EntityType != "image" {
		t.Errorf("EntityType = %q, want %q", e.EntityType, "image")
	}
	if e.SourceType != handler.SourceTypeImage {
		t.Errorf("SourceType = %q, want %q", e.SourceType, handler.SourceTypeImage)
	}
}

func TestImageHandler_Ingest_InstanceIsSHA256Prefix(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	instance := entities[0].Instance
	if len(instance) != 6 {
		t.Errorf("Instance length = %d, want 6 (sha256[:6]): %q", len(instance), instance)
	}
	for _, ch := range instance {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			t.Errorf("Instance %q contains non-hex character %q", instance, ch)
		}
	}
}

func TestImageHandler_Ingest_PNGDimensions(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	e := entities[0]

	// Check Properties map for width and height.
	if w, ok := e.Properties["width"]; !ok || w != 1 {
		t.Errorf("Properties[width] = %v, want 1", w)
	}
	if h2, ok := e.Properties["height"]; !ok || h2 != 1 {
		t.Errorf("Properties[height] = %v, want 1", h2)
	}

	// Confirm triples carry the same values.
	var wTriple, hTriple *tripleValue
	for _, tr := range e.Triples {
		switch tr.Predicate {
		case "source.media.width":
			v := tr.Object
			wTriple = &tripleValue{v: v}
		case "source.media.height":
			v := tr.Object
			hTriple = &tripleValue{v: v}
		}
	}
	if wTriple == nil {
		t.Error("missing width triple")
	} else if wTriple.v != 1 {
		t.Errorf("width triple Object = %v, want 1", wTriple.v)
	}
	if hTriple == nil {
		t.Error("missing height triple")
	} else if hTriple.v != 1 {
		t.Errorf("height triple Object = %v, want 1", hTriple.v)
	}
}

// tripleValue is a small helper to hold a Triple Object value for assertion.
type tripleValue struct{ v any }

func TestImageHandler_Ingest_SVGDimensionsAreZero(t *testing.T) {
	dir := t.TempDir()
	writeSVG(t, dir, "icon.svg")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities for SVG")
	}

	e := entities[0]
	if w, ok := e.Properties["width"]; !ok || w != 0 {
		t.Errorf("SVG Properties[width] = %v, want 0", w)
	}
	if h2, ok := e.Properties["height"]; !ok || h2 != 0 {
		t.Errorf("SVG Properties[height] = %v, want 0", h2)
	}
}

func TestImageHandler_Ingest_MimeType(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	e := entities[0]
	if mt, ok := e.Properties["mime_type"]; !ok || mt != "image/png" {
		t.Errorf("mime_type = %v, want image/png", mt)
	}

	// Confirm mime_type triple is present.
	found := false
	for _, tr := range e.Triples {
		if tr.Predicate == "source.media.mime_type" && tr.Object == "image/png" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected mime_type triple with value 'image/png'")
	}
}

func TestImageHandler_Ingest_Format(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	if fmt, ok := entities[0].Properties["format"]; !ok || fmt != "png" {
		t.Errorf("format = %v, want png", fmt)
	}
}

func TestImageHandler_Ingest_FilePathTriple(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	found := false
	for _, tr := range entities[0].Triples {
		if tr.Predicate == "source.media.file_path" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected triple with predicate source.media.file_path")
	}
}

func TestImageHandler_Ingest_ContentHashTriple(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	found := false
	for _, tr := range entities[0].Triples {
		if tr.Predicate == "source.media.file_hash" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected triple with predicate source.media.file_hash")
	}
}

func TestImageHandler_Ingest_FileSizeTriple(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	found := false
	for _, tr := range entities[0].Triples {
		if tr.Predicate == "source.media.file_size" {
			found = true
			if tr.Object.(int64) <= 0 {
				t.Errorf("file_size triple Object = %v, want > 0", tr.Object)
			}
			break
		}
	}
	if !found {
		t.Error("expected triple with predicate source.media.file_size")
	}
}

func TestImageHandler_Ingest_FiltersByExtension(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")
	// Non-image file — should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Doc"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) != 1 {
		t.Errorf("entity count: got %d, want 1 (only .png)", len(entities))
	}
}

func TestImageHandler_Ingest_MultipleImages(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "a.png")
	write1x1PNG(t, dir, "b.png")
	writeSVG(t, dir, "c.svg")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) != 3 {
		t.Errorf("entity count: got %d, want 3", len(entities))
	}
}

func TestImageHandler_Ingest_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("entity count: got %d, want 0 for empty dir", len(entities))
	}
}

func TestImageHandler_Ingest_InstanceChangesOnModify(t *testing.T) {
	dir := t.TempDir()
	path := write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities1, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() first: %v", err)
	}

	// Write a different PNG (2×2 so content hash changes).
	img2 := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img2.Set(0, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img2); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	entities2, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() second: %v", err)
	}

	if len(entities1) == 0 || len(entities2) == 0 {
		t.Fatal("expected entities from both ingests")
	}
	// Instance (sha256 prefix) must differ after content change.
	if entities1[0].Instance == entities2[0].Instance {
		t.Error("Instance should change when file content changes")
	}
}

// ---------------------------------------------------------------------------
// Watch — disabled returns nil
// ---------------------------------------------------------------------------

func TestImageHandler_Watch_WatchDisabledReturnsNil(t *testing.T) {
	dir := t.TempDir()

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir, watch: false}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := h.Watch(ctx, cfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	if ch != nil {
		t.Error("Watch() should return nil channel when watch is disabled")
	}
}
