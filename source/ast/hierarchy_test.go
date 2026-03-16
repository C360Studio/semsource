package ast_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/source/ast"
)

func TestBuildHierarchy_SingleRootFile(t *testing.T) {
	// A file at the root level should be contained by the repo entity directly.
	file := &ast.CodeEntity{
		ID:        entityid.Build("acme", entityid.PlatformSemsource, "golang", "my-project", "file", "main-go"),
		Type:      ast.TypeFile,
		Name:      "main.go",
		Path:      "main.go",
		IndexedAt: time.Now(),
	}
	results := []*ast.ParseResult{{
		FileEntity: file,
		Entities:   []*ast.CodeEntity{file},
		Path:       "main.go",
	}}

	entities := ast.BuildHierarchy(results, "acme", "my-project")

	// Should get exactly one entity: the repo
	var repo *ast.CodeEntity
	for _, e := range entities {
		if e.Type == ast.TypeRepo {
			repo = e
		}
	}
	if repo == nil {
		t.Fatal("expected repo entity")
	}

	// Repo should contain the file
	if !slices.Contains(repo.Contains, file.ID) {
		t.Errorf("repo.Contains = %v, want to include %q", repo.Contains, file.ID)
	}

	// File should belong to repo
	if file.ContainedBy != repo.ID {
		t.Errorf("file.ContainedBy = %q, want %q", file.ContainedBy, repo.ID)
	}

	// No folder entities expected
	for _, e := range entities {
		if e.Type == ast.TypeFolder {
			t.Errorf("unexpected folder entity: %s", e.ID)
		}
	}
}

func TestBuildHierarchy_NestedFile(t *testing.T) {
	// A file at pkg/auth/handler.go should produce:
	// repo → folder(pkg) → folder(pkg/auth) → file
	file := &ast.CodeEntity{
		ID:        entityid.Build("acme", entityid.PlatformSemsource, "golang", "my-project", "file", "pkg-auth-handler-go"),
		Type:      ast.TypeFile,
		Name:      "handler.go",
		Path:      "pkg/auth/handler.go",
		IndexedAt: time.Now(),
	}
	results := []*ast.ParseResult{{
		FileEntity: file,
		Entities:   []*ast.CodeEntity{file},
		Path:       "pkg/auth/handler.go",
	}}

	entities := ast.BuildHierarchy(results, "acme", "my-project")

	// Should have repo + 2 folders
	var repo *ast.CodeEntity
	folders := make(map[string]*ast.CodeEntity) // path → entity
	for _, e := range entities {
		switch e.Type {
		case ast.TypeRepo:
			repo = e
		case ast.TypeFolder:
			folders[e.Path] = e
		}
	}

	if repo == nil {
		t.Fatal("expected repo entity")
	}
	if len(folders) != 2 {
		t.Fatalf("got %d folders, want 2 (pkg, pkg/auth)", len(folders))
	}

	pkgFolder, ok := folders["pkg"]
	if !ok {
		t.Fatal("missing folder entity for 'pkg'")
	}
	authFolder, ok := folders["pkg/auth"]
	if !ok {
		t.Fatal("missing folder entity for 'pkg/auth'")
	}

	// Chain: repo → pkg → pkg/auth → file
	if pkgFolder.ContainedBy != repo.ID {
		t.Errorf("pkg folder ContainedBy = %q, want repo %q", pkgFolder.ContainedBy, repo.ID)
	}
	if authFolder.ContainedBy != pkgFolder.ID {
		t.Errorf("auth folder ContainedBy = %q, want pkg folder %q", authFolder.ContainedBy, pkgFolder.ID)
	}
	if file.ContainedBy != authFolder.ID {
		t.Errorf("file ContainedBy = %q, want auth folder %q", file.ContainedBy, authFolder.ID)
	}

	// Repo contains only pkg folder
	if len(repo.Contains) != 1 || repo.Contains[0] != pkgFolder.ID {
		t.Errorf("repo.Contains = %v, want [%s]", repo.Contains, pkgFolder.ID)
	}

	// pkg folder contains auth folder
	if !slices.Contains(pkgFolder.Contains, authFolder.ID) {
		t.Errorf("pkg folder should contain auth folder")
	}

	// auth folder contains file
	if !slices.Contains(authFolder.Contains, file.ID) {
		t.Errorf("auth folder should contain file")
	}
}

