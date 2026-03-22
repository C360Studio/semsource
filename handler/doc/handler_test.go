package doc_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semsource/handler"
	dochandler "github.com/c360studio/semsource/handler/doc"
	source "github.com/c360studio/semsource/source/vocabulary"
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
	paths []string
}

func (s sourceConfig) GetType() string             { return s.typ }
func (s sourceConfig) GetPath() string             { return s.path }
func (s sourceConfig) GetPaths() []string          { return s.paths }
func (s sourceConfig) GetURL() string              { return s.url }
func (s sourceConfig) GetBranch() string           { return "" }
func (s sourceConfig) IsWatchEnabled() bool        { return s.watch }
func (s sourceConfig) GetKeyframeMode() string     { return "" }
func (s sourceConfig) GetKeyframeInterval() string { return "" }
func (s sourceConfig) GetSceneThreshold() float64  { return 0 }

// writeMD writes a markdown file and returns its absolute path.
func writeMD(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// ---------------------------------------------------------------------------
// SourceType / Supports
// ---------------------------------------------------------------------------

func TestDocHandler_SourceType(t *testing.T) {
	h := dochandler.New()
	if got := h.SourceType(); got != "docs" {
		t.Errorf("SourceType() = %q, want %q", got, "docs")
	}
}

func TestDocHandler_Supports(t *testing.T) {
	h := dochandler.New()

	tests := []struct {
		typ  string
		want bool
	}{
		{"docs", true},
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

func TestDocHandler_Ingest_ProducesEntities(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "readme.md", "# Hello World\n\nSome content here.")
	writeMD(t, dir, "notes.txt", "Plain text notes.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) != 2 {
		t.Fatalf("entity count: got %d, want 2", len(entities))
	}
}

func TestDocHandler_Ingest_CorrectDomainAndEntityType(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# Title\nBody.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities returned")
	}

	e := entities[0]
	if e.Domain != handler.DomainWeb {
		t.Errorf("Domain = %q, want %q", e.Domain, handler.DomainWeb)
	}
	if e.EntityType != "doc" {
		t.Errorf("EntityType = %q, want %q", e.EntityType, "doc")
	}
	if e.SourceType != handler.SourceTypeDoc {
		t.Errorf("SourceType = %q, want %q", e.SourceType, handler.SourceTypeDoc)
	}
}

func TestDocHandler_Ingest_InstanceIsSHA256Prefix(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# Title\nContent.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

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
	// Must be hex characters only.
	for _, ch := range instance {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			t.Errorf("Instance %q contains non-hex character %q", instance, ch)
		}
	}
}

func TestDocHandler_Ingest_TripleContentHash(t *testing.T) {
	dir := t.TempDir()
	content := "# Hello\nThis is content."
	writeMD(t, dir, "doc.md", content)

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	// Expect a Properties entry for "content_hash".
	if _, ok := entities[0].Properties["content_hash"]; !ok {
		t.Error("expected Properties[\"content_hash\"] to be set")
	}
}

func TestDocHandler_Ingest_TripleFilePath(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "readme.md", "# Hi\nBody.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	if _, ok := entities[0].Properties["file_path"]; !ok {
		t.Error("expected Properties[\"file_path\"] to be set")
	}
}

func TestDocHandler_Ingest_TripleTitleFromFirstHeading(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# My Great Doc\n\nIntro paragraph.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	titleValue, ok := entities[0].Properties["title"]
	if !ok {
		t.Fatal("expected Properties[\"title\"] to be set, got none")
	}
	if titleValue != "My Great Doc" {
		t.Errorf("title = %q, want %q", titleValue, "My Great Doc")
	}
}

func TestDocHandler_Ingest_NoTitleFallback(t *testing.T) {
	dir := t.TempDir()
	// Plain text file with no markdown heading.
	writeMD(t, dir, "notes.txt", "Just some notes without a heading.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no entities")
	}

	// Properties["title"] should still be present, falling back to filename.
	if _, ok := entities[0].Properties["title"]; !ok {
		t.Error("expected Properties[\"title\"] even without markdown heading")
	}
}

func TestDocHandler_Ingest_ContentHashChangesOnModify(t *testing.T) {
	dir := t.TempDir()
	path := writeMD(t, dir, "doc.md", "# First\nOriginal content.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	entities1, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() first: %v", err)
	}

	// Modify the file.
	if err := os.WriteFile(path, []byte("# Second\nDifferent content entirely."), 0644); err != nil {
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

func TestDocHandler_Ingest_FiltersByExtension(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "valid.md", "# Doc\nContent.")
	writeMD(t, dir, "guide.mdx", "# MDX Guide\nSome MDX content.")
	writeMD(t, dir, "spec.adoc", "= AsciiDoc Spec\nSome AsciiDoc content.")
	// Non-doc file — should be ignored by DocHandler.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) != 3 {
		t.Errorf("entity count: got %d, want 3 (.adoc + .md + .mdx)", len(entities))
	}
}

