package ts

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	semtypes "github.com/c360studio/semstreams/pkg/types"

	"github.com/c360studio/semsource/source/ast"
)

// parseJS parses a JavaScript snippet and returns the non-file entities,
// keyed by name, plus the ordered slice for count assertions.
func parseJS(t *testing.T, src string) (map[string]*ast.CodeEntity, []*ast.CodeEntity) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.js")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewParser("test-org", "test-project", dir).ParseFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	byName := make(map[string]*ast.CodeEntity)
	var got []*ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeFile {
			continue
		}
		byName[e.Name] = e
		got = append(got, e)
	}
	return byName, got
}

func assertNames(t *testing.T, got []*ast.CodeEntity, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		var names []string
		for _, e := range got {
			names = append(names, e.Name)
		}
		t.Fatalf("got %d entities %v, want %d %v", len(got), names, len(want), want)
	}
	seen := make(map[string]bool, len(got))
	for _, e := range got {
		seen[e.Name] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("missing entity %q", w)
		}
	}
}

// TestDestructuring_OneEntityPerBinding is the core defect: the parser used the
// entire destructuring pattern as a single entity name, so `const {a, b, c}`
// became one entity called "{ a, b, c }" rather than three consts. That is both
// wrong modeling (three bindings are three symbols, none of them searchable by
// name) and the source of the pathological entity IDs that overflowed the
// graph's byte budget.
func TestDestructuring_OneEntityPerBinding(t *testing.T) {
	_, got := parseJS(t, "const { alpha, beta, gamma } = require('x');\n")
	assertNames(t, got, "alpha", "beta", "gamma")
}

// TestDestructuring_RenamedUsesLocalName pins the binding semantics that a
// naive walk gets backwards: in `{ renamed: localName }` the symbol introduced
// into scope is localName. The property key names a field on the source object,
// not anything declared here.
func TestDestructuring_RenamedUsesLocalName(t *testing.T) {
	byName, got := parseJS(t, "const { renamed: localName } = obj;\n")
	assertNames(t, got, "localName")
	if _, ok := byName["renamed"]; ok {
		t.Error("property key was emitted as an entity; only the local binding is declared here")
	}
}

func TestDestructuring_PatternShapes(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "nested object pattern",
			src:  "const { outer: { inner } } = obj;\n",
			want: []string{"inner"},
		},
		{
			name: "default value is not part of the name",
			src:  "const { withDefault = 5 } = opts;\n",
			want: []string{"withDefault"},
		},
		{
			name: "rest element binds a name",
			src:  "const { kept, ...rest } = obj;\n",
			want: []string{"kept", "rest"},
		},
		{
			name: "array pattern",
			src:  "const [first, second] = arr;\n",
			want: []string{"first", "second"},
		},
		{
			name: "array pattern with hole, default and rest",
			src:  "const [head, , third = 2, ...tail] = arr;\n",
			want: []string{"head", "third", "tail"},
		},
		{
			name: "array nested in object",
			src:  "const { items: [only] } = obj;\n",
			want: []string{"only"},
		},
		{
			name: "plain declaration is unaffected",
			src:  "const plain = 1;\n",
			want: []string{"plain"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := parseJS(t, tt.src)
			assertNames(t, got, tt.want...)
		})
	}
}

// TestDestructuring_KindIsPreserved keeps the const/var distinction per binding:
// destructuring does not change whether the declaration is mutable.
func TestDestructuring_KindIsPreserved(t *testing.T) {
	byName, _ := parseJS(t, "const { c1 } = a;\nlet { v1 } = b;\n")
	if got := byName["c1"]; got == nil || got.Type != ast.TypeConst {
		t.Errorf("const binding: got %v, want %v", entityType(got), ast.TypeConst)
	}
	if got := byName["v1"]; got == nil || got.Type != ast.TypeVar {
		t.Errorf("let binding: got %v, want %v", entityType(got), ast.TypeVar)
	}
}

// TestDestructuring_BindingsGetDistinctValidIDs guards the two ID properties
// that matter once one declaration yields several entities: each binding needs
// its own ID (otherwise siblings overwrite each other in the graph), and every
// ID must satisfy the graph contract.
func TestDestructuring_BindingsGetDistinctValidIDs(t *testing.T) {
	_, got := parseJS(t, "const { alpha, beta, gamma } = require('x');\n")
	seen := make(map[string]string, len(got))
	for _, e := range got {
		if prev, dup := seen[e.ID]; dup {
			t.Errorf("bindings %q and %q share ID %q", prev, e.Name, e.ID)
		}
		seen[e.ID] = e.Name
		if err := semtypes.ValidateEntityID(e.ID); err != nil {
			t.Errorf("binding %q has an invalid ID %q: %v", e.Name, e.ID, err)
		}
	}
}

// TestDestructuring_LongPatternStaysReadable is the tie back to the ID-length
// work: the pathological IDs came from patterns being used as names. With one
// entity per binding, a wide destructure yields short, meaningful IDs instead
// of one truncated hash-suffixed blob.
func TestDestructuring_LongPatternStaysReadable(t *testing.T) {
	_, got := parseJS(t, "const { cliConfigArray, configArrayFactory, finalizeCache, "+
		"loadConfigFile, normalizeOptions, resolveExtends, translateESLintRC } = require('x');\n")

	assertNames(t, got, "cliConfigArray", "configArrayFactory", "finalizeCache",
		"loadConfigFile", "normalizeOptions", "resolveExtends", "translateESLintRC")
	for _, e := range got {
		if len(e.ID) > 120 {
			t.Errorf("binding %q produced a %d-byte ID; per-binding IDs should stay compact: %q",
				e.Name, len(e.ID), e.ID)
		}
	}
}

func entityType(e *ast.CodeEntity) any {
	if e == nil {
		return "<missing>"
	}
	return e.Type
}