func TestBuildHierarchy_MultipleFilesInSameFolder(t *testing.T) {
	// Two files in the same folder should produce one folder entity with both files.
	file1 := &ast.CodeEntity{
		ID:        "acme.semsource.golang.proj.file.pkg-handler-go",
		Type:      ast.TypeFile,
		Name:      "handler.go",
		Path:      "pkg/handler.go",
		IndexedAt: time.Now(),
	}
	file2 := &ast.CodeEntity{
		ID:        "acme.semsource.golang.proj.file.pkg-service-go",
		Type:      ast.TypeFile,
		Name:      "service.go",
		Path:      "pkg/service.go",
		IndexedAt: time.Now(),
	}
	results := []*ast.ParseResult{
		{FileEntity: file1, Entities: []*ast.CodeEntity{file1}, Path: "pkg/handler.go"},
		{FileEntity: file2, Entities: []*ast.CodeEntity{file2}, Path: "pkg/service.go"},
	}

	entities := ast.BuildHierarchy(results, "acme", "proj")

	var folderCount int
	for _, e := range entities {
		if e.Type == ast.TypeFolder {
			folderCount++
			// The single folder should contain both files
			if len(e.Contains) != 2 {
				t.Errorf("folder.Contains has %d entries, want 2", len(e.Contains))
			}
			if !slices.Contains(e.Contains, file1.ID) || !slices.Contains(e.Contains, file2.ID) {
				t.Errorf("folder should contain both files, got %v", e.Contains)
			}
		}
	}
	if folderCount != 1 {
		t.Errorf("got %d folder entities, want 1 (deduplication)", folderCount)
	}
}

func TestBuildHierarchy_DeterministicIDs(t *testing.T) {
	// Same inputs must produce identical entity IDs.
	makeFile := func() *ast.CodeEntity {
		return &ast.CodeEntity{
			ID:        "acme.semsource.golang.proj.file.src-main-go",
			Type:      ast.TypeFile,
			Name:      "main.go",
			Path:      "src/main.go",
			IndexedAt: time.Now(),
		}
	}

	file1 := makeFile()
	results1 := []*ast.ParseResult{{FileEntity: file1, Entities: []*ast.CodeEntity{file1}, Path: "src/main.go"}}
	entities1 := ast.BuildHierarchy(results1, "acme", "proj")

	file2 := makeFile()
	results2 := []*ast.ParseResult{{FileEntity: file2, Entities: []*ast.CodeEntity{file2}, Path: "src/main.go"}}
	entities2 := ast.BuildHierarchy(results2, "acme", "proj")

	if len(entities1) != len(entities2) {
		t.Fatalf("different entity counts: %d vs %d", len(entities1), len(entities2))
	}
	for i := range entities1 {
		if entities1[i].ID != entities2[i].ID {
			t.Errorf("entity[%d] ID mismatch: %q vs %q", i, entities1[i].ID, entities2[i].ID)
		}
	}
}

func TestBuildHierarchy_RepoAndFoldersUseCodeDomain(t *testing.T) {
	// Repo and folder entities should use "code" domain, not a language-specific one.
	file := &ast.CodeEntity{
		ID:        "acme.semsource.golang.proj.file.main-go",
		Type:      ast.TypeFile,
		Name:      "main.go",
		Path:      "main.go",
		IndexedAt: time.Now(),
	}
	results := []*ast.ParseResult{{
		FileEntity: file,
		Entities:   []*ast.CodeEntity{file},
		Path:       "main.go",
	}}

	entities := ast.BuildHierarchy(results, "acme", "proj")

	for _, e := range entities {
		// ID format: org.platform.domain.system.type.instance
		parts := strings.Split(e.ID, ".")
		if len(parts) < 6 {
			t.Fatalf("entity ID has fewer than 6 parts: %q", e.ID)
		}
		domain := parts[2]
		if domain != "code" {
			t.Errorf("entity %q domain = %q, want \"code\"", e.ID, domain)
		}
	}
}

