package workspace_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/workspace"
)

// allowFileProtocol lets EnsureRepo's internal git commands clone local-path
// submodules (blocked by default since CVE-2022-39253). EnsureRepo builds its
// own commands, so the override must come through the process environment —
// which applyAuth only preserves when no token is set.
func allowFileProtocol(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")
}

func TestEnsureRepo_MaterializesSubmodules(t *testing.T) {
	allowFileProtocol(t)
	parent, _, _ := buildChain(t)
	baseDir := t.TempDir()

	local, err := workspace.EnsureRepo(context.Background(), parent, "", baseDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a/f.txt", "a/sub-b/f.txt", "nested/b/f.txt"} {
		if _, err := os.Stat(filepath.Join(local, f)); err != nil {
			t.Errorf("submodule file %s not materialized: %v", f, err)
		}
	}
}

func TestEnsureRepo_SkipSubmodules(t *testing.T) {
	allowFileProtocol(t)
	parent, _, _ := buildChain(t)
	baseDir := t.TempDir()

	local, err := workspace.EnsureRepo(context.Background(), parent, "", baseDir,
		workspace.Options{SkipSubmodules: true})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(local, "a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("submodule dir populated despite SkipSubmodules: %v", entries)
	}
}

func TestEnsureRepo_PullSyncsMovedGitlink(t *testing.T) {
	allowFileProtocol(t)
	parent, _, b := buildChain(t)
	baseDir := t.TempDir()
	ctx := context.Background()

	local, err := workspace.EnsureRepo(ctx, parent, "", baseDir)
	if err != nil {
		t.Fatal(err)
	}

	// Advance b, then move parent's nested/b gitlink to the new commit.
	if err := os.WriteFile(filepath.Join(b, "g.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, b, "add", ".")
	gitIn(t, b, "commit", "-m", "v2")
	newSHA := gitIn(t, b, "rev-parse", "HEAD")
	gitIn(t, filepath.Join(parent, "nested/b"), "pull", "origin", "main")
	gitIn(t, parent, "add", "nested/b")
	gitIn(t, parent, "commit", "-m", "move nested/b pin")

	// Second EnsureRepo takes the pull path and must sync the tree.
	if _, err := workspace.EnsureRepo(ctx, parent, "", baseDir); err != nil {
		t.Fatal(err)
	}
	got := gitIn(t, filepath.Join(local, "nested/b"), "rev-parse", "HEAD")
	if got != newSHA {
		t.Errorf("nested/b HEAD = %s after pull, want moved gitlink %s", got, newSHA)
	}
}

// TestEnsureRepo_NonTipPinFallsBackToFullFetch pins a submodule at a commit
// that is NOT its remote's branch tip. Over the file:// transport (which,
// unlike plain-path clones, honors --depth), the shallow submodule update
// cannot fetch a non-tip commit, so only the full-fetch fallback can
// materialize the tree — exactly the fixture's v1-pin situation.
func TestEnsureRepo_NonTipPinFallsBackToFullFetch(t *testing.T) {
	allowFileProtocol(t)
	sub := newRepoWithFile(t, "sub")
	oldSHA := gitIn(t, sub, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(sub, "g.txt"), []byte("tip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, sub, "add", ".")
	gitIn(t, sub, "commit", "-m", "tip")

	parent := newRepoWithFile(t, "parent")
	gitIn(t, parent, "submodule", "add", "file://"+sub, "pinned")
	gitIn(t, filepath.Join(parent, "pinned"), "checkout", oldSHA)
	gitIn(t, parent, "add", "pinned")
	gitIn(t, parent, "commit", "-m", "pin at non-tip")

	local, err := workspace.EnsureRepo(context.Background(), parent, "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got := gitIn(t, filepath.Join(local, "pinned"), "rev-parse", "HEAD")
	if got != oldSHA {
		t.Errorf("pinned HEAD = %s, want non-tip pin %s", got, oldSHA)
	}
	if _, err := os.Stat(filepath.Join(local, "pinned", "g.txt")); err == nil {
		t.Error("tip-only file present — tree is at the wrong commit")
	}
}
