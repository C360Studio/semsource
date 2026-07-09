package sourcespawn

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/c360studio/semsource/config"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/types"
)

// fakeStore wraps an in-memory SafeConfig and mirrors targeted component writes
// into `puts` so assertions can inspect what was committed. `deletes` records
// DeleteComponentFromKV calls.
type fakeStore struct {
	cfg     *semconfig.SafeConfig
	puts    map[string]types.ComponentConfig
	deletes []string
	failPut bool // fails PutComponentToKV
	failDel bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		cfg: semconfig.NewSafeConfig(&semconfig.Config{
			Platform:   semconfig.PlatformConfig{Org: "test", ID: "test"},
			Components: map[string]types.ComponentConfig{},
		}),
		puts: make(map[string]types.ComponentConfig),
	}
}

func (f *fakeStore) GetConfig() *semconfig.SafeConfig { return f.cfg }

func (f *fakeStore) PutComponentToKV(_ context.Context, name string, compConfig types.ComponentConfig) error {
	if f.failPut {
		return errors.New("kv unavailable")
	}
	cfg := f.cfg.Get()
	if cfg.Components == nil {
		cfg.Components = map[string]types.ComponentConfig{}
	}
	cfg.Components[name] = compConfig
	if err := f.cfg.Update(cfg); err != nil {
		return err
	}
	f.puts[name] = compConfig
	return nil
}

func (f *fakeStore) DeleteComponentFromKV(_ context.Context, name string) error {
	if f.failDel {
		return errors.New("kv unavailable")
	}
	f.deletes = append(f.deletes, name)
	return nil
}

// fakeChecker reports whether a name is in its known set.
type fakeChecker struct {
	known map[string]bool
}

func (f fakeChecker) HasComponent(name string) bool { return f.known[name] }

func TestAdd_FlatTypes_WriteOneComponent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		src         config.SourceEntry
		factoryName string
		wantPrefix  string
	}{
		{
			name:        "git",
			src:         config.SourceEntry{Type: "git", URL: "https://github.com/acme/foo"},
			factoryName: "git-source",
			wantPrefix:  "git-source-",
		},
		{
			name:        "ast",
			src:         config.SourceEntry{Type: "ast", Path: "./src", Language: "go"},
			factoryName: "ast-source",
			wantPrefix:  "ast-source-",
		},
		{
			name:        "docs",
			src:         config.SourceEntry{Type: "docs", Paths: []string{"./docs"}},
			factoryName: "doc-source",
			wantPrefix:  "doc-source-",
		},
		{
			name:        "config",
			src:         config.SourceEntry{Type: "config", Paths: []string{"./go.mod"}},
			factoryName: "cfgfile-source",
			wantPrefix:  "cfgfile-source-",
		},
		{
			name:        "url",
			src:         config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}},
			factoryName: "url-source",
			wantPrefix:  "url-source-",
		},
		{
			name:        "image",
			src:         config.SourceEntry{Type: "image", Paths: []string{"./assets"}},
			factoryName: "image-source",
			wantPrefix:  "image-source-",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newFakeStore()
			results, err := Add(context.Background(), tc.src, store, Options{Org: "acme"})
			if err != nil {
				t.Fatalf("Add: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}
			r := results[0]
			if r.FactoryName != tc.factoryName {
				t.Errorf("FactoryName = %q, want %q", r.FactoryName, tc.factoryName)
			}
			if !strings.HasPrefix(r.InstanceName, tc.wantPrefix) {
				t.Errorf("InstanceName = %q, want prefix %q", r.InstanceName, tc.wantPrefix)
			}
			if !r.Created {
				t.Error("Created = false; expected true with no checker")
			}
			if _, ok := store.puts[r.InstanceName]; !ok {
				t.Errorf("expected KV put for %q, got %v", r.InstanceName, store.puts)
			}
		})
	}
}

