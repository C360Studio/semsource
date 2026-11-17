package config

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

func TestExpandRepoSources_ExpandsSingleRepo(t *testing.T) {
	sources := []SourceEntry{
		{Type: "repo", URL: "https://github.com/opensensorhub/osh-core", Language: "java", Watch: true, Branch: "master"},
	}
	result, err := ExpandRepoSources(context.Background(), sources, "/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expanded := result.Sources
	if len(expanded) != 4 {
		t.Fatalf("expected 4 expanded sources, got %d", len(expanded))
	}
	// First must be git
	if expanded[0].Type != "git" {
		t.Errorf("first source type = %q, want git", expanded[0].Type)
	}
	if expanded[0].URL != "https://github.com/opensensorhub/osh-core" {
		t.Errorf("git URL = %q", expanded[0].URL)
	}
	if expanded[0].Branch != "master" {
		t.Errorf("git branch = %q, want master", expanded[0].Branch)
	}
	// Second must be ast with language
	if expanded[1].Type != "ast" {
		t.Errorf("second source type = %q, want ast", expanded[1].Type)
	}
	if expanded[1].Language != "java" {
		t.Errorf("ast language = %q, want java", expanded[1].Language)
	}
	if expanded[1].Path == "" {
		t.Error("ast path must not be empty")
	}
	// Third must be docs
	if expanded[2].Type != "docs" {
		t.Errorf("third source type = %q, want docs", expanded[2].Type)
	}
	// Fourth must be config
	if expanded[3].Type != "config" {
		t.Errorf("fourth source type = %q, want config", expanded[3].Type)
	}
	// All should have watch=true
	for i, s := range expanded {
		if !s.Watch {
			t.Errorf("expanded[%d].Watch = false, want true", i)
		}
	}
	// No branch watchers for single-branch mode
	if len(result.Watchers) != 0 {
		t.Errorf("expected 0 watchers, got %d", len(result.Watchers))
	}
}

func TestExpandRepoSources_PreservesNonRepoSources(t *testing.T) {
	sources := []SourceEntry{
		{Type: "ast", Path: "/some/path"},
		{Type: "git", URL: "https://example.com/repo"},
	}
	result, err := ExpandRepoSources(context.Background(), sources, "/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expanded := result.Sources
	if len(expanded) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(expanded))
	}
	if expanded[0].Type != "ast" || expanded[1].Type != "git" {
		t.Error("non-repo sources should pass through unchanged")
	}
}

