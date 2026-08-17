package subwatch_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourcespawn"
	"github.com/c360studio/semsource/internal/subwatch"
	"github.com/c360studio/semsource/workspace"
)

type fakeStore struct {
	mu       sync.Mutex
	puts     map[string]types.ComponentConfig
	putCalls int
	deletes  []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{puts: map[string]types.ComponentConfig{}}
}

func (f *fakeStore) PutComponentToKV(_ context.Context, name string, cfg types.ComponentConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.puts[name] = cfg
	f.putCalls++
	return nil
}

func (f *fakeStore) DeleteComponentFromKV(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes = append(f.deletes, name)
	return nil
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "protocol.file.allow=always"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func newRepoWithFile(t *testing.T, name, file, body string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-m", "init")
	return dir
}

// dualPinClone mirrors the semdev-test fixture shape: one submodule repo
// linked twice at DIFFERENT pinned commits (root path @ v2, nested path @
// v1). Returns the recursive clone plus both SHAs.
func dualPinClone(t *testing.T) (clone, sub, shaV1, shaV2 string) {
	t.Helper()
	sub = newRepoWithFile(t, "sub", "greeter.go", "package greeter\n\nfunc Greet() {}\n")
	shaV1 = gitIn(t, sub, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(sub, "farewell.go"),
		[]byte("package greeter\n\nfunc Farewell() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, sub, "add", ".")
	gitIn(t, sub, "commit", "-m", "v2")
	shaV2 = gitIn(t, sub, "rev-parse", "HEAD")

	parent := newRepoWithFile(t, "parent", "main.go", "package main\n\nfunc main() {}\n")
	gitIn(t, parent, "submodule", "add", sub, "sub")
	gitIn(t, parent, "submodule", "add", sub, "nested/sub")
	gitIn(t, filepath.Join(parent, "nested/sub"), "checkout", shaV1)
	gitIn(t, parent, "add", "nested/sub")
	gitIn(t, parent, "commit", "-m", "dual pin")

	clone = filepath.Join(t.TempDir(), "clone")
	gitIn(t, filepath.Dir(clone), "clone", "--recurse-submodules", parent, clone)
	return clone, sub, shaV1, shaV2
}

func watcherFor(clone string, store *fakeStore) *subwatch.Watcher {
	return subwatch.New(subwatch.Config{
		RepoPath:   clone,
		ParentSlug: "parentproj",
		Languages:  []string{"go"},
		Watch:      false,
		Opts:       sourcespawn.Options{Org: "acme", WorkspaceDir: "/tmp/ws"},
		Store:      store,
	})
}

func TestTick_ExpandsDualPinWithCanonicalIdentityAndScopedInstances(t *testing.T) {
	clone, sub, shaV1, shaV2 := dualPinClone(t)
	store := newFakeStore()
	w := watcherFor(clone, store)

	w.Tick(context.Background())

	// Two pins × three families.
	if len(store.puts) != 6 {
		names := make([]string, 0, len(store.puts))
		for n := range store.puts {
			names = append(names, n)
		}
		t.Fatalf("got %d component configs %v, want 6", len(store.puts), names)
	}

	// The stored project goes through entityid slugification (long slugs are
	// hash-capped), so expectations must apply the same transform.
	project := entityid.SystemSlug(workspace.URLToSlug(sub))
	var astSeen int
	for name, cc := range store.puts {
		if !strings.Contains(name, "via-parentproj") {
			t.Errorf("instance %q lacks the parent-scope suffix", name)
		}
		if cc.Name != "ast-source" {
			continue
		}
		astSeen++
		var cfg struct {
			WatchPaths []struct {
				Project string `json:"project"`
				Version string `json:"version"`
			} `json:"watch_paths"`
		}
		if err := json.Unmarshal(cc.Config, &cfg); err != nil {
			t.Fatal(err)
		}
		if len(cfg.WatchPaths) != 1 {
			t.Fatalf("instance %q has %d watch paths", name, len(cfg.WatchPaths))
		}
		wp := cfg.WatchPaths[0]
		// Entity identity is canonical: project from the resolved URL, no
		// parent contribution; version is the 12-hex gitlink prefix.
		if wp.Project != project {
			t.Errorf("%s: project = %q, want canonical %q", name, wp.Project, project)
		}
		if wp.Version != shaV1[:12] && wp.Version != shaV2[:12] {
			t.Errorf("%s: version = %q, want a 12-hex gitlink prefix", name, wp.Version)
		}
	}
	if astSeen != 2 {
		t.Errorf("got %d ast instances, want 2 (one per pin)", astSeen)
	}

	// Second tick: no new instances, but ONE confirm re-put per component
	// (defense against the framework watcher dropping a burst event).
	before, callsBefore := len(store.puts), store.putCalls
	w.Tick(context.Background())
	if len(store.puts) != before || len(store.deletes) != 0 {
		t.Errorf("no-change tick mutated instances: puts %d→%d deletes %v",
			before, len(store.puts), store.deletes)
	}
	if store.putCalls != callsBefore+before {
		t.Errorf("confirm tick made %d re-puts, want %d", store.putCalls-callsBefore, before)
	}

	// Third tick: fully quiet.
	callsBefore = store.putCalls
	w.Tick(context.Background())
	if store.putCalls != callsBefore {
		t.Errorf("post-confirm tick still re-putting (%d calls)", store.putCalls-callsBefore)
	}
}

func TestTick_MovedGitlinkIsAVersionTransition(t *testing.T) {
	clone, _, _, shaV2 := dualPinClone(t)
	store := newFakeStore()
	w := watcherFor(clone, store)
	w.Tick(context.Background())
	store.mu.Lock()
	initial := len(store.puts)
	store.mu.Unlock()

	// Move the nested pin v1 → v2 in the clone (what a pull sync produces).
	gitIn(t, filepath.Join(clone, "nested/sub"), "fetch", "origin")
	gitIn(t, filepath.Join(clone, "nested/sub"), "checkout", shaV2)
	gitIn(t, clone, "add", "nested/sub")

	w.Tick(context.Background())

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deletes) != 3 {
		t.Errorf("deletes = %v, want the 3 old-version instances removed", store.deletes)
	}
	for _, name := range store.deletes {
		if !strings.Contains(name, "via-parentproj") {
			t.Errorf("deleted %q is not a parent-scoped submodule instance", name)
		}
	}
	if len(store.puts) <= initial {
		t.Errorf("no new-version instances spawned (puts %d → %d)", initial, len(store.puts))
	}
}

func TestTick_MissingCheckoutIsQuietAndRetriable(t *testing.T) {
	store := newFakeStore()
	w := subwatch.New(subwatch.Config{
		RepoPath: filepath.Join(t.TempDir(), "not-cloned-yet"),
		Opts:     sourcespawn.Options{Org: "acme"},
		Store:    store,
	})
	w.Tick(context.Background())
	if len(store.puts) != 0 || len(store.deletes) != 0 {
		t.Errorf("tick on missing checkout mutated state: %v %v", store.puts, store.deletes)
	}
}
