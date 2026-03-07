package ast_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	asthandler "github.com/c360studio/semsource/handler/ast"
	"github.com/c360studio/semsource/handler"
)

// stubConfig implements handler.SourceConfig for testing.
type stubConfig struct {
	sourceType string
	path       string
	url        string
	watch      bool
	language   string
	org        string
	project    string
}

func (s *stubConfig) GetType() string            { return s.sourceType }
func (s *stubConfig) GetPath() string            { return s.path }
func (s *stubConfig) GetPaths() []string         { return nil }
func (s *stubConfig) GetURL() string             { return s.url }
func (s *stubConfig) IsWatchEnabled() bool       { return s.watch }
func (s *stubConfig) GetKeyframeMode() string    { return "" }
func (s *stubConfig) GetKeyframeInterval() string { return "" }
func (s *stubConfig) GetSceneThreshold() float64 { return 0 }

// ASTSourceConfig extends SourceConfig with ast-specific fields.
func (s *stubConfig) GetLanguage() string { return s.language }
func (s *stubConfig) GetOrg() string      { return s.org }
func (s *stubConfig) GetProject() string  { return s.project }

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testdataDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("resolve testdata dir: %v", err)
	}
	return dir
}

// --- Supports ---

func TestASTHandler_Supports(t *testing.T) {
	h := asthandler.New(newTestLogger())

	tests := []struct {
		name       string
		sourceType string
		want       bool
	}{
		{"ast type", "ast", true},
		{"git type", "git", false},
		{"doc type", "doc", false},
		{"url type", "url", false},
		{"empty type", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &stubConfig{sourceType: tt.sourceType}
			if got := h.Supports(cfg); got != tt.want {
				t.Errorf("Supports(%q) = %v, want %v", tt.sourceType, got, tt.want)
			}
		})
	}
}

func TestASTHandler_SourceType(t *testing.T) {
	h := asthandler.New(newTestLogger())
	if h.SourceType() != "ast" {
		t.Errorf("SourceType() = %q, want %q", h.SourceType(), "ast")
	}
}

// --- Ingest ---

func TestASTHandler_Ingest_GoFixture(t *testing.T) {
	h := asthandler.New(newTestLogger())
	cfg := &stubConfig{
		sourceType: "ast",
		path:       testdataDir(t),
		language:   "go",
		org:        "acme",
		project:    "testpkg",
	}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("Ingest() returned no entities from Go fixture")
	}

	// All entities must have required fields.
	for i, e := range entities {
		if e.Domain == "" {
			t.Errorf("entity[%d].Domain is empty", i)
		}
		if e.System == "" {
			t.Errorf("entity[%d].System is empty", i)
		}
		if e.EntityType == "" {
			t.Errorf("entity[%d].EntityType is empty", i)
		}
		if e.Instance == "" {
			t.Errorf("entity[%d].Instance is empty", i)
		}
		if e.SourceType != handler.SourceTypeAST {
			t.Errorf("entity[%d].SourceType = %q, want %q", i, e.SourceType, handler.SourceTypeAST)
		}
	}
}

func TestASTHandler_Ingest_ProducesFunction(t *testing.T) {
	h := asthandler.New(newTestLogger())
	cfg := &stubConfig{
		sourceType: "ast",
		path:       testdataDir(t),
		language:   "go",
		org:        "acme",
		project:    "testpkg",
	}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	var funcInstances []string
	for _, e := range entities {
		if e.EntityType == "function" || e.EntityType == "method" {
			funcInstances = append(funcInstances, e.Instance)
		}
	}
	if len(funcInstances) == 0 {
		t.Error("Ingest() produced no function or method entities from Go fixture")
	}
}

func TestASTHandler_Ingest_ProducesStruct(t *testing.T) {
	h := asthandler.New(newTestLogger())
	cfg := &stubConfig{
		sourceType: "ast",
		path:       testdataDir(t),
		language:   "go",
		org:        "acme",
		project:    "testpkg",
	}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	var structFound bool
	for _, e := range entities {
		if e.EntityType == "struct" {
			structFound = true
			break
		}
	}
	if !structFound {
		t.Error("Ingest() produced no struct entities from Go fixture")
	}
}

func TestASTHandler_Ingest_ProducesInterface(t *testing.T) {
	h := asthandler.New(newTestLogger())
	cfg := &stubConfig{
		sourceType: "ast",
		path:       testdataDir(t),
		language:   "go",
		org:        "acme",
		project:    "testpkg",
	}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	var ifaceFound bool
	for _, e := range entities {
		if e.EntityType == "interface" {
			ifaceFound = true
			break
		}
	}
	if !ifaceFound {
		t.Error("Ingest() produced no interface entities from Go fixture")
	}
}

func TestASTHandler_Ingest_DomainIsGolang(t *testing.T) {
	h := asthandler.New(newTestLogger())
	cfg := &stubConfig{
		sourceType: "ast",
		path:       testdataDir(t),
		language:   "go",
		org:        "acme",
		project:    "testpkg",
	}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}

	for i, e := range entities {
		if e.Domain != handler.DomainGolang {
			t.Errorf("entity[%d].Domain = %q, want %q", i, e.Domain, handler.DomainGolang)
		}
	}
}

func TestASTHandler_Ingest_ContextCancelled(t *testing.T) {
	h := asthandler.New(newTestLogger())
	cfg := &stubConfig{
		sourceType: "ast",
		path:       testdataDir(t),
		language:   "go",
		org:        "acme",
		project:    "testpkg",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	// Should return an error or empty result — must not hang.
	_, err := h.Ingest(ctx, cfg)
	// We don't assert specific error because a small fixture may finish before
	// the cancellation is noticed. We just check it returns promptly.
	_ = err
}

func TestASTHandler_Ingest_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	h := asthandler.New(newTestLogger())
	cfg := &stubConfig{
		sourceType: "ast",
		path:       tmp,
		language:   "go",
		org:        "acme",
		project:    "empty",
	}

	entities, err := h.Ingest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ingest() on empty dir returned error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("Ingest() on empty dir returned %d entities, want 0", len(entities))
	}
}

func TestASTHandler_Watch_NilWhenWatchDisabled(t *testing.T) {
	h := asthandler.New(newTestLogger())
	cfg := &stubConfig{
		sourceType: "ast",
		path:       testdataDir(t),
		language:   "go",
		org:        "acme",
		project:    "testpkg",
		watch:      false,
	}

	ch, err := h.Watch(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	if ch != nil {
		t.Error("Watch() should return nil channel when watch is disabled")
	}
}
