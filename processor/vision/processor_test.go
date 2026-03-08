package vision_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/processor/vision"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// mockProvider is a configurable VisionProvider for tests.
type mockProvider struct {
	result *vision.VisionResult
	err    error
	calls  int
	mu     sync.Mutex
}

func (m *mockProvider) Analyze(_ context.Context, _ []byte, _ string) (*vision.VisionResult, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return m.result, m.err
}

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

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// makeMediaEntity builds a RawEntity that the VisionProcessor should consider
// eligible for vision analysis.
//
// storageRef is the key used to pre-populate the memStore; the caller is
// responsible for calling store.Put before passing the entity to the processor.
func makeMediaEntity(mediaType, storageRef, mimeType string) handler.RawEntity {
	return handler.RawEntity{
		SourceType: handler.SourceTypeImage,
		Domain:     handler.DomainMedia,
		System:     "test-system",
		EntityType: mediaType,
		Instance:   "abc123",
		Properties: map[string]any{
			"media_type":  mediaType,
			"storage_ref": storageRef,
			"mime_type":   mimeType,
		},
	}
}

// seedStore writes a synthetic binary payload into store at key.
func seedStore(t *testing.T, store *memStore, key string, size int) {
	t.Helper()
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := store.Put(context.Background(), key, data); err != nil {
		t.Fatalf("seedStore: %v", err)
	}
}

// findProp returns a pointer to the value stored in entity.Properties under key,
// or nil when the key is absent.
func findProp(entity handler.RawEntity, key string) *struct{ val any } {
	v, ok := entity.Properties[key]
	if !ok {
		return nil
	}
	return &struct{ val any }{v}
}

// hasProp reports whether entity.Properties contains key.
func hasProp(entity handler.RawEntity, key string) bool {
	return findProp(entity, key) != nil
}

