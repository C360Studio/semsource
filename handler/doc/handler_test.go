package doc_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/handler"
	dochandler "github.com/c360studio/semsource/handler/doc"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sourceConfig adapts test parameters to handler.SourceConfig.
type sourceConfig struct {
	typ     string
	path    string
	url     string
	watch   bool
	paths   []string
}

func (s sourceConfig) GetType() string        { return s.typ }
func (s sourceConfig) GetPath() string        { return s.path }
func (s sourceConfig) GetURL() string         { return s.url }
func (s sourceConfig) IsWatchEnabled() bool   { return s.watch }

// writeMD writes a markdown file and returns its absolute path.
func writeMD(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// findTriple returns the first triple whose predicate matches pred, or nil.
func findTriple(triples interface{ GetTriples() []interface{ GetPredicate() string; GetObject() interface{} } }, pred string) interface{} {
	return nil // unused — we use the concrete type below
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

	// Expect a triple with predicate containing "content_hash".
	found := false
	for _, tr := range entities[0].Triples {
		if strings.Contains(tr.Predicate, "content_hash") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected triple with predicate containing 'content_hash'")
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

	found := false
	for _, tr := range entities[0].Triples {
		if strings.Contains(tr.Predicate, "file_path") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected triple with predicate containing 'file_path'")
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

	var titleValue interface{}
	for _, tr := range entities[0].Triples {
		if strings.Contains(tr.Predicate, "title") {
			titleValue = tr.Object
			break
		}
	}
	if titleValue == nil {
		t.Fatal("expected a title triple, got none")
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

	// Title triple should still be present, falling back to filename.
	found := false
	for _, tr := range entities[0].Triples {
		if strings.Contains(tr.Predicate, "title") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected title triple even without markdown heading")
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
	if len(entities) != 1 {
		t.Errorf("entity count: got %d, want 1 (only .md)", len(entities))
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
// Watch
// ---------------------------------------------------------------------------

func TestDocHandler_Watch_ReturnsChannel(t *testing.T) {
	dir := t.TempDir()

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir, watch: true}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := h.Watch(ctx, cfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
	if ch == nil {
		t.Fatal("Watch() returned nil channel")
	}
}

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

func TestDocHandler_Watch_EmitsOnCreate(t *testing.T) {
	dir := t.TempDir()

	h := dochandler.New()
	cfg := sourceConfig{typ: "docs", path: dir, watch: true}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := h.Watch(ctx, cfg)
	if err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	time.Sleep(30 * time.Millisecond) // let watcher register

	writeMD(t, dir, "new.md", "# New\nContent.")

	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		if ev.Operation != handler.OperationCreate {
			t.Errorf("Operation = %q, want %q", ev.Operation, handler.OperationCreate)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for watch event")
	}
}
