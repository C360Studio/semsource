package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsRepoReady_GitLinkFile pins the worktree/submodule case. In both, .git is
// a FILE pointing at the real gitdir, so .git/HEAD is a path inside a regular
// file and can never resolve. Reporting that as "clone in progress" told callers
// to retry forever: the source never ingested and the service sat at phase
// "seeding" with the reason only at debug level.
func TestIsRepoReady_GitLinkFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("README.md", "# content\n")
	write(".git", "gitdir: /elsewhere/.git/worktrees/corpus\n")

	if err := IsRepoReady(dir); err != nil {
		t.Fatalf("IsRepoReady on a git worktree = %v, want nil — .git is a gitlink file, "+
			"written after checkout completes, so the tree IS ready", err)
	}
}

func TestIsRepoReady_Cases(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		wantErr bool
		why     string
	}{
		{
			name:  "plain directory, no git",
			setup: func(t *testing.T, d string) { mustWrite(t, d, "a.md", "x") },
		},
		{
			name: "complete clone",
			setup: func(t *testing.T, d string) {
				mustWrite(t, d, "a.md", "x")
				mustMkdir(t, d, ".git")
				mustWrite(t, filepath.Join(d, ".git"), "HEAD", "ref: refs/heads/main\n")
			},
		},
		{
			name: "clone in progress: .git dir without HEAD",
			setup: func(t *testing.T, d string) {
				mustWrite(t, d, "a.md", "x")
				mustMkdir(t, d, ".git")
			},
			wantErr: true,
			why:     "a real .git directory with no HEAD IS an in-flight clone — the guard must stay",
		},
		{
			name: "empty working tree",
			setup: func(t *testing.T, d string) {
				mustMkdir(t, d, ".git")
				mustWrite(t, filepath.Join(d, ".git"), "HEAD", "ref: refs/heads/main\n")
			},
			wantErr: true,
			why:     "HEAD can land before the checkout populates",
		},
		{
			name: "gitlink file with empty working tree",
			setup: func(t *testing.T, d string) {
				mustWrite(t, d, ".git", "gitdir: /elsewhere\n")
			},
			wantErr: true,
			why:     "the gitlink shortcut must not skip the working-tree content check",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			err := IsRepoReady(dir)
			if tt.wantErr && err == nil {
				t.Errorf("expected an error: %s", tt.why)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func mustWrite(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
}