// defaultResult returns a non-trivial VisionResult for use in tests.
func defaultResult() *vision.VisionResult {
	return &vision.VisionResult{
		Labels:      []string{"cat", "indoor", "animal"},
		Description: "A cat sitting on a couch",
		Confidence:  0.92,
		Objects: []vision.DetectedObject{
			{Label: "cat", Confidence: 0.95, BoundingBox: &vision.BoundingBox{X: 0.1, Y: 0.2, Width: 0.3, Height: 0.4}},
		},
		Text:  "",
		Model: "claude-3-5-sonnet-20241022",
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestProcessor_Process_EnrichesImageEntity(t *testing.T) {
	store := newMemStore()
	seedStore(t, store, "images/test/abc123/original", 512)

	provider := &mockProvider{result: defaultResult()}
	proc := vision.New(provider, store)

	entity := makeMediaEntity("image", "images/test/abc123/original", "image/png")
	out := proc.Process(context.Background(), []handler.RawEntity{entity})

	if len(out) != 1 {
		t.Fatalf("Process returned %d entities, want 1", len(out))
	}
	enriched := out[0]

	// Labels property must be present and be a JSON array.
	lp := findProp(enriched, source.MediaVisionLabels)
	if lp == nil {
		t.Fatal("expected vision_labels property, got none")
	}
	var labels []string
	if err := json.Unmarshal([]byte(lp.val.(string)), &labels); err != nil {
		t.Fatalf("vision_labels is not a valid JSON array: %v", err)
	}
	if len(labels) != 3 || labels[0] != "cat" {
		t.Errorf("labels = %v, want [cat indoor animal]", labels)
	}

	// Description property.
	dp := findProp(enriched, source.MediaVisionDescription)
	if dp == nil {
		t.Fatal("expected vision_description property")
	}
	if dp.val != "A cat sitting on a couch" {
		t.Errorf("description = %v, want 'A cat sitting on a couch'", dp.val)
	}

	// Confidence property — stored as a string via fmt.Sprintf("%g", ...).
	cp := findProp(enriched, source.MediaVisionConfidence)
	if cp == nil {
		t.Fatal("expected vision_confidence property")
	}
	if cp.val != "0.92" {
		t.Errorf("confidence = %v, want \"0.92\"", cp.val)
	}

	// Model property.
	mp := findProp(enriched, source.MediaVisionModel)
	if mp == nil {
		t.Fatal("expected vision_model property")
	}
	if mp.val != "claude-3-5-sonnet-20241022" {
		t.Errorf("model = %v, want 'claude-3-5-sonnet-20241022'", mp.val)
	}

	// Objects property — JSON array.
	op := findProp(enriched, source.MediaVisionObjects)
	if op == nil {
		t.Fatal("expected vision_objects property")
	}
	var objs []vision.DetectedObject
	if err := json.Unmarshal([]byte(op.val.(string)), &objs); err != nil {
		t.Fatalf("vision_objects is not valid JSON: %v", err)
	}
	if len(objs) != 1 || objs[0].Label != "cat" {
		t.Errorf("objects = %v, want [{cat 0.95 ...}]", objs)
	}

	// Text property should NOT be present (result.Text == "").
	if hasProp(enriched, source.MediaVisionText) {
		t.Error("expected no vision_text property when result.Text is empty")
	}

	if provider.calls != 1 {
		t.Errorf("provider.Analyze called %d times, want 1", provider.calls)
	}
}

func TestProcessor_Process_SkipsNonMediaEntity(t *testing.T) {
	store := newMemStore()
	provider := &mockProvider{result: defaultResult()}
	proc := vision.New(provider, store)

	// Entity without media_type property.
	entity := handler.RawEntity{
		SourceType: handler.SourceTypeAST,
		Domain:     handler.DomainGolang,
		System:     "github.com-acme-repo",
		EntityType: "function",
		Instance:   "NewController",
		Properties: map[string]any{
			"file": "main.go",
		},
	}
	before := len(entity.Properties)

	out := proc.Process(context.Background(), []handler.RawEntity{entity})

	if len(out) != 1 {
		t.Fatalf("Process returned %d entities, want 1", len(out))
	}
	if len(out[0].Properties) != before {
		t.Errorf("non-media entity was modified: properties %d → %d", before, len(out[0].Properties))
	}
	if provider.calls != 0 {
		t.Errorf("provider should not be called for non-media entity; got %d calls", provider.calls)
	}
}

func TestProcessor_Process_SkipsWithoutStorageRef(t *testing.T) {
	store := newMemStore()
	provider := &mockProvider{result: defaultResult()}
	proc := vision.New(provider, store)

	// Media entity but no storage_ref property.
	entity := handler.RawEntity{
		SourceType: handler.SourceTypeImage,
		Domain:     handler.DomainMedia,
		System:     "test",
		EntityType: "image",
		Instance:   "abc123",
		Properties: map[string]any{
			"media_type": "image",
			"mime_type":  "image/png",
			// no storage_ref
		},
	}

	out := proc.Process(context.Background(), []handler.RawEntity{entity})

	if hasProp(out[0], source.MediaVisionLabels) {
		t.Error("entity without storage_ref should not have vision properties")
	}
	if provider.calls != 0 {
		t.Errorf("provider should not be called; got %d calls", provider.calls)
	}
}

func TestProcessor_Process_Disabled(t *testing.T) {
	store := newMemStore()
	seedStore(t, store, "images/test/abc123/original", 512)

	provider := &mockProvider{result: defaultResult()}
	cfg := vision.DefaultConfig()
	cfg.Enabled = false
	proc := vision.New(provider, store, vision.WithConfig(cfg))

	entity := makeMediaEntity("image", "images/test/abc123/original", "image/png")
	out := proc.Process(context.Background(), []handler.RawEntity{entity})

	if hasProp(out[0], source.MediaVisionLabels) {
		t.Error("disabled processor should not add vision properties")
	}
	if provider.calls != 0 {
		t.Errorf("disabled processor should not call provider; got %d calls", provider.calls)
	}
}

func TestProcessor_Process_MaxFileSizeExceeded(t *testing.T) {
	store := newMemStore()
	// Store 2 MiB; configure max at 1 MiB.
	seedStore(t, store, "images/test/large/original", 2*1024*1024)

	provider := &mockProvider{result: defaultResult()}
	cfg := vision.DefaultConfig()
	cfg.MaxFileSize = 1 * 1024 * 1024 // 1 MiB
	proc := vision.New(provider, store, vision.WithConfig(cfg))

	entity := makeMediaEntity("image", "images/test/large/original", "image/png")
	out := proc.Process(context.Background(), []handler.RawEntity{entity})

	if hasProp(out[0], source.MediaVisionLabels) {
		t.Error("entity exceeding MaxFileSize should not have vision properties")
	}
	if provider.calls != 0 {
		t.Errorf("provider should not be called for oversized binary; got %d calls", provider.calls)
	}
}

func TestProcessor_Process_ProviderError(t *testing.T) {
	store := newMemStore()
	seedStore(t, store, "images/test/abc123/original", 512)

	provider := &mockProvider{err: fmt.Errorf("model unavailable")}
	proc := vision.New(provider, store)

	entity := makeMediaEntity("image", "images/test/abc123/original", "image/png")
	beforeProps := len(entity.Properties)

	out := proc.Process(context.Background(), []handler.RawEntity{entity})

	// Entity should be returned unchanged — provider error is non-fatal.
	if len(out[0].Properties) != beforeProps {
		t.Errorf("provider error should leave entity properties unchanged: before=%d after=%d",
			beforeProps, len(out[0].Properties))
	}
	if hasProp(out[0], source.MediaVisionLabels) {
		t.Error("entity should not have vision properties after provider error")
	}
}

func TestProcessor_Process_MediaTypeFiltering(t *testing.T) {
	store := newMemStore()
	seedStore(t, store, "media/audio/track/original", 512)

	provider := &mockProvider{result: defaultResult()}
	// Only process "image" and "keyframe", not "audio".
	proc := vision.New(provider, store) // DefaultConfig already uses [image, keyframe]

	// "audio" type should be skipped.
	audioEntity := handler.RawEntity{
		SourceType: handler.SourceTypeAudio,
		Domain:     handler.DomainMedia,
		System:     "test",
		EntityType: "audio",
		Instance:   "track1",
		Properties: map[string]any{
			"media_type":  "audio",
			"storage_ref": "media/audio/track/original",
			"mime_type":   "audio/mp3",
		},
	}
	// "image" type should be processed.
	seedStore(t, store, "images/test/abc123/original", 512)
	imageEntity := makeMediaEntity("image", "images/test/abc123/original", "image/png")

	out := proc.Process(context.Background(), []handler.RawEntity{audioEntity, imageEntity})

	if len(out) != 2 {
		t.Fatalf("expected 2 entities out, got %d", len(out))
	}
	if hasProp(out[0], source.MediaVisionLabels) {
		t.Error("audio entity should not be vision-enriched")
	}
	if !hasProp(out[1], source.MediaVisionLabels) {
		t.Error("image entity should be vision-enriched")
	}
	if provider.calls != 1 {
		t.Errorf("provider called %d times, want 1 (only for image)", provider.calls)
	}
}

func TestProcessor_ProcessSingle_Keyframe(t *testing.T) {
	store := newMemStore()
	seedStore(t, store, "keyframes/video1/frame042/original", 1024)

	result := &vision.VisionResult{
		Labels:      []string{"car", "road", "outdoor"},
		Description: "A red car on a highway",
		Confidence:  0.88,
		Objects: []vision.DetectedObject{
			{Label: "car", Confidence: 0.90},
		},
		Text:  "",
		Model: "claude-vision-v1",
	}
	provider := &mockProvider{result: result}
	proc := vision.New(provider, store)

	entity := makeMediaEntity("keyframe", "keyframes/video1/frame042/original", "image/jpeg")
	out := proc.ProcessSingle(context.Background(), entity)

	if !hasProp(out, source.MediaVisionLabels) {
		t.Error("keyframe entity should have vision_labels property")
	}
	if !hasProp(out, source.MediaVisionDescription) {
		t.Error("keyframe entity should have vision_description property")
	}
	if !hasProp(out, source.MediaVisionModel) {
		t.Error("keyframe entity should have vision_model property")
	}

	if provider.calls != 1 {
		t.Errorf("provider called %d times, want 1", provider.calls)
	}
}

func TestProcessor_Process_OCRText(t *testing.T) {
	store := newMemStore()
	seedStore(t, store, "images/test/screenshot/original", 512)

	result := &vision.VisionResult{
		Labels:      []string{"screenshot", "ui"},
		Description: "A software screenshot showing a login form",
		Confidence:  0.85,
		Objects:     nil,
		Text:        "Username\nPassword\nLogin",
		Model:       "claude-3-5-sonnet-20241022",
	}
	provider := &mockProvider{result: result}
	proc := vision.New(provider, store)

	entity := makeMediaEntity("image", "images/test/screenshot/original", "image/png")
	out := proc.Process(context.Background(), []handler.RawEntity{entity})

	// Text property MUST be present when result.Text is non-empty.
	if !hasProp(out[0], source.MediaVisionText) {
		t.Fatal("expected vision_text property when result.Text is non-empty")
	}
	if out[0].Properties[source.MediaVisionText] != "Username\nPassword\nLogin" {
		t.Errorf("Properties[MediaVisionText] = %v, want 'Username\\nPassword\\nLogin'",
			out[0].Properties[source.MediaVisionText])
	}
}

func TestVisionResult_Empty(t *testing.T) {
	store := newMemStore()
	seedStore(t, store, "images/test/blank/original", 512)

	// Provider returns a zero-value result (empty labels, empty description, etc.)
	emptyResult := &vision.VisionResult{
		Labels:      []string{},
		Description: "",
		Confidence:  0.0,
		Objects:     nil,
		Text:        "",
		Model:       "test-model",
	}
	provider := &mockProvider{result: emptyResult}
	proc := vision.New(provider, store)

	entity := makeMediaEntity("image", "images/test/blank/original", "image/png")
	out := proc.Process(context.Background(), []handler.RawEntity{entity})

	enriched := out[0]

	// All non-text properties must still be set (even with zero values).
	for _, key := range []string{
		source.MediaVisionLabels,
		source.MediaVisionDescription,
		source.MediaVisionConfidence,
		source.MediaVisionObjects,
		source.MediaVisionModel,
	} {
		if !hasProp(enriched, key) {
			t.Errorf("expected property %s even with empty result", key)
		}
	}

	// Text property must NOT be set for empty text.
	if hasProp(enriched, source.MediaVisionText) {
		t.Error("expected no vision_text property for empty result.Text")
	}
}

func TestProcessor_Process_PreservesExistingTriples(t *testing.T) {
	store := newMemStore()
	seedStore(t, store, "images/test/abc123/original", 512)

	provider := &mockProvider{result: defaultResult()}
	proc := vision.New(provider, store)

	// Entity already has some properties from the ingest handler (e.g. mime_type).
	entity := makeMediaEntity("image", "images/test/abc123/original", "image/png")
	// makeMediaEntity already sets mime_type in Properties; record the count before enrichment.
	initialCount := len(entity.Properties)

	out := proc.Process(context.Background(), []handler.RawEntity{entity})
	finalCount := len(out[0].Properties)

	// Process must add vision properties without dropping existing ones.
	if finalCount <= initialCount {
		t.Errorf("Process should have added vision properties: before=%d after=%d", initialCount, finalCount)
	}
	// Confirm the original mime_type property survives.
	if out[0].Properties["mime_type"] != "image/png" {
		t.Errorf("original mime_type property was lost after vision enrichment: got %v",
			out[0].Properties["mime_type"])
	}
}

func TestProcessor_Process_MissingStorageKeyInStore(t *testing.T) {
	store := newMemStore()
	// Do NOT seed the store — key will be missing.

	provider := &mockProvider{result: defaultResult()}
	proc := vision.New(provider, store)

	entity := makeMediaEntity("image", "images/nonexistent/original", "image/png")
	out := proc.Process(context.Background(), []handler.RawEntity{entity})

	if hasProp(out[0], source.MediaVisionLabels) {
		t.Error("entity with missing storage key should not have vision properties")
	}
	if provider.calls != 0 {
		t.Errorf("provider should not be called when store.Get fails; got %d calls", provider.calls)
	}
}

func TestProcessor_Process_CustomMediaTypes(t *testing.T) {
	store := newMemStore()
	seedStore(t, store, "media/video1/original", 512)

	provider := &mockProvider{result: defaultResult()}
	cfg := vision.DefaultConfig()
	cfg.MediaTypes = []string{"video"} // only process video
	proc := vision.New(provider, store, vision.WithConfig(cfg))

	// "video" entity should now be processed.
	videoEntity := handler.RawEntity{
		SourceType: handler.SourceTypeVideo,
		Domain:     handler.DomainMedia,
		System:     "test",
		EntityType: "video",
		Instance:   "v1",
		Properties: map[string]any{
			"media_type":  "video",
			"storage_ref": "media/video1/original",
			"mime_type":   "video/mp4",
		},
	}
	// "image" entity should NOT be processed with this custom config.
	seedStore(t, store, "images/test/abc123/original", 512)
	imageEntity := makeMediaEntity("image", "images/test/abc123/original", "image/png")

	out := proc.Process(context.Background(), []handler.RawEntity{videoEntity, imageEntity})

	if !hasProp(out[0], source.MediaVisionLabels) {
		t.Error("video entity should be vision-enriched with custom MediaTypes config")
	}
	if hasProp(out[1], source.MediaVisionLabels) {
		t.Error("image entity should NOT be vision-enriched when MediaTypes=[video]")
	}
}