func TestAdd_RepoSingleBranch_Expands(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	src := config.SourceEntry{
		Type:   "repo",
		URL:    "https://github.com/acme/foo",
		Branch: "main",
	}
	results, err := Add(context.Background(), src, store, Options{Org: "acme", WorkspaceDir: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Repo expands to git + ast + docs + config = 4 components.
	if len(results) != 4 {
		t.Fatalf("expected 4 results from repo expansion, got %d: %+v", len(results), results)
	}
	wantFactories := map[string]bool{
		"git-source":     false,
		"ast-source":     false,
		"doc-source":     false,
		"cfgfile-source": false,
	}
	for _, r := range results {
		if _, ok := wantFactories[r.FactoryName]; !ok {
			t.Errorf("unexpected factory %q", r.FactoryName)
			continue
		}
		wantFactories[r.FactoryName] = true
	}
	for f, seen := range wantFactories {
		if !seen {
			t.Errorf("expected factory %q in results", f)
		}
	}
	if len(store.puts) != 4 {
		t.Errorf("expected 4 KV puts, got %d", len(store.puts))
	}
}

// TestAdd_RepoNoBranch_ResolvesRemoteDefault is the load-bearing test for
// the curator workflow: an AddRequest with Type="repo" and no Branch must
// resolve the remote's actual default branch and stamp it on the spawned
// git-source component's KV config. The hardcoded "main" fallback in
// gitComponentConfig:127 must NOT fire for repo-type adds whose remote
// default is something else (master / trunk / develop / etc.) — that was
// the original bug for osh-core and other pre-rename repos.
//
// Uses a real local git repo with HEAD on "master" so the test exercises
// the live ResolveDefaultBranch → ExpandRepoSources → buildSpecs →
// gitComponentConfig chain end-to-end, with no mocks of the resolution
// layer.
func TestAdd_RepoNoBranch_ResolvesRemoteDefault(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	// Create a local repo with HEAD pinned to "master" (pre-rename style).
	repoPath := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		cmd.Env = append(cmd.Env,
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
			"HOME="+t.TempDir(), "PATH=/usr/bin:/bin:/usr/local/bin",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "master")
	run("commit", "--allow-empty", "-m", "init")

	store := newFakeStore()
	src := config.SourceEntry{
		Type: "repo",
		URL:  repoPath, // local path works for git ls-remote
		// Branch deliberately empty — the curator hits this path.
	}
	results, err := Add(context.Background(), src, store, Options{
		Org:          "acme",
		WorkspaceDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Find the git-source result and inspect its KV config.
	var gitInstance string
	for _, r := range results {
		if r.FactoryName == "git-source" {
			gitInstance = r.InstanceName
			break
		}
	}
	if gitInstance == "" {
		t.Fatal("no git-source component in Add results")
	}

	gitCfg, ok := store.puts[gitInstance]
	if !ok {
		t.Fatalf("git-source instance %q has no KV write", gitInstance)
	}
	var compConfig map[string]any
	if err := json.Unmarshal(gitCfg.Config, &compConfig); err != nil {
		t.Fatalf("decode git-source config: %v", err)
	}
	branch, _ := compConfig["branch"].(string)
	if branch != "master" {
		t.Errorf("git-source branch = %q, want %q — the hardcoded \"main\" fallback fired for a curator-style repo add even though ls-remote should have resolved \"master\"",
			branch, "master")
	}
}

// TestAdd_GitDirect_NoBranch_ResolvesRemoteDefault covers the path where
// the curator (or any direct caller) submits a flat Type="git" source
// instead of "repo". Type="git" entries skip ExpandRepoSources's
// repo-expansion branch but still need default-branch resolution —
// otherwise the silent "main" fallback in components.go would re-bite us
// for non-main remotes.
func TestAdd_GitDirect_NoBranch_ResolvesRemoteDefault(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repoPath := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		cmd.Env = append(cmd.Env,
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
			"HOME="+t.TempDir(), "PATH=/usr/bin:/bin:/usr/local/bin",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "trunk")
	run("commit", "--allow-empty", "-m", "init")

	store := newFakeStore()
	src := config.SourceEntry{
		Type: "git",
		URL:  repoPath,
		// Branch deliberately empty.
	}
	results, err := Add(context.Background(), src, store, Options{
		Org:          "acme",
		WorkspaceDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for flat git source, got %d", len(results))
	}

	gitCfg := store.puts[results[0].InstanceName]
	var compConfig map[string]any
	if err := json.Unmarshal(gitCfg.Config, &compConfig); err != nil {
		t.Fatalf("decode git-source config: %v", err)
	}
	branch, _ := compConfig["branch"].(string)
	if branch != "trunk" {
		t.Errorf("git-source branch = %q, want %q — default-branch resolution did not run on a direct Type:\"git\" add",
			branch, "trunk")
	}
}

// TestAdd_GitDirect_NoBranch_ResolutionFails_LeavesBranchEmpty confirms
// the new behavior when ls-remote can't resolve a default (URL points at
// something that isn't a git repo, network is down, etc.): branch stays
// empty in the component config. workspace.clone omits --branch and git
// uses the remote's actual default — strictly better than force-cloning
// "main" and breaking pre-rename repos. This is the explicit replacement
// for the silent hardcoded "main" fallback the user flagged at
// components.go:127.
func TestAdd_GitDirect_NoBranch_ResolutionFails_LeavesBranchEmpty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// Point at a directory that exists but isn't a git repo — ls-remote
	// fails fast against it.
	notARepo := t.TempDir()

	store := newFakeStore()
	src := config.SourceEntry{
		Type:   "git",
		URL:    notARepo,
		Branch: "",
	}
	results, err := Add(context.Background(), src, store, Options{
		Org:          "acme",
		WorkspaceDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	gitCfg := store.puts[results[0].InstanceName]
	var compConfig map[string]any
	if err := json.Unmarshal(gitCfg.Config, &compConfig); err != nil {
		t.Fatalf("decode git-source config: %v", err)
	}
	branch, _ := compConfig["branch"].(string)
	if branch != "" {
		t.Errorf("git-source branch = %q, want empty — the silent \"main\" fallback fired when resolution failed", branch)
	}
}

// TestAdd_RepoBranchSlug_Propagates is a regression test for the branch-watcher
// path: when sourcespawn.Build is called with a "repo" entry carrying a
// BranchSlug, every expanded child must carry that slug so per-branch instance
// names stay distinct in KV.
func TestAdd_RepoBranchSlug_Propagates(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	src := config.SourceEntry{
		Type:       "repo",
		Path:       "/tmp/worktrees/scenario-auth-flow",
		Branch:     "scenario/auth-flow",
		BranchSlug: "scenario-auth-flow",
	}
	results, err := Add(context.Background(), src, store, Options{Org: "acme", WorkspaceDir: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	for _, r := range results {
		if !strings.HasSuffix(r.InstanceName, "-scenario-auth-flow") {
			t.Errorf("InstanceName = %q for factory %q; expected suffix \"-scenario-auth-flow\"",
				r.InstanceName, r.FactoryName)
		}
	}
}

func TestAdd_RepoMultiBranch_Unsupported(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	src := config.SourceEntry{
		Type:     "repo",
		URL:      "https://github.com/acme/foo",
		Branches: []string{"*"},
	}
	_, err := Add(context.Background(), src, store, Options{Org: "acme"})
	if CodeOf(err) != CodeUnsupportedType {
		t.Fatalf("CodeOf = %q, want %q (err=%v)", CodeOf(err), CodeUnsupportedType, err)
	}
	if len(store.puts) != 0 {
		t.Errorf("expected no KV writes on unsupported, got %d", len(store.puts))
	}
}

func TestAdd_ValidationFails_NoKVWrite(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	src := config.SourceEntry{Type: "git"} // missing URL
	_, err := Add(context.Background(), src, store, Options{Org: "acme"})
	if CodeOf(err) != CodeValidationFailed {
		t.Fatalf("CodeOf = %q, want %q", CodeOf(err), CodeValidationFailed)
	}
	if len(store.puts) != 0 {
		t.Errorf("expected no KV writes on validation failure, got %d", len(store.puts))
	}
}

func TestAdd_KVFailure_ReportsKVCode(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.failPut = true
	src := config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}
	_, err := Add(context.Background(), src, store, Options{Org: "acme"})
	if CodeOf(err) != CodeKVWriteFailed {
		t.Fatalf("CodeOf = %q, want %q", CodeOf(err), CodeKVWriteFailed)
	}
}

// TestAdd_PutFailure_ReportsNoSingleWrite verifies that a targeted component
// write failure surfaces as a KV-write error before reporting the failed
// component as committed.
func TestAdd_PutFailure_ReportsNoSingleWrite(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.failPut = true
	src := config.SourceEntry{Type: "docs", Paths: []string{"/tmp/docs"}}
	results, err := Add(context.Background(), src, store, Options{Org: "acme", WorkspaceDir: "/tmp/ws"})
	if err == nil {
		t.Fatal("Add returned nil error; want KV failure")
	}
	if CodeOf(err) != CodeKVWriteFailed {
		t.Errorf("CodeOf = %q, want %q", CodeOf(err), CodeKVWriteFailed)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
	if len(store.puts) != 0 {
		t.Errorf("len(store.puts) = %d, want 0", len(store.puts))
	}
}

func TestAddWithChecker_DetectsExisting(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	src := config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}

	// First add: no checker, Created should be true.
	first, err := Add(context.Background(), src, store, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if !first[0].Created {
		t.Fatal("first Created = false; want true")
	}

	// Second add with a checker that knows the instance exists.
	checker := fakeChecker{known: map[string]bool{first[0].InstanceName: true}}
	second, err := AddWithChecker(context.Background(), src, store, checker, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("second Add: %v", err)
	}
	if second[0].Created {
		t.Error("second Created = true; want false (existing instance)")
	}
}

func TestRemove_DeletesByInstanceName(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	if err := Remove(context.Background(), "url-source-example-com", store); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(store.deletes) != 1 || store.deletes[0] != "url-source-example-com" {
		t.Errorf("deletes = %v, want [url-source-example-com]", store.deletes)
	}
}

func TestRemove_EmptyName_Fails(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	err := Remove(context.Background(), "", store)
	if CodeOf(err) != CodeValidationFailed {
		t.Fatalf("CodeOf = %q, want %q", CodeOf(err), CodeValidationFailed)
	}
}

func TestBuild_EquivalentToAdd_NoKVWrite(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	src := config.SourceEntry{Type: "url", URLs: []string{"https://example.com"}}

	// Build is the marshal-only path; verify it produces the same instance
	// names as Add but never touches the store.
	built, err := Build(src, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(built) != 1 {
		t.Fatalf("len(built) = %d, want 1", len(built))
	}

	results, err := Add(context.Background(), src, store, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, ok := built[results[0].InstanceName]; !ok {
		t.Errorf("Build did not produce instance %q (got %v)", results[0].InstanceName, mapKeys(built))
	}
}

// TestAdd_EmptySlugFallback_IsContentStable guards H3: when the natural
// identifier yields an empty SystemSlug (e.g., a path of "."), the fallback
// must be a deterministic function of source content, not of position. Two
// equivalent SourceEntries — added from any context — must land on the same
// KV key.
func TestAdd_EmptySlugFallback_IsContentStable(t *testing.T) {
	t.Parallel()
	// "./" yields an empty SystemSlug; the fallback path is exercised.
	src := config.SourceEntry{Type: "docs", Paths: []string{"./"}}

	storeA := newFakeStore()
	resA, err := Add(context.Background(), src, storeA, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("Add A: %v", err)
	}
	storeB := newFakeStore()
	resB, err := Add(context.Background(), src, storeB, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("Add B: %v", err)
	}
	if resA[0].InstanceName != resB[0].InstanceName {
		t.Errorf("Add of equivalent sources produced different instance names: %q vs %q",
			resA[0].InstanceName, resB[0].InstanceName)
	}

	// Build (loader path) must match Add (programmatic path) for the same input.
	built, err := Build(src, Options{Org: "acme"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := built[resA[0].InstanceName]; !ok {
		t.Errorf("Build produced %v; expected %q", mapKeys(built), resA[0].InstanceName)
	}
}

func mapKeys(m map[string]types.ComponentConfig) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
