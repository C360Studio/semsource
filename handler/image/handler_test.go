package image_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semsource/handler"
	imagehandler "github.com/c360studio/semsource/handler/image"
)

// memStore is an in-memory implementation of storage.Store for testing.
type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStore() *memStore {
	return &memStore{data: make(map[string][]byte)}
}

func (s *memStore) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *memStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.data[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return d, nil
}

func (s *memStore) List(_ context.Context, prefix string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (s *memStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func (s *memStore) keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for k := range s.data {
		out = append(out, k)
	}
	return out
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sourceConfig adapts test parameters to handler.SourceConfig.
type sourceConfig struct {
	typ   string
	path  string
	paths []string
	url   string
	watch bool
}

func (s sourceConfig) GetType() string             { return s.typ }
func (s sourceConfig) GetPaths() []string          { return s.paths }
func (s sourceConfig) GetURL() string              { return s.url }
func (s sourceConfig) GetBranch() string           { return "" }
func (s sourceConfig) IsWatchEnabled() bool        { return s.watch }
func (s sourceConfig) GetKeyframeMode() string     { return "" }
func (s sourceConfig) GetKeyframeInterval() string { return "" }
func (s sourceConfig) GetSceneThreshold() float64  { return 0 }

// GetPath returns the primary path: the explicit path field when set, or the
// first element of paths when only a multi-path config is provided.
func (s sourceConfig) GetPath() string {
	if s.path != "" {
		return s.path
	}
	if len(s.paths) > 0 {
		return s.paths[0]
	}
	return ""
}

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

}

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

func TestImageHandler_Ingest_FilePathProperty(t *testing.T) {
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

	if fp, ok := entities[0].Properties["file_path"]; !ok || fp == "" {
		t.Errorf("Properties[file_path] = %v, want non-empty string", fp)
	}
}

func TestImageHandler_Ingest_ContentHashProperty(t *testing.T) {
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

	if fh, ok := entities[0].Properties["file_hash"]; !ok || fh == "" {
		t.Errorf("Properties[file_hash] = %v, want non-empty string", fh)
	}
}

func TestImageHandler_Ingest_FileSizeProperty(t *testing.T) {
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

	fs, ok := entities[0].Properties["file_size"]
	if !ok {
		t.Fatal("Properties[file_size] missing")
	}
	if fs.(int64) <= 0 {
		t.Errorf("Properties[file_size] = %v, want > 0", fs)
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

// ---------------------------------------------------------------------------
// ObjectStore binary storage
// ---------------------------------------------------------------------------

func TestImageHandler_Ingest_WithStore_StoresOriginal(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	store := newMemStore()
	h := imagehandler.New(imagehandler.WithStore(store))
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	// Should have stored the original binary.
	keys := store.keys()
	var originalKey string
	for _, k := range keys {
		if strings.Contains(k, "/original") {
			originalKey = k
		}
	}
	if originalKey == "" {
		t.Fatalf("no original key found in store; keys: %v", keys)
	}

	// Verify the stored data matches the file.
	fileData, _ := os.ReadFile(filepath.Join(dir, "photo.png"))
	storedData, _ := store.Get(context.Background(), originalKey)
	if !bytes.Equal(fileData, storedData) {
		t.Error("stored data does not match file content")
	}
}

func TestImageHandler_Ingest_WithStore_StorageRefProperty(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	store := newMemStore()
	h := imagehandler.New(imagehandler.WithStore(store))
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	ref, ok := entities[0].Properties["storage_ref"]
	if !ok {
		t.Fatal("Properties[storage_ref] missing")
	}
	key, ok := ref.(string)
	if !ok || !strings.Contains(key, "/original") {
		t.Errorf("Properties[storage_ref] = %v, expected string containing '/original'", ref)
	}
}

func TestImageHandler_Ingest_WithStore_NoThumbnailForSmallImage(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "tiny.png") // 1x1 — well under 512px threshold

	store := newMemStore()
	h := imagehandler.New(imagehandler.WithStore(store))
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	// No thumbnail should be generated for a 1x1 image.
	for _, k := range store.keys() {
		if strings.Contains(k, "/thumbnail") {
			t.Error("unexpected thumbnail stored for 1x1 image")
		}
	}

	// No thumbnail_ref property should exist.
	if _, ok := entities[0].Properties["thumbnail_ref"]; ok {
		t.Error("unexpected thumbnail_ref property for 1x1 image")
	}
}

// writeLargePNG generates a 1024×768 solid-color PNG — large enough to trigger thumbnail generation.
func writeLargePNG(t *testing.T, dir, name string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1024, 768))
	for y := range 768 {
		for x := range 1024 {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}

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

func TestImageHandler_Ingest_WithStore_ThumbnailForLargeImage(t *testing.T) {
	dir := t.TempDir()
	writeLargePNG(t, dir, "large.png")

	store := newMemStore()
	h := imagehandler.New(imagehandler.WithStore(store))
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	// Should have both original and thumbnail stored.
	var hasOriginal, hasThumbnail bool
	for _, k := range store.keys() {
		if strings.Contains(k, "/original") {
			hasOriginal = true
		}
		if strings.Contains(k, "/thumbnail") {
			hasThumbnail = true
		}
	}
	if !hasOriginal {
		t.Error("expected original in store")
	}
	if !hasThumbnail {
		t.Error("expected thumbnail in store for 1024x768 image")
	}

	// Should have thumbnail_ref property.
	ref, ok := entities[0].Properties["thumbnail_ref"]
	if !ok {
		t.Fatal("Properties[thumbnail_ref] missing")
	}
	key, ok := ref.(string)
	if !ok || !strings.Contains(key, "/thumbnail") {
		t.Errorf("Properties[thumbnail_ref] = %v, expected string containing '/thumbnail'", ref)
	}
}

func TestImageHandler_Ingest_WithoutStore_NoStorageTriples(t *testing.T) {
	dir := t.TempDir()
	writeLargePNG(t, dir, "large.png")

	// No store — metadata-only mode.
	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	if _, ok := entities[0].Properties["storage_ref"]; ok {
		t.Error("unexpected storage_ref property in metadata-only mode")
	}
	if _, ok := entities[0].Properties["thumbnail_ref"]; ok {
		t.Error("unexpected thumbnail_ref property in metadata-only mode")
	}
}

// ---------------------------------------------------------------------------
// Multi-path Ingest
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// IngestEntityStates — normalizer-free typed entity production
// ---------------------------------------------------------------------------

func TestImageHandler_IngestEntityStates_ReturnsEntityStates(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("entity state count: got %d, want 1", len(states))
	}
}

func TestImageHandler_IngestEntityStates_IDContainsOrg(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no states returned")
	}

	if !strings.HasPrefix(states[0].ID, "acme.") {
		t.Errorf("ID %q does not start with org prefix %q", states[0].ID, "acme.")
	}
}

