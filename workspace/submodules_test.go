package workspace_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semsource/workspace"
)

// gitIn runs a git command in dir with a hermetic identity and the file
// protocol allowed (submodule clones from local paths are blocked by default
// since CVE-2022-39253).
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "protocol.file.allow=always"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

// newRepoWithFile creates a repo containing one committed file.
func newRepoWithFile(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-m", "init")
	return dir
}

// buildChain builds and returns (parent, a, b) where parent declares
// submodule "a" and a nested-path submodule "nested/b", and a itself
// declares submodule "sub-b" — a two-level chain plus a nested path.
func buildChain(t *testing.T) (parent, a, b string) {
	t.Helper()
	b = newRepoWithFile(t, "b")
	a = newRepoWithFile(t, "a")
	gitIn(t, a, "submodule", "add", b, "sub-b")
	gitIn(t, a, "commit", "-m", "add sub-b")

	parent = newRepoWithFile(t, "parent")
	gitIn(t, parent, "submodule", "add", a, "a")
	gitIn(t, parent, "submodule", "add", b, "nested/b")
	gitIn(t, parent, "commit", "-m", "add submodules")
	return parent, a, b
}

func bySubPath(inv *workspace.SubmoduleInventory) map[string]workspace.SubmoduleInfo {
	m := map[string]workspace.SubmoduleInfo{}
	for _, s := range inv.Submodules {
		m[s.Path] = s
	}
	return m
}

func TestListSubmodules_RecursiveClone(t *testing.T) {
	parent, _, b := buildChain(t)
	clone := filepath.Join(t.TempDir(), "clone")
	gitIn(t, filepath.Dir(clone), "clone", "--recurse-submodules", parent, clone)

	inv, err := workspace.ListSubmodules(context.Background(), clone)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.BeyondCap) != 0 {
		t.Errorf("BeyondCap = %v, want empty", inv.BeyondCap)
	}
	subs := bySubPath(inv)
	if len(subs) != 3 {
		t.Fatalf("got %d submodules (%v), want 3", len(subs), inv.Submodules)
	}

	bHead := gitIn(t, b, "rev-parse", "HEAD")
	for _, tc := range []struct {
		path  string
		depth int
		sha   string
	}{
		{"a", 1, ""},
		{"nested/b", 1, bHead},
		{"a/sub-b", 2, bHead},
	} {
		s, ok := subs[tc.path]
		if !ok {
			t.Errorf("missing submodule %q", tc.path)
			continue
		}
		if !s.Materialized {
			t.Errorf("%s: Materialized = false, want true", tc.path)
		}
		if s.Depth != tc.depth {
			t.Errorf("%s: Depth = %d, want %d", tc.path, s.Depth, tc.depth)
		}
		if s.SHA == "" || len(s.SHA) != 40 {
			t.Errorf("%s: SHA = %q, want full 40-hex gitlink", tc.path, s.SHA)
		}
		if tc.sha != "" && s.SHA != tc.sha {
			t.Errorf("%s: SHA = %s, want %s", tc.path, s.SHA, tc.sha)
		}
	}
}

func TestListSubmodules_PlainCloneIsUnmaterializedButPinned(t *testing.T) {
	parent, _, _ := buildChain(t)
	clone := filepath.Join(t.TempDir(), "clone")
	gitIn(t, filepath.Dir(clone), "clone", parent, clone)

	inv, err := workspace.ListSubmodules(context.Background(), clone)
	if err != nil {
		t.Fatal(err)
	}
	subs := bySubPath(inv)
	// Depth 2 is unreachable: "a" is not materialized, so no recursion.
	if len(subs) != 2 {
		t.Fatalf("got %d submodules (%v), want 2", len(subs), inv.Submodules)
	}
	for _, p := range []string{"a", "nested/b"} {
		s := subs[p]
		if s.Materialized {
			t.Errorf("%s: Materialized = true, want false after plain clone", p)
		}
		if len(s.SHA) != 40 {
			t.Errorf("%s: SHA = %q, want pinned gitlink even when unmaterialized", p, s.SHA)
		}
	}
}

