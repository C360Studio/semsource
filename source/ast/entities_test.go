package ast

import (
	"strings"
	"testing"
	"time"
)

func TestNewCodeEntity(t *testing.T) {
	entity := NewCodeEntity("acme", "myproject", TypeFunction, "Foo", "pkg/foo.go")

	if entity.Type != TypeFunction {
		t.Errorf("Type = %q, want %q", entity.Type, TypeFunction)
	}
	if entity.Name != "Foo" {
		t.Errorf("Name = %q, want %q", entity.Name, "Foo")
	}
	if entity.Path != "pkg/foo.go" {
		t.Errorf("Path = %q, want %q", entity.Path, "pkg/foo.go")
	}
	if entity.Visibility != VisibilityPublic {
		t.Errorf("Visibility = %q, want %q", entity.Visibility, VisibilityPublic)
	}
	if entity.IndexedAt.IsZero() {
		t.Error("IndexedAt should not be zero")
	}

	// Check entity ID format
	expectedPrefix := "acme.semspec.code.function.myproject."
	if !strings.HasPrefix(entity.ID, expectedPrefix) {
		t.Errorf("ID = %q, want prefix %q", entity.ID, expectedPrefix)
	}
}

func TestNewCodeEntity_PrivateVisibility(t *testing.T) {
	entity := NewCodeEntity("acme", "myproject", TypeFunction, "foo", "pkg/foo.go")

	if entity.Visibility != VisibilityPrivate {
		t.Errorf("Visibility = %q, want %q", entity.Visibility, VisibilityPrivate)
	}
}

func TestNewCodeEntity_FileType(t *testing.T) {
	entity := NewCodeEntity("acme", "myproject", TypeFile, "foo.go", "pkg/foo.go")

	// File entities don't append name to instance ID
	if !strings.Contains(entity.ID, "pkg-foo-go") {
		t.Errorf("ID = %q, want to contain 'pkg-foo-go'", entity.ID)
	}
}

func TestDetermineVisibility(t *testing.T) {
	tests := []struct {
		name     string
		expected Visibility
	}{
		{"Foo", VisibilityPublic},
		{"foo", VisibilityPrivate},
		{"FOO", VisibilityPublic},
		{"_foo", VisibilityPrivate},
		{"", VisibilityPrivate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineVisibility(tt.name)
			if result != tt.expected {
				t.Errorf("determineVisibility(%q) = %q, want %q", tt.name, result, tt.expected)
			}
		})
	}
}

func TestComputeHash(t *testing.T) {
	content := []byte("package main\n\nfunc main() {}\n")
	hash := ComputeHash(content)

	if hash == "" {
		t.Error("hash is empty")
	}
	if len(hash) != 16 { // 8 bytes = 16 hex chars
		t.Errorf("hash length = %d, want 16", len(hash))
	}

	// Same content should produce same hash
	hash2 := ComputeHash(content)
	if hash != hash2 {
		t.Errorf("hash not deterministic: %q != %q", hash, hash2)
	}

	// Different content should produce different hash
	content2 := []byte("package main\n\nfunc main() { fmt.Println(\"hi\") }\n")
	hash3 := ComputeHash(content2)
	if hash == hash3 {
		t.Error("different content produced same hash")
	}
}

func TestCodeEntity_Triples(t *testing.T) {
	entity := NewCodeEntity("acme", "myproject", TypeFunction, "Foo", "pkg/foo.go")
	entity.Package = "pkg"
	entity.Hash = "abc123"
	entity.StartLine = 10
	entity.EndLine = 20
	entity.DocComment = "Foo does something."
	entity.ContainedBy = "acme.semspec.code.file.myproject.pkg-foo-go"
	entity.Calls = []string{"helper", "fmt.Println"}
	entity.Returns = []string{"error"}

	triples := entity.Triples()

	// Check for required predicates
	predicateMap := make(map[string]interface{})
	for _, triple := range triples {
		predicateMap[triple.Predicate] = triple.Object
	}

	requiredPredicates := []string{
		CodeType,
		DcTitle,
		CodePath,
		CodePackage,
		CodeHash,
		CodeLanguage,
		CodeVisibility,
		CodeStartLine,
		CodeEndLine,
		CodeLines,
		CodeDocComment,
		CodeBelongsTo,
		DcCreated,
	}

	for _, pred := range requiredPredicates {
		if _, ok := predicateMap[pred]; !ok {
			t.Errorf("missing predicate %q", pred)
		}
	}

	// Check specific values
	if predicateMap[CodeType] != string(TypeFunction) {
		t.Errorf("CodeType = %v, want %q", predicateMap[CodeType], string(TypeFunction))
	}
	if predicateMap[DcTitle] != "Foo" {
		t.Errorf("DcTitle = %v, want %q", predicateMap[DcTitle], "Foo")
	}
	if predicateMap[CodeLanguage] != "go" {
		t.Errorf("CodeLanguage = %v, want %q", predicateMap[CodeLanguage], "go")
	}
	if predicateMap[CodeLines] != 11 {
		t.Errorf("CodeLines = %v, want 11", predicateMap[CodeLines])
	}

	// Check relationship triples
	callCount := 0
	returnCount := 0
	for _, triple := range triples {
		if triple.Predicate == CodeCalls {
			callCount++
		}
		if triple.Predicate == CodeReturns {
			returnCount++
		}
	}
	if callCount != 2 {
		t.Errorf("CodeCalls triples = %d, want 2", callCount)
	}
	if returnCount != 1 {
		t.Errorf("CodeReturns triples = %d, want 1", returnCount)
	}
}