func TestDocHandler_Ingest_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("entity count: got %d, want 0 for empty dir", len(entities))
	}
}

// ---------------------------------------------------------------------------
// Ingest — multiple paths
// ---------------------------------------------------------------------------

func TestDocHandler_Ingest_MultiplePaths(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	writeMD(t, dirA, "alpha.md", "# Alpha\nContent in dirA.")
	writeMD(t, dirA, "beta.txt", "Plain text in dirA.")
	writeMD(t, dirB, "gamma.md", "# Gamma\nContent in dirB.")

	h := dochandler.New()
	// GetPath returns paths[0] when paths is non-empty; GetPaths returns both dirs.
	cfg := sourceConfig{
		typ:   "docs",
		path:  dirA,
		paths: []string{dirA, dirB},
	}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}

	// Three files total: two in dirA, one in dirB.
	if len(entities) != 3 {
		t.Fatalf("entity count: got %d, want 3", len(entities))
	}

	// Collect relative file_path values across all entities to confirm both
	// directories contributed files.
	seenPaths := make(map[string]bool)
	for _, e := range entities {
		if fp, ok := e.Properties["file_path"].(string); ok {
			seenPaths[fp] = true
		}
	}

	for _, want := range []string{"alpha.md", "beta.txt", "gamma.md"} {
		if !seenPaths[want] {
			t.Errorf("expected file_path %q in Properties, not found; seen: %v", want, seenPaths)
		}
	}
}

// ---------------------------------------------------------------------------
// Watch — disabled returns nil (no fsnotify needed)
// ---------------------------------------------------------------------------

func TestDocHandler_Watch_WatchDisabledReturnsNil(t *testing.T) {
	dir := t.TempDir()

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir, watch: false}

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
// IngestEntityStates — normalizer-free path
// ---------------------------------------------------------------------------

func TestDocHandler_IngestEntityStates_ReturnsStates(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "readme.md", "# Hello\n\nContent here.")
	writeMD(t, dir, "notes.txt", "Plain text.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("state count: got %d, want 2", len(states))
	}
}

func TestDocHandler_IngestEntityStates_IDHasSixParts(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# Title\nBody.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) == 0 {
		t.Fatal("no states returned")
	}

	parts := strings.Split(states[0].ID, ".")
	if len(parts) != 6 {
		t.Errorf("entity ID has %d parts, want 6: %q", len(parts), states[0].ID)
	}
	if parts[0] != "acme" {
		t.Errorf("ID org segment = %q, want %q", parts[0], "acme")
	}
	if parts[2] != "web" {
		t.Errorf("ID domain segment = %q, want %q", parts[2], "web")
	}
	if parts[4] != "doc" {
		t.Errorf("ID type segment = %q, want %q", parts[4], "doc")
	}
}

func TestDocHandler_IngestEntityStates_TriplesUseVocabularyPredicates(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# My Doc\nSome content.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

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

	wantPredicates := []string{
		source.DocType,
		source.DocFilePath,
		source.DocMimeType,
		source.DocFileHash,
		source.DocContent,
		source.DocSummary,
	}
	for _, p := range wantPredicates {
		if !predicates[p] {
			t.Errorf("missing triple with predicate %q", p)
		}
	}
}

func TestDocHandler_IngestEntityStates_DeterministicID(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# Title\nContent.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	states1, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("first IngestEntityStates() error: %v", err)
	}
	states2, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("second IngestEntityStates() error: %v", err)
	}
	if len(states1) == 0 || len(states2) == 0 {
		t.Fatal("expected states from both calls")
	}
	if states1[0].ID != states2[0].ID {
		t.Errorf("entity ID not deterministic: %q vs %q", states1[0].ID, states2[0].ID)
	}
}

func TestDocHandler_IngestEntityStates_IDChangesOnContentChange(t *testing.T) {
	dir := t.TempDir()
	path := writeMD(t, dir, "doc.md", "# First\nOriginal content.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	states1, _ := h.IngestEntityStates(context.Background(), cfg, "acme")

	if err := os.WriteFile(path, []byte("# Second\nDifferent content."), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	states2, _ := h.IngestEntityStates(context.Background(), cfg, "acme")

	if len(states1) == 0 || len(states2) == 0 {
		t.Fatal("expected states from both ingests")
	}
	// Instance (sha256 prefix in ID) must change when content changes.
	if states1[0].ID == states2[0].ID {
		t.Error("entity ID should change when file content changes")
	}
}

func TestDocHandler_IngestEntityStates_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("state count: got %d, want 0 for empty dir", len(states))
	}
}

func TestDocHandler_IngestEntityStates_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "doc.md", "# Title\nContent.")

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err := h.IngestEntityStates(ctx, cfg, "acme")
	if err == nil {
		t.Error("expected error on cancelled context")
	}
}

// ---------------------------------------------------------------------------
// ObjectStore content threshold
// ---------------------------------------------------------------------------

