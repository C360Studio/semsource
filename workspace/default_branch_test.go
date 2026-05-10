package workspace

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSymrefHEAD(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "modern main",
			input: "ref: refs/heads/main\tHEAD\nabc123\tHEAD\n",
			want:  "main",
		},
		{
			name:  "legacy master",
			input: "ref: refs/heads/master\tHEAD\ndef456\tHEAD\n",
			want:  "master",
		},
		{
			name:  "custom default trunk",
			input: "ref: refs/heads/trunk\tHEAD\n0123abc\tHEAD\n",
			want:  "trunk",
		},
		{
			name:  "develop with multiple lines",
			input: "ref: refs/heads/develop\tHEAD\nf00d\tHEAD\nbeef\trefs/tags/v1.0\n",
			want:  "develop",
		},
		{
			name:  "branch name with slashes",
			input: "ref: refs/heads/release/v2\tHEAD\n0000\tHEAD\n",
			want:  "release/v2",
		},
		{
			name:    "missing symref line",
			input:   "abc123\tHEAD\n",
			wantErr: true,
		},
		{
			name:    "HEAD points outside refs/heads",
			input:   "ref: refs/tags/v1.0\tHEAD\n",
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSymrefHEAD(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseSymrefHEAD err = %v, wantErr = %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseSymrefHEAD = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveDefaultBranch_LocalRepo verifies the round-trip against a
// real local repo with a commit on the configured initial branch. Skips
// when git is unavailable. Covers the load-bearing case (pre-rename
// "master" repos, custom defaults like trunk/develop) without hitting
// the network — empty bare repos don't resolve HEAD via ls-remote, so
// we use a working repo with one commit instead.
func TestResolveDefaultBranch_LocalRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	cases := []string{"main", "master", "trunk", "develop"}
	for _, branch := range cases {
		t.Run(branch, func(t *testing.T) {
			repoPath := t.TempDir()
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command("git", args...)
				cmd.Dir = repoPath
				// Avoid leaking the test runner's user identity into commits.
				cmd.Env = append(cmd.Env,
					"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
					"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
					"HOME="+t.TempDir(), "PATH=/usr/bin:/bin:/usr/local/bin",
				)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
			}
			run("init", "-b", branch)
			run("commit", "--allow-empty", "-m", "init")

			got, err := ResolveDefaultBranch(context.Background(), repoPath, "")
			if err != nil {
				t.Fatalf("ResolveDefaultBranch(%s) error: %v", repoPath, err)
			}
			if got != branch {
				t.Errorf("ResolveDefaultBranch = %q, want %q", got, branch)
			}
		})
	}
}

// TestResolveDefaultBranch_BadURL ensures network/URL failures surface as
// errors rather than empty strings, so callers can fall back deliberately.
func TestResolveDefaultBranch_BadURL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// Path that definitely does not contain a git repo. git ls-remote
	// will fail; we expect a non-nil error.
	_, err := ResolveDefaultBranch(context.Background(), filepath.Join(t.TempDir(), "nope"), "")
	if err == nil {
		t.Fatal("expected error for non-existent repo, got nil")
	}
	if !strings.Contains(err.Error(), "ls-remote") {
		t.Errorf("error should mention ls-remote, got: %v", err)
	}
}
