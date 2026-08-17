//go:build integration

package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	githandler "github.com/c360studio/semsource/handler/git"
)

// TestIngest_ProbesSubmoduleInventory pins the loudness plumbing: resolving
// the repo path (initial ingest and every poll) refreshes the submodule
// inventory the owning component reports on status — a declared but
// unmaterialized submodule is visible, not silently absent.
func TestIngest_ProbesSubmoduleInventory(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	// Declare a submodule whose tree was never initialized (plain-clone
	// shape): .gitmodules entry + fabricated gitlink, empty dir.
	run("config", "-f", ".gitmodules", "submodule.sub.path", "sub")
	run("config", "-f", ".gitmodules", "submodule.sub.url", "https://example.com/org/sub.git")
	run("add", ".gitmodules")
	run("update-index", "--add", "--cacheinfo",
		"160000,0123456789abcdef0123456789abcdef01234567,sub")
	run("commit", "-m", "declare uninitialized submodule")

	h := githandler.New(githandler.Config{Org: "acme"})
	if h.SubmoduleInventory() != nil {
		t.Fatal("inventory non-nil before first ingest")
	}
	if _, err := h.Ingest(context.Background(), &srcCfg{typ: "git", path: dir}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	inv := h.SubmoduleInventory()
	if inv == nil || len(inv.Submodules) != 1 {
		t.Fatalf("inventory after ingest = %+v, want one declared submodule", inv)
	}
	s := inv.Submodules[0]
	if s.Path != "sub" || s.Materialized || s.SHA == "" {
		t.Errorf("probe = %+v, want unmaterialized pinned declaration", s)
	}
}
