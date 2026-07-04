package ast

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	semvocab "github.com/c360studio/semstreams/vocabulary"
)

func TestNewCodeEntity(t *testing.T) {
	entity := NewCodeEntity("acme", "golang", "myproject", TypeFunction, "Foo", "pkg/foo.go")

	if entity.Type != TypeFunction {
		t.Errorf("Type = %q, want %q", entity.Type, TypeFunction)
	}
	if entity.Name != "Foo" {
		t.Errorf("Name = %q, want %q", entity.Name, "Foo")
	}
	if entity.Path != "pkg/foo.go" {
		t.Errorf("Path = %q, want %q", entity.Path, "pkg/foo.go")
	}
	if entity.Language != "golang" {
		t.Errorf("Language = %q, want %q", entity.Language, "golang")
	}
	if entity.Visibility != VisibilityPublic {
		t.Errorf("Visibility = %q, want %q", entity.Visibility, VisibilityPublic)
	}
	if entity.IndexedAt.IsZero() {
		t.Error("IndexedAt should not be zero")
	}

	// Check entity ID format: {org}.semsource.{language}.{system}.{type}.{instance}
	expectedPrefix := "acme.semsource.golang.myproject.function."
	if !strings.HasPrefix(entity.ID, expectedPrefix) {
		t.Errorf("ID = %q, want prefix %q", entity.ID, expectedPrefix)
	}
}

func TestNewCodeEntity_PrivateVisibility(t *testing.T) {
	entity := NewCodeEntity("acme", "golang", "myproject", TypeFunction, "foo", "pkg/foo.go")

	if entity.Visibility != VisibilityPrivate {
		t.Errorf("Visibility = %q, want %q", entity.Visibility, VisibilityPrivate)
	}
}

