package main

import (
	"context"
	"io"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	semconfig "github.com/c360studio/semengine/config"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
)

// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestCheckBootCollisions(t *testing.T) {
	checkout, sha := pinnedCheckout(t, "pinned")
	elsewhere, _ := pinnedCheckout(t, "other")
	pin := func(parent string) config.SourceEntry {
		return config.SourceEntry{Type: "ast", Path: "/" + parent + "/lib", Project: "github-com-acme-lib",
			Version: sha[:12], InstanceSuffix: "via-" + parent + "-lib"}
	}
	operator := config.SourceEntry{Type: "ast", Path: checkout, Project: "github-com-acme-lib", Version: sha[:12]}
	moved := operator
	moved.Path = elsewhere
	cases := []struct {
		name    string
		entries []config.SourceEntry
		pins    int // the first pins entries are filed as submodule pins
		refused bool
	}{
		{"two pins of one SHA", []config.SourceEntry{pin("p1"), pin("p2")}, 2, false},
		{"a pin and a standalone source at its identity", []config.SourceEntry{pin("p1"), operator}, 1, false},
		{"a pin and a standalone checkout at another commit", []config.SourceEntry{pin("p1"), moved}, 1, true},
		{"pins the record does not name are operator sources", []config.SourceEntry{pin("p1"), pin("p2")}, 0, true},
		{"one doc source over two roots of one project", []config.SourceEntry{{Type: "docs", Paths: []string{"/r/a", "/r/b"}, Project: "svc"}}, 0, true},
		{"a doc source and an object-store source under one project", []config.SourceEntry{
			{Type: "docs", Paths: []string{"/r/a"}, Project: "svc"}, {Type: "s3", Bucket: "artifacts", Project: "svc"},
		}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			components := semconfig.ComponentConfigs{}
			var pinned []string
			for i, e := range tc.entries {
				built, err := sourcespawn.Build(e, reconcileOpts)
				if err != nil {
					t.Fatal(err)
				}
				for name, cc := range built {
					components[name] = cc
					if i < tc.pins {
						pinned = append(pinned, name)
					}
				}
			}
			components["source-manifest"] = manifestWith(t, nil, []sourcemanifest.DiscoveredRecord{
				{Parent: "/p", Kind: "submodule", Components: pinned},
			})
			err := checkBootCollisions(context.Background(), components)
			if tc.refused != (err != nil) {
				t.Fatalf("refused = %v, want %v (err %v)", err != nil, tc.refused, err)
			}
			if err != nil && !strings.Contains(err.Error(), "refusing to boot") {
				t.Errorf("boot refusal %q does not say boot stopped", err)
			}
		})
	}
}

// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestLoadAndExpandConfigRefusesSourcesSharingASystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "semsource.json")
	body := `{"namespace":"acme","sources":[` +
		`{"type":"ast","path":"./a","project":"svc"},{"type":"ast","path":"./b","project":"svc"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := loadAndExpandConfig(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), `system "svc"`) {
		t.Fatalf("run accepted two ast sources under one project: err = %v", err)
	}
}

// spec: openspec/changes/refuse-shared-source-system/specs/entity-identity-safety/spec.md#Requirement: Sources that would share an entity-ID system are refused
func TestReconcileDiscoveryRefusesABranchReplacingARetainedSource(t *testing.T) {
	retainedEntry := config.SourceEntry{Type: "ast", Path: "/retained/standalone", Project: "svc-feature"}
	discoveredEntry := config.SourceEntry{Type: "ast", Path: "/discovered/worktree", Project: "svc", BranchSlug: "feature"}
	retained, err := sourcespawn.Build(retainedEntry, reconcileOpts)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	planOf := func(t *testing.T, entry config.SourceEntry) *discoveryPlan {
		t.Helper()
		plan := &discoveryPlan{desired: semconfig.ComponentConfigs{}, entries: map[string]config.SourceEntry{}}
		if err := plan.add(plan.owner("/discovered", "branch", false), entry, reconcileOpts); err != nil {
			t.Fatal(err)
		}
		return plan
	}

	t.Run("a discovered branch building a retained source's instance from another root", func(t *testing.T) {
		plan := planOf(t, discoveredEntry)
		if got, want := slices.Sorted(maps.Keys(plan.desired)), slices.Sorted(maps.Keys(retained)); !slices.Equal(got, want) {
			t.Fatalf("fixture drift: discovered %v and retained %v build different instances", got, want)
		}
		components := maps.Clone(retained)
		components["source-manifest"] = manifestWith(t, nil, nil)
		before := maps.Clone(components)
		store := &reconcileStore{components: components}
		err := reconcileDiscovery(context.Background(), store, plan, nil, reconcileOpts, logger)
		if err == nil {
			t.Fatal("a discovered branch replacing a retained source was admitted")
		}
		for _, want := range []string{"refusing to boot", "(root /retained/standalone)", "(root /discovered/worktree)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("refusal %q does not name %q", err, want)
			}
		}
		if len(store.puts)+len(store.dels) != 0 {
			t.Errorf("a refused discovery wrote puts=%v deletes=%v", store.puts, store.dels)
		}
		for name, want := range before {
			if got := store.components[name]; !got.Equal(want) {
				t.Errorf("%s changed: %s, want %s", name, got.Config, want.Config)
			}
		}
	})

	t.Run("a refresh of the same discovered root is admitted", func(t *testing.T) {
		plan := planOf(t, discoveredEntry)
		components := maps.Clone(plan.desired)
		components["source-manifest"] = manifestWith(t, nil, []sourcemanifest.DiscoveredRecord{
			{Parent: "/discovered", Kind: "branch", Components: slices.Collect(maps.Keys(plan.desired))},
		})
		store := &reconcileStore{components: components}
		if err := reconcileDiscovery(context.Background(), store, plan, nil, reconcileOpts, logger); err != nil {
			t.Fatalf("a refresh of a discovered root was refused: %v", err)
		}
	})

	t.Run("a discovered pin and a retained standalone source", func(t *testing.T) {
		checkout, sha := pinnedCheckout(t, "pinned")
		elsewhere, _ := pinnedCheckout(t, "other")
		pinEntry := config.SourceEntry{Type: "ast", Path: "/parent/lib", Project: "github-com-acme-lib",
			Version: sha[:12], InstanceSuffix: "via-parent-lib"}
		for _, tc := range []struct {
			root    string
			refused bool
		}{{checkout, false}, {elsewhere, true}} {
			standalone, err := sourcespawn.Build(config.SourceEntry{Type: "ast", Path: tc.root,
				Project: "github-com-acme-lib", Version: sha[:12]}, reconcileOpts)
			if err != nil {
				t.Fatal(err)
			}
			plan := &discoveryPlan{desired: semconfig.ComponentConfigs{}, entries: map[string]config.SourceEntry{}}
			if err := plan.add(plan.owner("/parent", "submodule", false), pinEntry, reconcileOpts); err != nil {
				t.Fatal(err)
			}
			components := maps.Clone(standalone)
			components["source-manifest"] = manifestWith(t, nil, nil)
			store := &reconcileStore{components: components}
			err = reconcileDiscovery(context.Background(), store, plan, nil, reconcileOpts, logger)
			if tc.refused != (err != nil) {
				t.Fatalf("standalone at %s: refused = %v, want %v (err %v)", tc.root, err != nil, tc.refused, err)
			}
			if tc.refused && len(store.puts) != 0 {
				t.Errorf("a refused discovery wrote %v", store.puts)
			}
		}
	})

	t.Run("a runtime source at the discovered root is the same writer", func(t *testing.T) {
		plan := planOf(t, discoveredEntry)
		components := maps.Clone(plan.desired)
		components["source-manifest"] = manifestWith(t, nil, nil)
		store := &reconcileStore{components: components}
		if err := reconcileDiscovery(context.Background(), store, plan, nil, reconcileOpts, logger); err != nil {
			t.Fatalf("a discovered root already registered at the same root was refused: %v", err)
		}
	})
}

// pinnedCheckout is a git repository in a temp dir with one commit whose
// message is msg (distinct messages give distinct commits), and its SHA.
func pinnedCheckout(t *testing.T, msg string) (dir, sha string) {
	t.Helper()
	dir = t.TempDir()
	git := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@e.com", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("commit", "-q", "--allow-empty", "-m", msg)
	return dir, git("rev-parse", "HEAD")
}
