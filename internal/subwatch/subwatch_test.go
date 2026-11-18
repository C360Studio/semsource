package subwatch_test

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/types"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourcespawn"
	"github.com/c360studio/semsource/internal/subwatch"
	"github.com/c360studio/semsource/workspace"
)

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

var expandOpts = sourcespawn.Options{Org: "acme", WorkspaceDir: "/tmp/ws"}

// expandBuilt runs boot-time expansion for the clone and builds every entry
// into its component configs, as boot reconcile does.
func expandBuilt(t *testing.T, clone string) (map[string]types.ComponentConfig, map[string]string) {
	t.Helper()
	entries, pins, err := subwatch.Expand(context.Background(), subwatch.Config{
		RepoPath: clone, ParentSlug: "parentproj", Languages: []string{"go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]types.ComponentConfig{}
	for _, entry := range entries {
		built, err := sourcespawn.Build(entry, expandOpts)
		if err != nil {
			t.Fatal(err)
		}
		maps.Copy(out, built)
	}
	return out, pins
}

func TestExpand_DualPinWithCanonicalIdentityAndScopedInstances(t *testing.T) {
	clone, sub, shaV1, shaV2 := dualPinClone(t)
	built, _ := expandBuilt(t, clone)

	// Two pins × three families.
	if len(built) != 6 {
		t.Fatalf("got %d component configs %v, want 6", len(built), slices.Sorted(maps.Keys(built)))
	}

	// The stored project goes through entityid slugification (long slugs are
	// hash-capped), so expectations must apply the same transform.
	project := entityid.SystemSlug(workspace.URLToSlug(sub))
	var astSeen int
	for name, cc := range built {
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
}

// TestExpand_MovedGitlinkIsAVersionTransition: a moved pin expands to new
// instance names and drops the old version's, so the next boot's reconcile
// admits the new version and retires the old one (both are parent-owned).
func TestExpand_MovedGitlinkIsAVersionTransition(t *testing.T) {
	clone, _, _, shaV2 := dualPinClone(t)
	before, _ := expandBuilt(t, clone)

	// Move the nested pin v1 → v2 in the clone (what a pull sync produces).
	gitIn(t, filepath.Join(clone, "nested/sub"), "fetch", "origin")
	gitIn(t, filepath.Join(clone, "nested/sub"), "checkout", shaV2)
	gitIn(t, clone, "add", "nested/sub")

	after, pins := expandBuilt(t, clone)
	if !strings.HasPrefix(pins["nested/sub"], shaV2+" ") {
		t.Fatalf("pins = %v; want nested/sub at %s", pins, shaV2)
	}
	var gone, added int
	for name := range before {
		if _, ok := after[name]; !ok {
			gone++
			if !strings.Contains(name, "via-parentproj-nested-sub") {
				t.Errorf("retired %q is not the moved pin's instance", name)
			}
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			added++
		}
	}
	if gone != 3 || added != 3 {
		t.Errorf("moved pin: %d names gone, %d added; want the 3 old-version instances replaced", gone, added)
	}
}

// TestExpand_MissingCheckoutExpandsNothing: a checkout that does not exist
// yet has no submodules (boot reports it as awaiting_checkout).
func TestExpand_MissingCheckoutExpandsNothing(t *testing.T) {
	entries, pins, err := subwatch.Expand(context.Background(), subwatch.Config{
		RepoPath: filepath.Join(t.TempDir(), "not-cloned-yet"),
	})
	if err != nil || len(entries) != 0 || len(pins) != 0 {
		t.Errorf("Expand on a missing checkout = %v, %v, %v; want nothing", entries, pins, err)
	}
}

// TestExpand_Pins pins the boot-time path (task 3.3): Pins lists exactly the
// pins Expand built from, and every expanded name carries the parent link.
func TestExpand_Pins(t *testing.T) {
	clone, _, _, _ := dualPinClone(t)
	built, pins := expandBuilt(t, clone)
	if listed, err := subwatch.Pins(context.Background(), clone); err != nil || !maps.Equal(listed, pins) || len(pins) != 2 {
		t.Fatalf("Pins = %v, %v; Expand built from %v (want the two pins)", listed, err, pins)
	}
	for name := range built {
		if !strings.Contains(name, "via-parentproj-") {
			t.Errorf("%s lacks the parent link suffix", name)
		}
	}
}