func TestBuildHierarchy_NoLanguageOnStructuralEntities(t *testing.T) {
	// Repo and folder entities should not have Language set (it's not a programming language).
	file := &ast.CodeEntity{
		ID:        "acme.semsource.golang.proj.file.src-main-go",
		Type:      ast.TypeFile,
		Name:      "main.go",
		Path:      "src/main.go",
		IndexedAt: time.Now(),
	}
	results := []*ast.ParseResult{{
		FileEntity: file,
		Entities:   []*ast.CodeEntity{file},
		Path:       "src/main.go",
	}}

	entities := ast.BuildHierarchy(results, "acme", "proj")

	for _, e := range entities {
		if e.Language != "" {
			t.Errorf("structural entity %q (type=%s) has Language=%q, want empty",
				e.ID, e.Type, e.Language)
		}
	}
}

func TestBuildHierarchy_FolderEntityNames(t *testing.T) {
	// Folder entities should be named after their last path segment.
	file := &ast.CodeEntity{
		ID:        "acme.semsource.golang.proj.file.pkg-auth-handler-go",
		Type:      ast.TypeFile,
		Name:      "handler.go",
		Path:      "pkg/auth/handler.go",
		IndexedAt: time.Now(),
	}
	results := []*ast.ParseResult{{
		FileEntity: file,
		Entities:   []*ast.CodeEntity{file},
		Path:       "pkg/auth/handler.go",
	}}

	entities := ast.BuildHierarchy(results, "acme", "proj")

	for _, e := range entities {
		if e.Type == ast.TypeFolder {
			lastSeg := e.Path[strings.LastIndex(e.Path, "/")+1:]
			if e.Name != lastSeg {
				t.Errorf("folder %q Name = %q, want %q", e.Path, e.Name, lastSeg)
			}
		}
	}
}

func TestBuildHierarchy_NATSKVSafeIDs(t *testing.T) {
	// All generated entity IDs must be valid NATS KV keys.
	file := &ast.CodeEntity{
		ID:        "acme.semsource.golang.proj.file.src-deep-nested-file-go",
		Type:      ast.TypeFile,
		Name:      "file.go",
		Path:      "src/deep/nested/file.go",
		IndexedAt: time.Now(),
	}
	results := []*ast.ParseResult{{
		FileEntity: file,
		Entities:   []*ast.CodeEntity{file},
		Path:       "src/deep/nested/file.go",
	}}

	entities := ast.BuildHierarchy(results, "acme", "proj")

	for _, e := range entities {
		if err := entityid.ValidateNATSKVKey(e.ID); err != nil {
			t.Errorf("entity %q has invalid NATS KV key: %v", e.ID, err)
		}
	}
}

func TestBuildHierarchy_EmptyResults(t *testing.T) {
	entities := ast.BuildHierarchy(nil, "acme", "proj")
	if len(entities) != 0 {
		t.Errorf("expected no entities for nil results, got %d", len(entities))
	}

	entities = ast.BuildHierarchy([]*ast.ParseResult{}, "acme", "proj")
	if len(entities) != 0 {
		t.Errorf("expected no entities for empty results, got %d", len(entities))
	}
}

func TestBuildHierarchy_NilFileEntity(t *testing.T) {
	// ParseResults with nil FileEntity should be skipped without panicking.
	goodFile := &ast.CodeEntity{
		ID:        "acme.semsource.golang.proj.file.main-go",
		Type:      ast.TypeFile,
		Name:      "main.go",
		Path:      "main.go",
		IndexedAt: time.Now(),
	}
	results := []*ast.ParseResult{
		{FileEntity: nil, Entities: nil, Path: "bad.go"},
		{FileEntity: goodFile, Entities: []*ast.CodeEntity{goodFile}, Path: "main.go"},
	}

	entities := ast.BuildHierarchy(results, "acme", "proj")

	var repo *ast.CodeEntity
	for _, e := range entities {
		if e.Type == ast.TypeRepo {
			repo = e
		}
	}
	if repo == nil {
		t.Fatal("expected repo entity")
	}
	// Only the good file should be in repo.Contains
	if !slices.Contains(repo.Contains, goodFile.ID) {
		t.Errorf("repo should contain good file")
	}
	if len(repo.Contains) != 1 {
		t.Errorf("repo.Contains has %d entries, want 1", len(repo.Contains))
	}
}

