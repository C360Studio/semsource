package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
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

// TestReconcileDiscovery pins task 3.2: a discovered component that is missing
// is put; a stale one a working parent owns is deleted; a runtime-disabled one
// is neither re-enabled nor deleted (Q1); one a failed parent owns is kept; an
// operator or file-generated component is untouched; and the next-boot manifest
// is rewritten to match.
func TestReconcileDiscovery(t *testing.T) {
	newName, newEntry, newCC := docComponent(t, "new")
	staleName, staleEntry, staleCC := docComponent(t, "stale")
	offName, offEntry, offCC := docComponent(t, "off")
	keptName, _, keptCC := docComponent(t, "kept")
	fileName, _, fileCC := docComponent(t, "filegen")
	opName, _, opCC := docComponent(t, "op")
	disabled := offCC
	disabled.Enabled = false

	manifest, _ := json.Marshal(sourcemanifest.Config{Namespace: "acme", Sources: []sourcemanifest.ManifestSource{
		sourcemanifest.ManifestSourceFromEntry(staleEntry),
	}})
	store := &reconcileStore{components: semconfig.ComponentConfigs{
		"source-manifest": {Name: "source-manifest", Type: types.ComponentTypeProcessor, Enabled: true, Config: manifest},
		staleName:         staleCC, offName: disabled, keptName: keptCC, fileName: fileCC, opName: opCC,
	}}
	owned := map[string]bool{newName: true, staleName: true, offName: true, fileName: true}
	plan := &discoveryPlan{
		desired: semconfig.ComponentConfigs{newName: newCC, offName: offCC},
		entries: map[string]config.SourceEntry{newName: newEntry, offName: offEntry},
		owners: []discoveryOwner{
			{owns: func(n string) bool { return owned[n] }},
			{owns: func(n string) bool { return n == keptName }, failed: true},
		},
	}
	file := semconfig.ComponentConfigs{fileName: fileCC}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := reconcileDiscovery(context.Background(), store, plan, file, reconcileOpts, logger); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.puts, []string{newName, "source-manifest"}) {
		t.Errorf("puts = %v; want the new component, then the manifest", store.puts)
	}
	if !slices.Equal(store.dels, []string{staleName}) {
		t.Errorf("deletes = %v; want only the stale owned component", store.dels)
	}
	if store.components[offName].Enabled {
		t.Error("a runtime-disabled discovered component was re-enabled")
	}
	for _, name := range []string{keptName, fileName, opName} {
		if _, ok := store.components[name]; !ok {
			t.Errorf("%s was removed; want it untouched", name)
		}
	}
	m := manifestOf(t, store)
	var paths []string
	for _, src := range m.Sources {
		paths = append(paths, src.Paths...)
	}
	if !slices.Equal(paths, []string{"/r/new"}) {
		t.Errorf("manifest sources = %v; want only the new entry", paths)
	}
	// new, kept, filegen, op are the enabled source components.
	if m.ExpectedSourceCount != 4 {
		t.Errorf("expected_source_count = %d; want 4", m.ExpectedSourceCount)
	}

	store.puts, store.dels = nil, nil
	if err := reconcileDiscovery(context.Background(), store, plan, file, reconcileOpts, logger); err != nil {
		t.Fatal(err)
	}
	if len(store.puts)+len(store.dels) != 0 {
		t.Errorf("a second reconcile wrote puts=%v deletes=%v; want none", store.puts, store.dels)
	}
}

// TestBranchOwner pins task 3.1 for branches: a multi-branch source owns the
// components any of its branches would produce, including a branch that no
// longer exists (nothing on disk names it), and nothing else.
func TestBranchOwner(t *testing.T) {
	base := t.TempDir()
	for _, src := range []config.SourceEntry{
		{Type: "repo", Path: "/r/app", Branches: []string{"feature/*"}},
		{Type: "repo", URL: "https://example.com/acme/app.git", Project: "app", Version: "v1", Branches: []string{"feature/*"}},
	} {
		owns := branchOwner(config.BranchWatcherRef{WorktreeBase: base, Source: src}, reconcileOpts)
		for _, slug := range []string{"feature-a", "release-2026-11"} {
			entries := config.BranchEntries(src, workspace.BranchState{Branch: slug, Slug: slug, WorktreePath: filepath.Join(base, slug)})
			for name := range instanceNames(entries, reconcileOpts) {
				if !owns(name) {
					t.Errorf("%s: branch component %s is not owned", src.URL+src.Path, name)
				}
			}
		}
		for _, name := range []string{"doc-source-guide", "ast-source-app", "source-manifest", "doc-source-other-x"} {
			if owns(name) {
				t.Errorf("%s: %s is owned; want only branch components", src.URL+src.Path, name)
			}
		}
	}
}