func TestListSubmodules_StaleDeclarationHasNoSHA(t *testing.T) {
	parent := newRepoWithFile(t, "parent")
	gitIn(t, parent, "config", "-f", ".gitmodules", "submodule.ghost.path", "ghost")
	gitIn(t, parent, "config", "-f", ".gitmodules", "submodule.ghost.url", "https://example.com/ghost.git")
	gitIn(t, parent, "add", ".gitmodules")
	gitIn(t, parent, "commit", "-m", "stale declaration")

	inv, err := workspace.ListSubmodules(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Submodules) != 1 {
		t.Fatalf("got %v, want one ghost entry", inv.Submodules)
	}
	s := inv.Submodules[0]
	if s.Path != "ghost" || s.SHA != "" || s.Materialized {
		t.Errorf("ghost = %+v, want declared-but-absent (empty SHA, unmaterialized)", s)
	}
}

func TestListSubmodules_RelativeURLResolvesAgainstOrigin(t *testing.T) {
	repo := newRepoWithFile(t, "repo")
	gitIn(t, repo, "remote", "add", "origin", "https://example.com/org/parent.git")
	gitIn(t, repo, "config", "-f", ".gitmodules", "submodule.sib.path", "sib")
	gitIn(t, repo, "config", "-f", ".gitmodules", "submodule.sib.url", "../sibling.git")
	gitIn(t, repo, "add", ".gitmodules")
	// Fabricate the gitlink without a real subrepo: hermetic, no clone.
	gitIn(t, repo, "update-index", "--add", "--cacheinfo",
		"160000,0123456789abcdef0123456789abcdef01234567,sib")
	gitIn(t, repo, "commit", "-m", "declare sibling")

	inv, err := workspace.ListSubmodules(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Submodules) != 1 {
		t.Fatalf("got %v, want one entry", inv.Submodules)
	}
	s := inv.Submodules[0]
	if s.URL != "../sibling.git" {
		t.Errorf("URL = %q, want raw relative form preserved", s.URL)
	}
	if s.ResolvedURL != "https://example.com/org/sibling.git" {
		t.Errorf("ResolvedURL = %q, want https://example.com/org/sibling.git", s.ResolvedURL)
	}
	if s.SHA != "0123456789abcdef0123456789abcdef01234567" {
		t.Errorf("SHA = %q, want fabricated gitlink", s.SHA)
	}
}

func TestListSubmodules_DepthCapReportsBeyondCap(t *testing.T) {
	parent, _, _ := buildChain(t)
	clone := filepath.Join(t.TempDir(), "clone")
	gitIn(t, filepath.Dir(clone), "clone", "--recurse-submodules", parent, clone)

	inv, err := workspace.ListSubmodulesCapped(context.Background(), clone, 1)
	if err != nil {
		t.Fatal(err)
	}
	subs := bySubPath(inv)
	if _, ok := subs["a/sub-b"]; ok {
		t.Error("a/sub-b expanded despite cap 1")
	}
	if len(inv.BeyondCap) != 1 || inv.BeyondCap[0] != "a/sub-b" {
		t.Errorf("BeyondCap = %v, want [a/sub-b]", inv.BeyondCap)
	}
}

func TestListSubmodules_NoGitmodulesIsEmpty(t *testing.T) {
	repo := newRepoWithFile(t, "repo")
	inv, err := workspace.ListSubmodules(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Submodules) != 0 || len(inv.BeyondCap) != 0 {
		t.Errorf("inventory = %+v, want empty", inv)
	}
}

func TestResolveSubmoduleURL(t *testing.T) {
	tests := []struct {
		base, raw, want string
	}{
		{"https://example.com/org/parent.git", "../sibling.git", "https://example.com/org/sibling.git"},
		{"https://example.com/org/parent", "./child.git", "https://example.com/org/parent/child.git"},
		{"git@example.com:org/parent.git", "../sibling.git", "git@example.com:org/sibling.git"},
		{"/srv/git/parent", "../sibling", "/srv/git/sibling"},
		{"https://example.com/org/parent.git", "https://other.com/x.git", "https://other.com/x.git"},
		{"", "../sibling.git", "../sibling.git"},
	}
	for _, tt := range tests {
		if got := workspace.ResolveSubmoduleURL(tt.base, tt.raw); got != tt.want {
			t.Errorf("resolve(%q, %q) = %q, want %q", tt.base, tt.raw, got, tt.want)
		}
	}
}
