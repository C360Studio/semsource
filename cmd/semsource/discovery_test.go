package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	semconfig "github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
	"github.com/c360studio/semsource/workspace"
)

type reconcileStore struct {
	components semconfig.ComponentConfigs
	puts, dels []string
}

func (s *reconcileStore) GetConfig() *semconfig.SafeConfig {
	return semconfig.NewSafeConfig(&semconfig.Config{Components: s.components})
}

func (s *reconcileStore) PutComponentToKV(_ context.Context, name string, cfg types.ComponentConfig) error {
	s.puts = append(s.puts, name)
	s.components[name] = cfg
	return nil
}

func (s *reconcileStore) DeleteComponentFromKV(_ context.Context, name string) error {
	s.dels = append(s.dels, name)
	delete(s.components, name)
	return nil
}

var reconcileOpts = sourcespawn.Options{Org: "acme"}

// docComponent builds the docs component sourcespawn makes for /r/<slug>,
// whose instance name is doc-source-<slug>.
func docComponent(t *testing.T, slug string) (string, config.SourceEntry, types.ComponentConfig) {
	t.Helper()
	entry := config.SourceEntry{Type: "docs", Paths: []string{"/r/" + slug}}
	built, err := sourcespawn.Build(entry, reconcileOpts)
	if err != nil || len(built) != 1 {
		t.Fatalf("build %s: %v %v", slug, built, err)
	}
	for name, cc := range built {
		return name, entry, cc
	}
	return "", entry, types.ComponentConfig{}
}