func TestNewCodeEntity_FileType(t *testing.T) {
	entity := NewCodeEntity("acme", "golang", "myproject", TypeFile, "foo.go", "pkg/foo.go")

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

// TestCodeEntity_ExportedMarker verifies the presence-only ranking marker
// (task #38): stamped on exported symbols, absent on unexported ones — the
// asymmetry is what lets salience boost public API over internals.
func TestCodeEntity_ExportedMarker(t *testing.T) {
	hasExported := func(e *CodeEntity) bool {
		for _, tr := range e.Triples() {
			if tr.Predicate == CodeExported {
				return true
			}
		}
		return false
	}
	pub := NewCodeEntity("acme", "golang", "myproject", TypeFunction, "Foo", "pkg/foo.go")
	if !hasExported(pub) {
		t.Error("exported symbol Foo: missing CodeExported marker")
	}
	priv := NewCodeEntity("acme", "golang", "myproject", TypeFunction, "foo", "pkg/foo.go")
	if hasExported(priv) {
		t.Error("unexported symbol foo: CodeExported marker must NOT be stamped")
	}
}

func TestCodeEntity_Triples(t *testing.T) {
	entity := NewCodeEntity("acme", "golang", "myproject", TypeFunction, "Foo", "pkg/foo.go")
	entity.Package = "pkg"
	entity.Hash = "abc123"
	entity.StartLine = 10
	entity.EndLine = 20
	entity.DocComment = "Foo does something."
	entity.Signature = "func Foo() error"
	entity.ContainedBy = "acme.semsource.golang.myproject.file.pkg-foo-go"
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
		CodeExported, // "Foo" is exported → presence marker stamped
		CodeStartLine,
		CodeEndLine,
		CodeLines,
		CodeDocComment,
		CodeSignature,
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
	if predicateMap[CodeLanguage] != "golang" {
		t.Errorf("CodeLanguage = %v, want %q", predicateMap[CodeLanguage], "golang")
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

func TestCodeEntity_SignaturePredicateRegistered(t *testing.T) {
	if meta := semvocab.GetPredicateMetadata(CodeSignature); meta == nil {
		t.Fatalf("CodeSignature predicate %q is not registered", CodeSignature)
	}
}

func TestCodeEntity_EntityState(t *testing.T) {
	entity := NewCodeEntity("acme", "golang", "myproject", TypeStruct, "User", "pkg/user.go")
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
	if state.IndexingProfile != semvocab.IndexingProfileContent {
		t.Errorf("IndexingProfile = %q, want %q", state.IndexingProfile, semvocab.IndexingProfileContent)
	}
	assertASTEntityStateID(t, state)
}

func assertASTEntityStateID(t *testing.T, state *EntityState) {
	t.Helper()

	if err := entityid.ValidateNATSKVKey(state.ID); err != nil {
		t.Fatalf("entity ID %q is not a valid NATS KV key: %v", state.ID, err)
	}
	parts := strings.Split(state.ID, ".")
	if len(parts) != 6 {
		t.Fatalf("entity ID %q has %d parts, want 6", state.ID, len(parts))
	}
	for i, part := range parts {
		if part == "" {
			t.Fatalf("entity ID %q has empty part %d", state.ID, i)
		}
	}
}

func TestCodeEntity_TriplesAreSelfSubject(t *testing.T) {
	entity := NewCodeEntity("acme", "golang", "myproject", TypeFunction, "Foo", "pkg/foo.go")
	entity.Contains = []string{"acme.semsource.golang.myproject.method.Bar"}
	entity.Calls = []string{"acme.semsource.golang.myproject.function.Helper"}
	entity.Returns = []string{"error"}

	state := entity.EntityState()
	for i, triple := range state.Triples {
		if triple.Subject != state.ID {
			t.Fatalf("triple %d subject = %q, want %q", i, triple.Subject, state.ID)
		}
	}
}

func TestCodeEntity_IndexingProfile(t *testing.T) {
	tests := []struct {
		name       string
		entityType CodeEntityType
		want       string
	}{
		{"function", TypeFunction, semvocab.IndexingProfileContent},
		{"component", TypeComponent, semvocab.IndexingProfileContent},
		{"repo", TypeRepo, semvocab.IndexingProfileControl},
		{"folder", TypeFolder, semvocab.IndexingProfileControl},
		{"file", TypeFile, semvocab.IndexingProfileControl},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity := NewCodeEntity("acme", "golang", "myproject", tt.entityType, tt.name, "pkg/item.go")
			if got := entity.IndexingProfile(); got != tt.want {
				t.Errorf("IndexingProfile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseResult_AllTriples(t *testing.T) {
	result := &ParseResult{
		Entities: []*CodeEntity{
			NewCodeEntity("acme", "golang", "test", TypeFile, "foo.go", "foo.go"),
			NewCodeEntity("acme", "golang", "test", TypeFunction, "Foo", "foo.go"),
			NewCodeEntity("acme", "golang", "test", TypeStruct, "Bar", "foo.go"),
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
			NewCodeEntity("acme", "golang", "test", TypeFile, "foo.go", "foo.go"),
			NewCodeEntity("acme", "golang", "test", TypeFunction, "Foo", "foo.go"),
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

// TestNewCodeEntity_VersionBearingProject_ValidID verifies that a project string
// containing characters illegal in entity IDs (such as '@' from Go module-cache
// paths or '.' from canonical module paths) is sanitized to a valid NATS KV key
// with exactly 6 dot-separated parts and no stray dots in the system segment.
func TestNewCodeEntity_VersionBearingProject_ValidID(t *testing.T) {
	// "semstreams@v1.9.0" mimics a Go module-cache path component — the '@'
	// and '.' were previously passed raw to Build, yielding an invalid NATS key.
	entity := NewCodeEntity("acme", "golang", "semstreams@v1.9.0", TypeFunction, "Run", "pkg/run.go")

	if err := entityid.ValidateNATSKVKey(entity.ID); err != nil {
		t.Errorf("entity ID %q failed NATS KV validation: %v", entity.ID, err)
	}
	parts := strings.Split(entity.ID, ".")
	if len(parts) != 6 {
		t.Errorf("entity ID %q has %d dot-separated parts, want exactly 6 (stray dots in system segment?)",
			entity.ID, len(parts))
	}
}

// TestNewCodeEntity_CrossVersionDistinctness verifies that the same symbol at
// two different versions produces IDs that are distinct and differ only in the
// system segment. The component pre-computes the scoped system slug via
// entityid.VersionScopedSlug before passing it to CreateParser/BuildHierarchy;
// this test mirrors that boundary.
func TestNewCodeEntity_CrossVersionDistinctness(t *testing.T) {
	systemV19 := entityid.VersionScopedSlug(
		entityid.SystemSlug("semstreams"),
		entityid.SystemSlug("v1.9.0"),
	)
	systemV110 := entityid.VersionScopedSlug(
		entityid.SystemSlug("semstreams"),
		entityid.SystemSlug("v1.10.0"),
	)

	e1 := NewCodeEntity("acme", "golang", systemV19, TypeFunction, "Run", "pkg/run.go")
	e2 := NewCodeEntity("acme", "golang", systemV110, TypeFunction, "Run", "pkg/run.go")

	if e1.ID == e2.ID {
		t.Fatalf("v1.9.0 and v1.10.0 entities must have distinct IDs, but both are %q", e1.ID)
	}

	p1 := strings.SplitN(e1.ID, ".", 6)
	p2 := strings.SplitN(e2.ID, ".", 6)
	if len(p1) != 6 || len(p2) != 6 {
		t.Fatalf("expected 6-part IDs, got %d and %d parts", len(p1), len(p2))
	}

	// System segment (index 3) must differ between the two versions.
	if p1[3] == p2[3] {
		t.Errorf("system segments are identical (%q) — cross-version scoping did not take effect", p1[3])
	}

	// All other segments (org, platform, domain, type, instance) must be identical.
	for _, idx := range []int{0, 1, 2, 4, 5} {
		if p1[idx] != p2[idx] {
			t.Errorf("segment[%d] differs unexpectedly: %q vs %q", idx, p1[idx], p2[idx])
		}
	}
}

// TestNewCodeEntity_BackwardCompat_CleanProject is a golden-string assertion:
// a clean project (no illegal chars) with an empty version must produce a
// byte-identical ID to the pre-change construction. A future refactor that
// silently churns these IDs will break this test.
func TestNewCodeEntity_BackwardCompat_CleanProject(t *testing.T) {
	const wantID = "acme.semsource.golang.myproject.function.pkg-foo-go-Foo"
	entity := NewCodeEntity("acme", "golang", "myproject", TypeFunction, "Foo", "pkg/foo.go")
	if entity.ID != wantID {
		t.Errorf("backward-compat golden ID mismatch:\ngot  %q\nwant %q", entity.ID, wantID)
	}
}

// TestCodeEntity_VersionTriples_Present covers ADR-0008 #2 spec scenario
// "Entity indexed at a version": when Project and Version are set, the entity
// carries both the source-identity triple (code.artifact.project) and the
// version triple (code.artifact.version).
func TestCodeEntity_VersionTriples_Present(t *testing.T) {
	e := NewCodeEntity("acme", "golang", "semstreams", TypeFunction, "Run", "pkg/run.go")
	e.Project = "semstreams"
	e.Version = "v1.9.0"

	got := make(map[string]interface{})
	for _, tr := range e.Triples() {
		got[tr.Predicate] = tr.Object
	}
	if got[CodeProject] != "semstreams" {
		t.Errorf("code.artifact.project = %v, want %q", got[CodeProject], "semstreams")
	}
	if got[CodeVersion] != "v1.9.0" {
		t.Errorf("code.artifact.version = %v, want %q", got[CodeVersion], "v1.9.0")
	}
}

// TestCodeEntity_VersionTriples_AbsentWhenVersionless covers ADR-0008 #2 spec
// scenario "Entity indexed without a version (backward compatible)": a
// version-less entity carries NEITHER triple — even when Project is set, the
// gate on Version suppresses both — and its emitted triples are byte-identical
// to an entity with no source-scoping set at all.
func TestCodeEntity_VersionTriples_AbsentWhenVersionless(t *testing.T) {
	withProject := NewCodeEntity("acme", "golang", "semstreams", TypeFunction, "Run", "pkg/run.go")
	withProject.Project = "semstreams" // Project set, Version empty → gate must suppress both

	for _, tr := range withProject.Triples() {
		if tr.Predicate == CodeVersion || tr.Predicate == CodeProject {
			t.Fatalf("version-less entity must not carry %q (object %v)", tr.Predicate, tr.Object)
		}
	}

	// Golden: with Version empty, setting Project must not perturb the triple set.
	// Pin IndexedAt so the DcCreated timestamp doesn't spuriously differ.
	bare := NewCodeEntity("acme", "golang", "semstreams", TypeFunction, "Run", "pkg/run.go")
	bare.IndexedAt = withProject.IndexedAt
	if !reflect.DeepEqual(bare.Triples(), withProject.Triples()) {
		t.Errorf("version-less triples not byte-identical when Project is set:\n bare=%v\n proj=%v",
			bare.Triples(), withProject.Triples())
	}
}

func TestEntityState_Fields(t *testing.T) {
	now := time.Now()
	state := &EntityState{
		ID:        "acme.semsource.golang.test.function.foo",
		UpdatedAt: now,
	}

	if state.ID != "acme.semsource.golang.test.function.foo" {
		t.Errorf("ID = %q, want %q", state.ID, "acme.semsource.golang.test.function.foo")
	}
	if !state.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", state.UpdatedAt, now)
	}
}

func TestCodeEntity_MethodWithReceiver(t *testing.T) {
	entity := NewCodeEntity("acme", "golang", "test", TypeMethod, "String", "user.go")
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
	entity := NewCodeEntity("acme", "golang", "test", TypeStruct, "Derived", "types.go")
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
	entity := NewCodeEntity("acme", "golang", "test", TypeFile, "main.go", "main.go")
	entity.Contains = []string{
		"acme.semsource.golang.test.function.main-go-main",
		"acme.semsource.golang.test.function.main-go-helper",
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
