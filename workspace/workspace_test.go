package workspace_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/workspace"
)

func TestURLToSlug(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "https URL without .git suffix",
			input: "https://github.com/opensensorhub/osh-core",
			want:  "github-com-opensensorhub-osh-core",
		},
		{
			name:  "https URL with .git suffix",
			input: "https://github.com/acme/myrepo.git",
			want:  "github-com-acme-myrepo",
		},
		{
			name:  "SSH shorthand git@",
			input: "git@github.com:owner/repo.git",
			want:  "github-com-owner-repo",
		},
		{
			name:  "git:// protocol",
			input: "git://github.com/org/project.git",
			want:  "github-com-org-project",
		},
		{
			name:  "URL with dots in repo name",
			input: "https://github.com/acme/my.project",
			want:  "github-com-acme-my-project",
		},
		{
			name:  "URL with uppercase letters",
			input: "https://github.com/Acme/MyRepo",
			want:  "github-com-acme-myrepo",
		},
		{
			name:  "URL with deep path",
			input: "https://gitlab.example.com/team/subgroup/repo.git",
			want:  "gitlab-example-com-team-subgroup-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workspace.URLToSlug(tt.input)
			if got != tt.want {
				t.Errorf("URLToSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEnsureRepo_RequiresURL(t *testing.T) {
	ctx := context.Background()
	_, err := workspace.EnsureRepo(ctx, "", "main", "/tmp/workspace-test")
	if err == nil {
		t.Fatal("EnsureRepo with empty URL: expected error, got nil")
	}
}

func TestEnsureRepo_RequiresBaseDir(t *testing.T) {
	ctx := context.Background()
	_, err := workspace.EnsureRepo(ctx, "https://github.com/acme/repo.git", "main", "")
	if err == nil {
		t.Fatal("EnsureRepo with empty baseDir: expected error, got nil")
	}
}

// TestIsPathReady_AcceptsExistingFile guards the boot-blocking bug beta.159's
// component-start barrier exposed: doc-source and cfgfile-source legitimately
// name FILES ("README.md", "go.mod"), but the directory-only IsRepoReady can
// never accept one. Callers retry persistently, so under fail-closed boot the
// component never finishes Start and takes the whole service down with it.
func TestIsPathReady_AcceptsExistingFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(file, []byte("module example.com/x\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := workspace.IsPathReady(file); err != nil {
		t.Errorf("workspace.IsPathReady(regular file) = %v, want nil — a file that exists is ready", err)
	}
	// The directory-only contract is unchanged, and that is the point: ast-source
	// and git-source still need a non-directory to be an error.
	if err := workspace.IsRepoReady(file); err == nil {
		t.Error("workspace.IsRepoReady(regular file) = nil; its documented directory contract must not have been relaxed")
	}
}

// TestIsPathReady_DirectoryStillDelegates proves a directory keeps the full
// repo-readiness semantics rather than being short-circuited to "it exists".
func TestIsPathReady_DirectoryStillDelegates(t *testing.T) {
	dir := t.TempDir()
	if err := workspace.IsPathReady(dir); err != nil {
		t.Errorf("workspace.IsPathReady(plain directory) = %v, want nil", err)
	}

	// A .git directory with no HEAD is a clone in progress; IsPathReady must
	// report it, not mask it.
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("create fixture repo: %v", err)
	}
	if err := workspace.IsPathReady(repo); err == nil {
		t.Error("workspace.IsPathReady(.git without HEAD) = nil; an in-progress clone must not read as ready")
	}
}

// TestIsPathReady_MissingPath keeps absence distinct from readiness.
func TestIsPathReady_MissingPath(t *testing.T) {
	if err := workspace.IsPathReady(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("workspace.IsPathReady(missing path) = nil, want an error")
	}
}
