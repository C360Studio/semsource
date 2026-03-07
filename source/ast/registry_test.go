package ast

import (
	"context"
	"sync"
	"testing"
)

// mockParser implements FileParser for testing
type mockParser struct {
	org      string
	project  string
	repoRoot string
}

func (m *mockParser) ParseFile(_ context.Context, filePath string) (*ParseResult, error) {
	return &ParseResult{Path: filePath}, nil
}

func newMockFactory(org, project, repoRoot string) FileParser {
	return &mockParser{org: org, project: project, repoRoot: repoRoot}
}

func TestParserRegistry_Register(t *testing.T) {
	registry := NewParserRegistry()

	registry.Register("test", []string{".test", ".tst"}, newMockFactory)

	if !registry.HasParser("test") {
		t.Error("expected parser 'test' to be registered")
	}

	parsers := registry.ListParsers()
	if len(parsers) != 1 || parsers[0] != "test" {
		t.Errorf("expected [test], got %v", parsers)
	}
}

func TestParserRegistry_GetParserName(t *testing.T) {
	registry := NewParserRegistry()
	registry.Register("test", []string{".test", ".tst"}, newMockFactory)

	tests := []struct {
		ext      string
		wantName string
		wantOK   bool
	}{
		{".test", "test", true},
		{".tst", "test", true},
		{".unknown", "", false},
	}

	for _, tc := range tests {
		name, ok := registry.GetParserName(tc.ext)
		if ok != tc.wantOK {
			t.Errorf("GetParserName(%q): got ok=%v, want ok=%v", tc.ext, ok, tc.wantOK)
		}
		if name != tc.wantName {
			t.Errorf("GetParserName(%q): got name=%q, want name=%q", tc.ext, name, tc.wantName)
		}
	}
}

func TestParserRegistry_CreateParser(t *testing.T) {
	registry := NewParserRegistry()
	registry.Register("test", []string{".test"}, newMockFactory)

	parser, err := registry.CreateParser("test", "myorg", "myproject", "/repo")
	if err != nil {
		t.Fatalf("CreateParser failed: %v", err)
	}

	mock, ok := parser.(*mockParser)
	if !ok {
		t.Fatal("expected *mockParser")
	}

	if mock.org != "myorg" || mock.project != "myproject" || mock.repoRoot != "/repo" {
		t.Errorf("factory received wrong args: org=%q, project=%q, root=%q",
			mock.org, mock.project, mock.repoRoot)
	}
}

func TestParserRegistry_CreateParser_NotRegistered(t *testing.T) {
	registry := NewParserRegistry()

	_, err := registry.CreateParser("nonexistent", "org", "proj", "/")
	if err == nil {
		t.Error("expected error for unregistered parser")
	}
}

func TestParserRegistry_CreateParserForExtension(t *testing.T) {
	registry := NewParserRegistry()
	registry.Register("test", []string{".test"}, newMockFactory)

	parser, err := registry.CreateParserForExtension(".test", "org", "proj", "/")
	if err != nil {
		t.Fatalf("CreateParserForExtension failed: %v", err)
	}
	if parser == nil {
		t.Error("expected non-nil parser")
	}

	_, err = registry.CreateParserForExtension(".unknown", "org", "proj", "/")
	if err == nil {
		t.Error("expected error for unknown extension")
	}
}

func TestParserRegistry_FirstRegistrationWins(t *testing.T) {
	registry := NewParserRegistry()

	// Register first parser for .ext
	registry.Register("first", []string{".ext"}, func(org, _, _ string) FileParser {
		return &mockParser{org: org}
	})

	// Try to register second parser for same extension
	registry.Register("second", []string{".ext"}, func(org, _, _ string) FileParser {
		return &mockParser{org: org}
	})

	// Extension should still map to first parser
	name, _ := registry.GetParserName(".ext")
	if name != "first" {
		t.Errorf("expected extension to map to 'first', got %q", name)
	}

	// But both parsers should be registered
	if !registry.HasParser("first") || !registry.HasParser("second") {
		t.Error("both parsers should be registered")
	}
}

func TestParserRegistry_ListExtensions(t *testing.T) {
	registry := NewParserRegistry()
	registry.Register("parser1", []string{".a", ".b"}, newMockFactory)
	registry.Register("parser2", []string{".c"}, newMockFactory)

	exts := registry.ListExtensions()
	if len(exts) != 3 {
		t.Errorf("expected 3 extensions, got %d", len(exts))
	}

	extSet := make(map[string]bool)
	for _, ext := range exts {
		extSet[ext] = true
	}

	for _, want := range []string{".a", ".b", ".c"} {
		if !extSet[want] {
			t.Errorf("expected extension %q in list", want)
		}
	}
}

func TestParserRegistry_GetExtensionsForParser(t *testing.T) {
	registry := NewParserRegistry()
	registry.Register("multi", []string{".x", ".y", ".z"}, newMockFactory)

	exts := registry.GetExtensionsForParser("multi")
	if len(exts) != 3 {
		t.Errorf("expected 3 extensions, got %d", len(exts))
	}

	exts = registry.GetExtensionsForParser("nonexistent")
	if len(exts) != 0 {
		t.Errorf("expected 0 extensions for nonexistent parser, got %d", len(exts))
	}
}

func TestParserRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewParserRegistry()
	var wg sync.WaitGroup

	// Concurrent registrations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			registry.Register("parser"+string(rune('A'+i)), []string{"." + string(rune('a'+i))}, newMockFactory)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry.ListParsers()
			registry.ListExtensions()
		}()
	}

	wg.Wait()

	// Should have 10 parsers
	parsers := registry.ListParsers()
	if len(parsers) != 10 {
		t.Errorf("expected 10 parsers, got %d", len(parsers))
	}
}

// Note: Tests for Go/TS/JS parser registration are in
// processor/ast-indexer/registry_integration_test.go because
// we can't import language packages here without causing import cycles.