func manifestOf(t *testing.T, s *reconcileStore) sourcemanifest.Config {
	t.Helper()
	var m sourcemanifest.Config
	if err := json.Unmarshal(s.components["source-manifest"].Config, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func recordOf(t *testing.T, s *reconcileStore) []sourcemanifest.DiscoveredRecord {
	t.Helper()
	return manifestOf(t, s).Discovered
}

func manifestWith(t *testing.T, sources []sourcemanifest.ManifestSource, record []sourcemanifest.DiscoveredRecord) types.ComponentConfig {
	t.Helper()
	raw, err := json.Marshal(sourcemanifest.Config{Namespace: "acme", Sources: sources, Discovered: record})
	if err != nil {
		t.Fatal(err)
	}
	return types.ComponentConfig{Name: "source-manifest", Type: types.ComponentTypeProcessor, Enabled: true, Config: raw}
}

// TestReconcileDiscovery pins task 3.2 against the ownership record: a
// discovered component that is missing is put; a recorded one nothing
// discovers any more is deleted, including one whose parent left the
// configuration; a runtime-disabled one is neither re-enabled nor deleted
// (Q1); a failed parent's recorded names are kept and recorded again; a
// file-generated or unrecorded component is untouched, even one whose name a
// branch could have produced; and the record and manifest are rewritten.
func TestReconcileDiscovery(t *testing.T) {
	newName, newEntry, _ := docComponent(t, "new")
	staleName, staleEntry, staleCC := docComponent(t, "stale")
	goneName, goneEntry, goneCC := docComponent(t, "gone")
	offName, offEntry, offCC := docComponent(t, "off")
	keptName, _, keptCC := docComponent(t, "kept")
	fileName, _, fileCC := docComponent(t, "filegen")
	sibling := "git-source-github-com-acme-app-v2" // a branch "v2" of acme/app would build this name
	disabled := offCC
	disabled.Enabled = false

	prev := []sourcemanifest.DiscoveredRecord{
		{Parent: "/r/a", Kind: "branch", Components: []string{fileName, offName, staleName}},
		{Parent: "/r/b", Kind: "submodule", Components: []string{keptName}},
		{Parent: "/r/removed", Kind: "branch", Components: []string{goneName}},
	}
	store := &reconcileStore{components: semconfig.ComponentConfigs{
		"source-manifest": manifestWith(t, []sourcemanifest.ManifestSource{
			sourcemanifest.ManifestSourceFromEntry(staleEntry), sourcemanifest.ManifestSourceFromEntry(goneEntry),
		}, prev),
		staleName: staleCC, goneName: goneCC, offName: disabled, keptName: keptCC, fileName: fileCC, sibling: staleCC,
	}}
	plan := &discoveryPlan{desired: semconfig.ComponentConfigs{}, entries: map[string]config.SourceEntry{}}
	a := plan.owner("/r/a", "branch", false)
	plan.owner("/r/b", "submodule", true)
	for _, entry := range []config.SourceEntry{newEntry, offEntry} {
		if err := plan.add(a, entry, reconcileOpts); err != nil {
			t.Fatal(err)
		}
	}
	file := semconfig.ComponentConfigs{fileName: fileCC}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := reconcileDiscovery(context.Background(), store, plan, file, reconcileOpts, logger); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.puts, []string{newName, "source-manifest"}) {
		t.Errorf("puts = %v; want the new component, then the manifest", store.puts)
	}
	slices.Sort(store.dels)
	if want := slices.Sorted(slices.Values([]string{staleName, goneName})); !slices.Equal(store.dels, want) {
		t.Errorf("deletes = %v; want the stale and the removed parent's components %v", store.dels, want)
	}
	if store.components[offName].Enabled {
		t.Error("a runtime-disabled discovered component was re-enabled")
	}
	for _, name := range []string{keptName, fileName, sibling} {
		if _, ok := store.components[name]; !ok {
			t.Errorf("%s was removed; want it untouched", name)
		}
	}
	wantRecord := []sourcemanifest.DiscoveredRecord{
		{Parent: "/r/a", Kind: "branch", Components: slices.Sorted(slices.Values([]string{newName, offName}))},
		{Parent: "/r/b", Kind: "submodule", Components: []string{keptName}},
	}
	if got := recordOf(t, store); !reflect.DeepEqual(got, wantRecord) {
		t.Errorf("record = %+v; want %+v", got, wantRecord)
	}
	var paths []string
	for _, src := range manifestOf(t, store).Sources {
		paths = append(paths, src.Paths...)
	}
	if !slices.Equal(paths, []string{"/r/new"}) {
		t.Errorf("manifest sources = %v; want only the new entry", paths)
	}

	store.puts, store.dels = nil, nil
	if err := reconcileDiscovery(context.Background(), store, plan, file, reconcileOpts, logger); err != nil {
		t.Fatal(err)
	}
	if len(store.puts)+len(store.dels) != 0 {
		t.Errorf("a second reconcile wrote puts=%v deletes=%v; want none", store.puts, store.dels)
	}
}

// TestPlanDiscovery_RemoteCloneComponentStays: a remote multi-branch source
// always admits its clone component, so a checkout whose shallow clone lists
// only the default branch keeps ingesting it (owner ruling 2026-11-18).
func TestPlanDiscovery_RemoteCloneComponentStays(t *testing.T) {
	src := config.SourceEntry{Type: "repo", URL: "https://example.com/acme/app.git", Branches: []string{"feature/*"}}
	cfg := &config.Config{Namespace: "acme", WorkspaceDir: t.TempDir(), Sources: []config.SourceEntry{src}}
	checkout := filepath.Join(cfg.WorkspaceDir, workspace.URLToSlug(src.URL))
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@e.com", "commit", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = checkout
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	expand, err := config.ExpandRepoSources(context.Background(), cfg.Sources, cfg.WorkspaceDir)
	if err != nil {
		t.Fatal(err)
	}
	if expand.Discovery[0].State != config.DiscoveryOK || len(expand.Watchers[0].Entries) != 0 {
		t.Fatalf("expansion = %+v; want ok with no matching branch", expand.Discovery[0])
	}
	plan, err := planDiscovery(context.Background(), cfg, expand, reconcileOpts)
	if err != nil {
		t.Fatal(err)
	}
	for name := range instanceNames([]config.SourceEntry{config.ParentCloneEntry(src)}, reconcileOpts) {
		if _, ok := plan.desired[name]; !ok {
			t.Errorf("clone component %s not admitted with the checkout present", name)
		}
	}
}

// instanceNames is the set of component names the entries build.
func instanceNames(entries []config.SourceEntry, opts sourcespawn.Options) map[string]struct{} {
	out := map[string]struct{}{}
	for _, entry := range entries {
		built, err := sourcespawn.InstanceNames(entry, opts)
		if err != nil {
			continue
		}
		for name := range built {
			out[name] = struct{}{}
		}
	}
	return out
}

// TestRuntimeDiscoveryPathsStayDeleted is the task 5.1 grep gate: the runtime
// branch and submodule watchers and their "capability: blocked" warnings are
// gone, and no Go file reintroduces them. Discovery happens at boot only
// (source-discovery spec). The needles are split so this file does not match.
func TestRuntimeDiscoveryPathsStayDeleted(t *testing.T) {
	needles := []string{
		"start" + "BranchWatchers", "start" + "SubmoduleWatchers",
		"publish" + "BranchComponents", "remove" + "BranchComponents",
		"subwatch" + ".New(", "parse" + "BranchPollInterval",
		"dynamic branch discovery " + "unavailable", "dynamic submodule discovery " + "unavailable",
	}
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".worktrees" || d.Name() == "node_modules" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, n := range needles {
			if strings.Contains(string(body), n) {
				t.Errorf("%s reintroduces %q", path, n)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestReconcileDiscovery_AwaitingCheckoutKeepsBranches: a remote multi-branch
// source whose checkout is gone (a wiped workspace on a retained store) keeps
// the branch components admitted before and gets its clone placeholder back,
// so the next boot can rediscover; nothing is deleted.
func TestReconcileDiscovery_AwaitingCheckoutKeepsBranches(t *testing.T) {
	src := config.SourceEntry{Type: "repo", URL: "https://example.com/acme/app.git", Branches: []string{"feature/*"}}
	cfg := &config.Config{Namespace: "acme", WorkspaceDir: t.TempDir(), Sources: []config.SourceEntry{src}}
	expand, err := config.ExpandRepoSources(context.Background(), cfg.Sources, cfg.WorkspaceDir)
	if err != nil {
		t.Fatal(err)
	}
	if expand.Discovery[0].State != config.DiscoveryAwaitingCheckout {
		t.Fatalf("state = %s; want awaiting_checkout", expand.Discovery[0].State)
	}
	cfg.Sources = []config.SourceEntry{src}
	plan, err := planDiscovery(context.Background(), cfg, expand, reconcileOpts)
	if err != nil {
		t.Fatal(err)
	}

	kept := semconfig.ComponentConfigs{}
	entries := config.BranchEntries(src, workspace.BranchState{Branch: "feature/a", Slug: "feature-a",
		WorktreePath: filepath.Join(expand.Watchers[0].WorktreeBase, "feature-a")})
	for _, entry := range entries {
		built, err := sourcespawn.Build(entry, reconcileOpts)
		if err != nil {
			t.Fatal(err)
		}
		maps.Copy(kept, built)
	}
	kept["source-manifest"] = manifestWith(t, nil, []sourcemanifest.DiscoveredRecord{
		{Parent: src.URL, Kind: "branch", Components: slices.Sorted(maps.Keys(kept))},
	})
	store := &reconcileStore{components: maps.Clone(kept)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := reconcileDiscovery(context.Background(), store, plan, semconfig.ComponentConfigs{}, reconcileOpts, logger); err != nil {
		t.Fatal(err)
	}
	if len(store.dels) != 0 {
		t.Errorf("deleted %v; want the admitted branch kept while the checkout is missing", store.dels)
	}
	for name := range instanceNames([]config.SourceEntry{config.ParentCloneEntry(src)}, reconcileOpts) {
		if _, ok := store.components[name]; !ok {
			t.Errorf("clone placeholder %s not admitted", name)
		}
	}
}
