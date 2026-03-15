package workspace_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/c360studio/semsource/workspace"
)

// createTestRepo initialises a bare-minimum git repo in a temp directory with
// a single empty initial commit on the default branch (main).
func createTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		// Provide minimal identity so git commit succeeds in CI environments
		// that have no global config.
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
	}

	run("init", "-b", "main")
	run("commit", "--allow-empty", "-m", "init")
	return dir
}

// TestBranchSlug verifies that branch names are converted to filesystem-safe slugs.
func TestBranchSlug(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"main", "main"},
		{"scenario/auth-flow", "scenario-auth-flow"},
		{"feature/JIRA-123/impl", "feature-jira-123-impl"},
	}
	for _, tt := range tests {
		got := workspace.BranchSlug(tt.branch)
		if got != tt.want {
			t.Errorf("BranchSlug(%q) = %q, want %q", tt.branch, got, tt.want)
		}
	}
}

// TestListBranches verifies that all local branches are returned.
func TestListBranches(t *testing.T) {
	dir := createTestRepo(t)
	ctx := context.Background()

	// Create a second branch.
	cmd := exec.Command("git", "branch", "feature/hello")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v\n%s", err, string(out))
	}

	branches, err := workspace.ListBranches(ctx, dir)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}

	want := []string{"feature/hello", "main"}
	slices.Sort(branches)
	slices.Sort(want)

	if len(branches) != len(want) {
		t.Fatalf("ListBranches returned %v, want %v", branches, want)
	}
	for i := range want {
		if branches[i] != want[i] {
			t.Errorf("branches[%d] = %q, want %q", i, branches[i], want[i])
		}
	}
}

// TestListWorktrees verifies that both the main worktree and an added worktree
// are returned with correct branch names.
func TestListWorktrees(t *testing.T) {
	dir := createTestRepo(t)
	ctx := context.Background()

	// Create a branch for the new worktree.
	runInDir := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
	}
	runInDir(dir, "branch", "feature/wt")

	wtDir := t.TempDir()
	wtPath := filepath.Join(wtDir, "feature-wt")
	runInDir(dir, "worktree", "add", wtPath, "feature/wt")

	worktrees, err := workspace.ListWorktrees(ctx, dir)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}

	if len(worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %d: %+v", len(worktrees), worktrees)
	}

	// On macOS, git resolves /var/folders through the /private symlink.
	// Normalise both sides so the comparison is platform-independent.
	realWtPath, err := filepath.EvalSymlinks(wtPath)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", wtPath, err)
	}

	// Build a map for order-independent assertions.
	byBranch := make(map[string]string, len(worktrees))
	for _, wt := range worktrees {
		byBranch[wt.Branch] = wt.Path
	}

	if _, ok := byBranch["main"]; !ok {
		t.Errorf("expected worktree for branch 'main', got %v", byBranch)
	}
	if path, ok := byBranch["feature/wt"]; !ok {
		t.Errorf("expected worktree for branch 'feature/wt', got %v", byBranch)
	} else if path != realWtPath {
		t.Errorf("worktree path = %q, want %q", path, realWtPath)
	}
}

// TestMatchBranches verifies glob pattern filtering behaviour.
// filepath.Match does NOT cross path separators — "scenario/*" matches
// "scenario/auth" but not "scenario/deep/nested", and "*" only matches
// names that contain no slash (i.e. "main").
func TestMatchBranches(t *testing.T) {
	branches := []string{"main", "scenario/auth", "scenario/deep/nested", "feature/x"}

	tests := []struct {
		patterns []string
		want     []string
	}{
		{
			// "*" matches a single path segment with no slashes — only "main".
			patterns: []string{"*"},
			want:     []string{"main"},
		},
		{
			patterns: []string{"main"},
			want:     []string{"main"},
		},
		{
			// "scenario/*" matches exactly one segment after "scenario/".
			patterns: []string{"scenario/*"},
			want:     []string{"scenario/auth"},
		},
		{
			patterns: []string{"scenario/*", "main"},
			want:     []string{"main", "scenario/auth"},
		},
	}

	for _, tt := range tests {
		got := workspace.MatchBranches(branches, tt.patterns)
		slices.Sort(got)
		want := slices.Clone(tt.want)
		slices.Sort(want)

		if len(got) != len(want) {
			t.Errorf("MatchBranches(patterns=%v) = %v, want %v", tt.patterns, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("MatchBranches(patterns=%v)[%d] = %q, want %q", tt.patterns, i, got[i], want[i])
			}
		}
	}
}

// TestEnsureWorktree verifies that a worktree is created for a branch and that
// a second call is idempotent, returning the same path without error.
func TestEnsureWorktree(t *testing.T) {
	dir := createTestRepo(t)
	ctx := context.Background()

	// Create the branch we want a worktree for.
	cmd := exec.Command("git", "branch", "feature/wt-test")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v\n%s", err, string(out))
	}

	wtBase := t.TempDir()
	path1, err := workspace.EnsureWorktree(ctx, dir, "feature/wt-test", wtBase)
	if err != nil {
		t.Fatalf("EnsureWorktree (first call): %v", err)
	}

	expectedPath := filepath.Join(wtBase, "feature-wt-test")
	// On macOS, git resolves temp paths through /private. Compare real paths.
	realPath1, err := filepath.EvalSymlinks(path1)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", path1, err)
	}
	realExpected, err := filepath.EvalSymlinks(expectedPath)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", expectedPath, err)
	}
	if realPath1 != realExpected {
		t.Errorf("EnsureWorktree path = %q, want %q", realPath1, realExpected)
	}
	if _, err := os.Stat(path1); err != nil {
		t.Errorf("worktree directory does not exist: %v", err)
	}

	// Second call must return the same real path without error (idempotent).
	path2, err := workspace.EnsureWorktree(ctx, dir, "feature/wt-test", wtBase)
	if err != nil {
		t.Fatalf("EnsureWorktree (second call): %v", err)
	}
	realPath2, err := filepath.EvalSymlinks(path2)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", path2, err)
	}
	if realPath2 != realPath1 {
		t.Errorf("EnsureWorktree idempotency: got %q, want %q", realPath2, realPath1)
	}
}