func TestCodeEntity_EntityState(t *testing.T) {
	entity := NewCodeEntity("acme", "myproject", TypeStruct, "User", "pkg/user.go")
	entity.Package = "pkg"
	entity.DocComment = "User represents a user."

	state := entity.EntityState()

	if state.ID != entity.ID {
		t.Errorf("state.ID = %q, want %q", state.ID, entity.ID)
	}
	if len(state.Triples) == 0 {
		t.Error("state.Triples is empty")
	}
	if state.UpdatedAt.IsZero() {
		t.Error("state.UpdatedAt should not be zero")
	}
}

func TestParseResult_AllTriples(t *testing.T) {
	result := &ParseResult{
		Entities: []*CodeEntity{
			NewCodeEntity("acme", "test", TypeFile, "foo.go", "foo.go"),
			NewCodeEntity("acme", "test", TypeFunction, "Foo", "foo.go"),
			NewCodeEntity("acme", "test", TypeStruct, "Bar", "foo.go"),
		},
	}

	triples := result.AllTriples()
	if len(triples) == 0 {
		t.Error("AllTriples returned empty")
	}

	// Each entity produces multiple triples
	if len(triples) < 3 {
		t.Errorf("AllTriples count = %d, want at least 3", len(triples))
	}
}

func TestParseResult_AllEntityStates(t *testing.T) {
	result := &ParseResult{
		Entities: []*CodeEntity{
			NewCodeEntity("acme", "test", TypeFile, "foo.go", "foo.go"),
			NewCodeEntity("acme", "test", TypeFunction, "Foo", "foo.go"),
		},
	}

	states := result.AllEntityStates()
	if len(states) != 2 {
		t.Errorf("AllEntityStates count = %d, want 2", len(states))
	}
}

func TestBuildInstanceID(t *testing.T) {
	tests := []struct {
		path         string
		name         string
		entityType   CodeEntityType
		wantContains string
	}{
		{"pkg/foo.go", "Foo", TypeFunction, "pkg-foo-go-Foo"},
		{"internal/util.go", "Helper", TypeFunction, "internal-util-go-Helper"},
		{"main.go", "main.go", TypeFile, "main-go"},
		{"./foo.go", "foo.go", TypeFile, "foo-go"},
	}

	for _, tt := range tests {
		t.Run(tt.path+"/"+tt.name, func(t *testing.T) {
			result := BuildInstanceID(tt.path, tt.name, tt.entityType)
			if !strings.Contains(result, tt.wantContains) {
				t.Errorf("BuildInstanceID(%q, %q, %v) = %q, want to contain %q",
					tt.path, tt.name, tt.entityType, result, tt.wantContains)
			}
		})
	}
}

func TestEntityState_Fields(t *testing.T) {
	now := time.Now()
	state := &EntityState{
		ID:        "acme.semspec.code.function.test.foo",
		UpdatedAt: now,
	}

	if state.ID != "acme.semspec.code.function.test.foo" {
		t.Errorf("ID = %q, want %q", state.ID, "acme.semspec.code.function.test.foo")
	}
	if !state.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", state.UpdatedAt, now)
	}
}

func TestCodeEntity_MethodWithReceiver(t *testing.T) {
	entity := NewCodeEntity("acme", "test", TypeMethod, "String", "user.go")
	entity.Receiver = "User"

	triples := entity.Triples()

	var hasReceiver bool
	for _, triple := range triples {
		if triple.Predicate == CodeReceiver && triple.Object == "User" {
			hasReceiver = true
			break
		}
	}
	if !hasReceiver {
		t.Error("method should have CodeReceiver triple")
	}
}

func TestCodeEntity_StructWithEmbeds(t *testing.T) {
	entity := NewCodeEntity("acme", "test", TypeStruct, "Derived", "types.go")
	entity.Embeds = []string{"Base", "io.Reader"}
	entity.References = []string{"string", "int"}

	triples := entity.Triples()

	embedCount := 0
	refCount := 0
	for _, triple := range triples {
		if triple.Predicate == CodeEmbeds {
			embedCount++
		}
		if triple.Predicate == CodeReferences {
			refCount++
		}
	}

	if embedCount != 2 {
		t.Errorf("embed triples = %d, want 2", embedCount)
	}
	if refCount != 2 {
		t.Errorf("reference triples = %d, want 2", refCount)
	}
}

func TestCodeEntity_FileWithContains(t *testing.T) {
	entity := NewCodeEntity("acme", "test", TypeFile, "main.go", "main.go")
	entity.Contains = []string{
		"acme.semspec.code.function.test.main-go-main",
		"acme.semspec.code.function.test.main-go-helper",
	}
	entity.Imports = []string{"fmt", "context"}

	triples := entity.Triples()

	containsCount := 0
	importsCount := 0
	for _, triple := range triples {
		if triple.Predicate == CodeContains {
			containsCount++
		}
		if triple.Predicate == CodeImports {
			importsCount++
		}
	}

	if containsCount != 2 {
		t.Errorf("contains triples = %d, want 2", containsCount)
	}
	if importsCount != 2 {
		t.Errorf("imports triples = %d, want 2", importsCount)
	}
}