// memStore is an in-memory storage.Store for testing.
type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStore() *memStore { return &memStore{data: make(map[string][]byte)} }

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
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}

func TestDocHandler_IngestEntityStates_LargeDocGoesToStore(t *testing.T) {
	dir := t.TempDir()
	// Create a doc larger than the 100-byte threshold we'll set.
	largeContent := "# Large Doc\n" + strings.Repeat("word ", 200)
	writeMD(t, dir, "large.md", largeContent)

	store := newMemStore()
	h := dochandler.NewWithOrg("acme",
		dochandler.WithStore(store, "MESSAGES"),
		dochandler.WithContentThreshold(100),
	)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("state count: got %d, want 1", len(states))
	}

	state := states[0]

	// StorageRef should be set.
	if state.StorageRef == nil {
		t.Fatal("StorageRef should be set for large doc")
	}
	if state.StorageRef.StorageInstance != "MESSAGES" {
		t.Errorf("StorageInstance: got %q, want MESSAGES", state.StorageRef.StorageInstance)
	}

	// DocContent triple should be absent.
	for _, triple := range state.Triples {
		if triple.Predicate == source.DocContent {
			t.Error("DocContent triple should be absent when content is in ObjectStore")
		}
	}

	// Metadata triples should still be present.
	hasHash := false
	for _, triple := range state.Triples {
		if triple.Predicate == source.DocFileHash {
			hasHash = true
		}
	}
	if !hasHash {
		t.Error("DocFileHash triple should still be present")
	}

	// Store should contain the raw content.
	storedKeys := store.keys()
	if len(storedKeys) != 1 {
		t.Fatalf("store key count: got %d, want 1", len(storedKeys))
	}
	stored, _ := store.Get(context.Background(), storedKeys[0])
	if string(stored) != largeContent {
		t.Error("stored content does not match original")
	}
}

func TestDocHandler_IngestEntityStates_SmallDocStaysInline(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "small.md", "# Small\nTiny doc.")

	store := newMemStore()
	h := dochandler.NewWithOrg("acme",
		dochandler.WithStore(store, "MESSAGES"),
		dochandler.WithContentThreshold(4096),
	)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("state count: got %d, want 1", len(states))
	}

	state := states[0]

	// StorageRef should be nil.
	if state.StorageRef != nil {
		t.Error("StorageRef should be nil for small doc")
	}

	// DocContent triple should be present.
	hasContent := false
	for _, triple := range state.Triples {
		if triple.Predicate == source.DocContent {
			hasContent = true
		}
	}
	if !hasContent {
		t.Error("DocContent triple should be present for small doc")
	}

	// Store should be empty.
	if len(store.keys()) != 0 {
		t.Error("store should be empty for small doc")
	}
}

func TestDocHandler_IngestEntityStates_StoreFailureFallsBackToInline(t *testing.T) {
	dir := t.TempDir()
	largeContent := "# Large Doc\n" + strings.Repeat("word ", 200)
	writeMD(t, dir, "large.md", largeContent)

	store := &failStore{}
	h := dochandler.NewWithOrg("acme",
		dochandler.WithStore(store, "MESSAGES"),
		dochandler.WithContentThreshold(100),
	)
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("state count: got %d, want 1", len(states))
	}

	state := states[0]

	// StorageRef should be nil because store.Put failed.
	if state.StorageRef != nil {
		t.Error("StorageRef should be nil when store fails")
	}

	// DocContent triple should be present (fallback to inline).
	hasContent := false
	for _, triple := range state.Triples {
		if triple.Predicate == source.DocContent {
			hasContent = true
		}
	}
	if !hasContent {
		t.Error("DocContent triple should be present when store fails")
	}
}

// failStore always returns an error from Put.
type failStore struct{}

func (s *failStore) Put(context.Context, string, []byte) error {
	return fmt.Errorf("simulated store failure")
}
func (s *failStore) Get(context.Context, string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *failStore) List(context.Context, string) ([]string, error) { return nil, nil }
func (s *failStore) Delete(context.Context, string) error           { return nil }

func TestDocHandler_IngestEntityStates_NoStoreAllInline(t *testing.T) {
	dir := t.TempDir()
	largeContent := "# Large Doc\n" + strings.Repeat("word ", 200)
	writeMD(t, dir, "large.md", largeContent)

	// No store configured — all content stays inline regardless of size.
	h := dochandler.NewWithOrg("acme")
	cfg := sourceConfig{typ: "docs", path: dir}

	states, err := h.IngestEntityStates(context.Background(), cfg, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("state count: got %d, want 1", len(states))
	}

	state := states[0]

	if state.StorageRef != nil {
		t.Error("StorageRef should be nil when no store is configured")
	}

	hasContent := false
	for _, triple := range state.Triples {
		if triple.Predicate == source.DocContent {
			hasContent = true
		}
	}
	if !hasContent {
		t.Error("DocContent triple should be present when no store is configured")
	}
}
