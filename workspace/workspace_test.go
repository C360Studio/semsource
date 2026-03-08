package workspace_test

import (
	"context"
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