func TestImageHandler_IngestEntityStates_IDHasSixParts(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no states returned")
	}

	parts := strings.Split(states[0].ID, ".")
	if len(parts) != 6 {
		t.Errorf("ID %q has %d parts, want 6", states[0].ID, len(parts))
	}
}

func TestImageHandler_IngestEntityStates_TriplesContainVocabPredicates(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no states returned")
	}

	predicates := make(map[string]bool)
	for _, tr := range states[0].Triples {
		predicates[tr.Predicate] = true
	}

	required := []string{
		"source.media.type",
		"source.media.file_path",
		"source.media.mime_type",
		"source.media.file_hash",
		"source.media.file_size",
		"source.media.format",
		"source.media.width",
		"source.media.height",
	}
	for _, p := range required {
		if !predicates[p] {
			t.Errorf("missing required predicate %q in triples", p)
		}
	}
}

func TestImageHandler_IngestEntityStates_MediaTypeTripleIsImage(t *testing.T) {
	dir := t.TempDir()
	write1x1PNG(t, dir, "photo.png")

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no states returned")
	}

	for _, tr := range states[0].Triples {
		if tr.Predicate == "source.media.type" {
			if tr.Object != "image" {
				t.Errorf("media.type triple Object = %v, want %q", tr.Object, "image")
			}
			return
		}
	}
	t.Error("source.media.type triple not found")
}

func TestImageHandler_IngestEntityStates_EmptyDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()

	h := imagehandler.New()
	cfg := sourceConfig{typ: "image", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("entity state count: got %d, want 0 for empty dir", len(states))
	}
}

func TestImageHandler_Ingest_MultiplePaths(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	// Place distinct PNG files in each directory so entities from both are
	// expected in the result set.
	write1x1PNG(t, dirA, "alpha.png")
	writeSVG(t, dirA, "alpha.svg")
	write1x1PNG(t, dirB, "beta.png")

	h := imagehandler.New()
	cfg := sourceConfig{
		typ:   "image",
		paths: []string{dirA, dirB},
	}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}

	// dirA contributes 2 files (alpha.png, alpha.svg); dirB contributes 1 (beta.png).
	if len(entities) != 3 {
		t.Errorf("entity count: got %d, want 3 (2 from dirA, 1 from dirB)", len(entities))
	}
}