func TestExpandRepoSources_MixedSources(t *testing.T) {
	sources := []SourceEntry{
		{Type: "url", URLs: []string{"https://example.com"}},
		// Branch set explicitly so ExpandRepoSources doesn't try a live
		// `git ls-remote` against a fake URL in unit tests.
		{Type: "repo", URL: "https://github.com/example/repo", Branch: "main"},
		{Type: "ast", Path: "."},
	}
	result, err := ExpandRepoSources(context.Background(), sources, "/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expanded := result.Sources
	// 1 (url) + 4 (repo expanded) + 1 (ast) = 6
	if len(expanded) != 6 {
		t.Fatalf("expected 6 sources, got %d", len(expanded))
	}
	if expanded[0].Type != "url" {
		t.Errorf("first should be url, got %q", expanded[0].Type)
	}
	if expanded[1].Type != "git" {
		t.Errorf("second should be git (from repo expansion), got %q", expanded[1].Type)
	}
	if expanded[5].Type != "ast" {
		t.Errorf("last should be ast, got %q", expanded[5].Type)
	}
}

func TestExpandRepoSources_RequiresURLOrPath(t *testing.T) {
	sources := []SourceEntry{
		{Type: "repo"},
	}
	_, err := ExpandRepoSources(context.Background(), sources, "/tmp/workspace")
	if err == nil {
		t.Fatal("expected error for repo without URL or path")
	}
}

func TestExpandRepoSources_LanguagePropagation(t *testing.T) {
	sources := []SourceEntry{
		// Branch set explicitly so ExpandRepoSources doesn't try a live
		// `git ls-remote` against a fake URL in unit tests.
		{Type: "repo", URL: "https://github.com/example/repo", Language: "python", Branch: "main"},
	}
	result, err := ExpandRepoSources(context.Background(), sources, "/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range result.Sources {
		if s.Type == "ast" {
			if s.Language != "python" {
				t.Errorf("ast language = %q, want python", s.Language)
			}
			return
		}
	}
	t.Error("no ast entry found in expanded sources")
}

// Default-branch resolution moved from ExpandRepoSources to
// sourcespawn.gitComponentConfig (the leaf where Branch is consumed) so it
// covers every code path uniformly. End-to-end coverage of the curator
// add_source_repo flow lives in
// internal/sourcespawn/sourcespawn_test.go: TestAdd_RepoNoBranch_ResolvesRemoteDefault.

func TestExpandRepoSources_LocalRepoPath(t *testing.T) {
	sources := []SourceEntry{
		{Type: "repo", Path: "/home/user/projects/my-app", Watch: true},
	}
	result, err := ExpandRepoSources(context.Background(), sources, "/tmp/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expanded := result.Sources
	if len(expanded) != 4 {
		t.Fatalf("expected 4 expanded sources, got %d", len(expanded))
	}
	if expanded[0].Path != "/home/user/projects/my-app" {
		t.Errorf("git path = %q, want /home/user/projects/my-app", expanded[0].Path)
	}
	if expanded[1].Path != "/home/user/projects/my-app" {
		t.Errorf("ast path = %q, want /home/user/projects/my-app", expanded[1].Path)
	}
}

func gitRepoWithBranches(t *testing.T, branches ...string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("commit", "--allow-empty", "-m", "init")
	for _, b := range branches {
		run("branch", b)
	}
	return dir
}

// TestExpandRepoSources_DiscoveryStates pins task 3.4: discovery never fails
// expansion; each multi-branch source reports a typed state.
func TestExpandRepoSources_DiscoveryStates(t *testing.T) {
	ctx := context.Background()
	t.Run("ok", func(t *testing.T) {
		repo := gitRepoWithBranches(t, "feature/a")
		res, err := ExpandRepoSources(ctx, []SourceEntry{{Type: "repo", Path: repo, Branches: []string{"feature/*"}}}, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if res.Discovery[0].State != DiscoveryOK || len(res.Discovered) != 4 || len(res.Sources) != 4 {
			t.Fatalf("state %+v, discovered %d, sources %d; want ok with one branch's four entries",
				res.Discovery[0], len(res.Discovered), len(res.Sources))
		}
	})
	t.Run("awaiting_checkout", func(t *testing.T) {
		src := SourceEntry{Type: "repo", URL: "https://example.invalid/acme/r.git", Branches: []string{"feature/*"}}
		res, err := ExpandRepoSources(ctx, []SourceEntry{src}, t.TempDir())
		if err != nil {
			t.Fatalf("expansion failed boot: %v", err)
		}
		if res.Discovery[0].State != DiscoveryAwaitingCheckout || len(res.Discovered) != 0 {
			t.Fatalf("state %+v, discovered %d; want awaiting_checkout with nothing discovered", res.Discovery[0], len(res.Discovered))
		}
		if len(res.Sources) != 1 || res.Sources[0].Type != "git" || res.Sources[0].URL != src.URL {
			t.Fatalf("sources = %+v; want only the clone's git source", res.Sources)
		}
	})
	t.Run("listing_failed", func(t *testing.T) {
		res, err := ExpandRepoSources(ctx, []SourceEntry{{Type: "repo", Path: t.TempDir(), Branches: []string{"feature/*"}}}, t.TempDir())
		if err != nil {
			t.Fatalf("expansion failed boot: %v", err)
		}
		st := res.Discovery[0]
		if st.State != DiscoveryListingFailed || st.Error == "" || len(res.Sources) != 0 {
			t.Fatalf("state %+v, sources %d; want listing_failed with an error and nothing expanded", st, len(res.Sources))
		}
	})
	t.Run("capped", func(t *testing.T) {
		repo := gitRepoWithBranches(t, "feature/a", "feature/b", "feature/c")
		res, err := ExpandRepoSources(ctx, []SourceEntry{{Type: "repo", Path: repo, Branches: []string{"feature/*"}, MaxBranches: 1}}, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if st := res.Discovery[0]; st.State != DiscoveryCapped || st.NotAdmitted != 2 || len(res.Discovered) != 4 {
			t.Fatalf("state %+v, discovered %d; want capped with 2 not admitted and one branch expanded", st, len(res.Discovered))
		}
	})
}