func TestBuildHierarchy_ContainsSorted(t *testing.T) {
	// Contains lists should be deterministically sorted.
	fileA := &ast.CodeEntity{
		ID: "acme.semsource.golang.proj.file.pkg-z-go", Type: ast.TypeFile,
		Name: "z.go", Path: "pkg/z.go", IndexedAt: time.Now(),
	}
	fileB := &ast.CodeEntity{
		ID: "acme.semsource.golang.proj.file.pkg-a-go", Type: ast.TypeFile,
		Name: "a.go", Path: "pkg/a.go", IndexedAt: time.Now(),
	}
	results := []*ast.ParseResult{
		{FileEntity: fileA, Entities: []*ast.CodeEntity{fileA}, Path: "pkg/z.go"},
		{FileEntity: fileB, Entities: []*ast.CodeEntity{fileB}, Path: "pkg/a.go"},
	}

	entities := ast.BuildHierarchy(results, "acme", "proj")

	for _, e := range entities {
		if len(e.Contains) > 1 {
			sorted := slices.IsSorted(e.Contains)
			if !sorted {
				t.Errorf("entity %q Contains not sorted: %v", e.ID, e.Contains)
			}
		}
	}
}

func TestBuildHierarchy_SystemSlugApplied(t *testing.T) {
	// Project names with URL-like values should be slugified in the system segment.
	file := &ast.CodeEntity{
		ID:        "acme.semsource.golang.github.com-acme-repo.file.main-go",
		Type:      ast.TypeFile,
		Name:      "main.go",
		Path:      "main.go",
		IndexedAt: time.Now(),
	}
	results := []*ast.ParseResult{{
		FileEntity: file,
		Entities:   []*ast.CodeEntity{file},
		Path:       "main.go",
	}}

	entities := ast.BuildHierarchy(results, "acme", "https://github.com/acme/repo")

	repo := entities[0]
	// The repo ID should contain the slugified system segment, not raw URL
	expectedSlug := entityid.SystemSlug("https://github.com/acme/repo")
	if !strings.Contains(repo.ID, expectedSlug) {
		t.Errorf("repo ID %q does not contain system slug %q", repo.ID, expectedSlug)
	}
	// Should not contain URL characters
	if strings.Contains(repo.ID, "://") || strings.Contains(repo.ID, "/") {
		t.Errorf("repo ID contains unslugified URL characters: %q", repo.ID)
	}
}

func TestBuildFolderChain(t *testing.T) {
	entities := ast.BuildFolderChain("pkg/auth/handler.go", "acme", "proj")

	folders := make(map[string]*ast.CodeEntity)
	for _, e := range entities {
		if e.Type == ast.TypeRepo {
			t.Error("BuildFolderChain should not return repo entity")
		}
		if e.Type == ast.TypeFolder {
			folders[e.Path] = e
		}
	}

	if len(folders) != 2 {
		t.Fatalf("got %d folders, want 2 (pkg, pkg/auth)", len(folders))
	}

	pkgFolder := folders["pkg"]
	authFolder := folders["pkg/auth"]

	if pkgFolder == nil || authFolder == nil {
		t.Fatal("missing expected folder entities")
	}

	// Verify containment wiring
	if authFolder.ContainedBy != pkgFolder.ID {
		t.Errorf("auth folder ContainedBy = %q, want pkg folder %q",
			authFolder.ContainedBy, pkgFolder.ID)
	}
	if !slices.Contains(pkgFolder.Contains, authFolder.ID) {
		t.Errorf("pkg folder should contain auth folder")
	}

	// Top-level folder should link to repo ID
	if pkgFolder.ContainedBy == "" {
		t.Error("top-level folder ContainedBy should reference repo, got empty")
	}
}

func TestBuildFolderChain_RootFile(t *testing.T) {
	// A root-level file should produce no folder entities.
	entities := ast.BuildFolderChain("main.go", "acme", "proj")
	if len(entities) != 0 {
		t.Errorf("expected no entities for root file, got %d", len(entities))
	}
}

func TestSanitizePathSegment(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"pkg/auth", "pkg-auth"},
		{"main.go", "main-go"},
		{".hidden/dir", "hidden-dir"},
		{"a/b/c/d", "a-b-c-d"},
		{"no-change", "no-change"},
		{"", ""},
		{"...dots", "--dots"},      // leading dots become hyphens, first stripped
		{"a//double", "a--double"}, // consecutive slashes
		{"/leading/slash", "leading-slash"},
	}

	for _, tt := range tests {
		got := ast.SanitizePathSegment(tt.input)
		if got != tt.want {
			t.Errorf("SanitizePathSegment(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
