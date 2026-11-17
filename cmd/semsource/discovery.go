package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	semconfig "github.com/c360studio/semengine/config"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/internal/sourcespawn"
	"github.com/c360studio/semsource/internal/subwatch"
	sourcemanifest "github.com/c360studio/semsource/processor/source-manifest"
	"github.com/c360studio/semsource/workspace"
)

// discoveryOwner is one discovering parent source: which component names its
// discovery derives, and whether its discovery failed this boot, in which case
// the names it owns are kept rather than deleted (source-discovery spec).
type discoveryOwner struct {
	owns   func(name string) bool
	failed bool
}

// discoveryPlan is what boot-time discovery found: the components it admits
// this boot, the entry each was built from, every discovering parent, and the
// per-parent states.
type discoveryPlan struct {
	desired semconfig.ComponentConfigs
	entries map[string]config.SourceEntry
	owners  []discoveryOwner
	states  []config.DiscoveryState
	// watches is what the runtime check compares, one per state.
	watches []sourcemanifest.DiscoveryWatch
}

func (p *discoveryPlan) add(entry config.SourceEntry, opts sourcespawn.Options) error {
	built, err := sourcespawn.Build(entry, opts)
	if err != nil {
		return err
	}
	for name, cc := range built {
		p.desired[name] = cc
		p.entries[name] = entry
	}
	return nil
}

// admitDiscovered plans boot-time discovery, reports each discovering source's
// state, and reconciles the discovered components into the configuration in
// effect. It returns what the runtime check compares against this boot.
func admitDiscovered(ctx context.Context, cfg *config.Config, expand *config.ExpandResult, file semconfig.ComponentConfigs, store sourcespawn.ConfigStore, logger *slog.Logger) ([]sourcemanifest.DiscoveryWatch, error) {
	opts := sourcespawn.Options{
		Org:           cfg.Namespace,
		WorkspaceDir:  cfg.WorkspaceDir,
		GitToken:      cfg.GitToken,
		MediaStoreDir: cfg.MediaStoreDir,
	}
	plan, err := planDiscovery(ctx, cfg, expand, opts)
	if err != nil {
		return nil, err
	}
	for _, st := range plan.states {
		logger.Info("source discovery", "source", st.Source, "kind", st.Kind, "state", st.State,
			"error", st.Error, "not_admitted", st.NotAdmitted)
	}
	return plan.watches, reconcileDiscovery(ctx, store, plan, file, opts, logger)
}

// owned reports whether a working parent's discovery derives name; protected
// whether a failed parent's does.
func (p *discoveryPlan) owned(name string) (owned, protected bool) {
	for _, o := range p.owners {
		if o.owns(name) {
			if o.failed {
				return true, true
			}
			owned = true
		}
	}
	return owned, false
}

// planDiscovery collects the branch components expansion already found and
// expands the materialized submodules of every single-branch repo or git
// source. It fails only when a found source cannot be built; a discovery
// failure is a state, never an error.
func planDiscovery(ctx context.Context, cfg *config.Config, expand *config.ExpandResult, opts sourcespawn.Options) (*discoveryPlan, error) {
	plan := &discoveryPlan{desired: semconfig.ComponentConfigs{}, entries: map[string]config.SourceEntry{}, states: expand.Discovery}
	for _, entry := range expand.Discovered {
		if err := plan.add(entry, opts); err != nil {
			return nil, fmt.Errorf("discovered branch source %s: %w", entry.Path, err)
		}
	}
	for i, ref := range expand.Watchers {
		var state string
		if i < len(expand.Discovery) {
			state = expand.Discovery[i].State
		}
		// A missing checkout lists nothing, which says nothing about the
		// branches: keep the names admitted before, and admit the clone
		// placeholder whatever arbitration chose so the checkout returns.
		failed := state == config.DiscoveryListingFailed || state == config.DiscoveryAwaitingCheckout
		if state == config.DiscoveryAwaitingCheckout {
			if err := plan.add(config.ParentCloneEntry(ref.Source), opts); err != nil {
				return nil, fmt.Errorf("clone placeholder for %s: %w", ref.Source.URL, err)
			}
		}
		plan.owners = append(plan.owners, discoveryOwner{owns: branchOwner(ref, opts), failed: failed})
		if i < len(expand.Discovery) {
			plan.watches = append(plan.watches, sourcemanifest.DiscoveryWatch{
				DiscoveryState: expand.Discovery[i], RepoPath: ref.RepoPath, Patterns: ref.Patterns, MaxBranches: ref.MaxBranches,
			})
		}
	}

	for _, src := range cfg.Sources {
		if (src.Type != "repo" && src.Type != "git") || src.BranchSlug != "" {
			continue // per-branch submodule expansion is a non-goal
		}
		if src.Submodules != nil && !*src.Submodules {
			continue
		}
		repoPath := src.Path
		if repoPath == "" && src.URL != "" {
			repoPath = filepath.Join(cfg.WorkspaceDir, workspace.URLToSlug(src.URL))
		}
		if repoPath == "" {
			continue
		}
		parentSlug := entityid.SystemSlug(repoPath)
		if src.Project != "" {
			parentSlug = entityid.SystemSlug(src.Project)
		}
		state := config.DiscoveryState{Source: src.URL + src.Path, Kind: "submodule", State: config.DiscoveryOK}
		owner := discoveryOwner{owns: func(name string) bool { return subwatch.OwnedBy(name, parentSlug) }}
		if _, err := os.Stat(repoPath); errors.Is(err, fs.ErrNotExist) {
			state.State = config.DiscoveryAwaitingCheckout
			owner.failed = true // nothing listed; keep the children admitted before
		} else if entries, pins, err := subwatch.Expand(ctx, subwatch.Config{
			RepoPath: repoPath, ParentSlug: parentSlug, Languages: src.EffectiveLanguages(), Watch: src.Watch,
		}); err != nil {
			state.State, state.Error = config.DiscoveryListingFailed, err.Error()
			owner.failed = true
		} else {
			state.Admitted = pins
			for _, entry := range entries {
				if err := plan.add(entry, opts); err != nil {
					return nil, fmt.Errorf("submodule source %s: %w", entry.Path, err)
				}
			}
		}
		plan.owners = append(plan.owners, owner)
		plan.states = append(plan.states, state)
		plan.watches = append(plan.watches, sourcemanifest.DiscoveryWatch{DiscoveryState: state, RepoPath: repoPath})
	}
	return plan, nil
}

// branchOwner derives the component names a multi-branch source's discovery
// owns: those any branch of the source would produce, whether or not the
// branch still exists, plus the clone placeholder admitted while the checkout
// did not. A deleted branch leaves no worktree or ref to enumerate, so the
// candidate branch slug is read back from the name itself: every run of its
// dash-separated words is tried, and the name is owned only when rebuilding
// that branch's entries yields exactly this name.
func branchOwner(ref config.BranchWatcherRef, opts sourcespawn.Options) func(string) bool {
	placeholder := map[string]bool{}
	if ref.Source.URL != "" {
		for name := range instanceNames([]config.SourceEntry{config.ParentCloneEntry(ref.Source)}, opts) {
			placeholder[name] = true
		}
	}
	return func(name string) bool {
		if placeholder[name] {
			return true
		}
		for _, prefix := range []string{"git-source-", "ast-source-", "doc-source-", "cfgfile-source-"} {
			tail, ok := strings.CutPrefix(name, prefix)
			if !ok {
				continue
			}
			words := strings.Split(tail, "-")
			for i := range words {
				for j := i + 1; j <= len(words); j++ {
					slug := strings.Join(words[i:j], "-")
					entries := config.BranchEntries(ref.Source, workspace.BranchState{
						Branch: slug, Slug: slug, WorktreePath: filepath.Join(ref.WorktreeBase, slug),
					})
					if _, ok := instanceNames(entries, opts)[name]; ok {
						return true
					}
				}
			}
		}
		return false
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

// reconcileDiscovery makes the configuration in effect hold exactly the
// discovered components, before the service manager snapshots it. It puts a
// discovered component that is missing or differs, unless a runtime removal
// disabled it (owner ruling Q1). It deletes an enabled component a working
// parent's discovery derives that nothing discovered or the file generates,
// unless a failed parent also derives it. It then rewrites the next-boot
// manifest so its sources and expected count match. It touches no other name.
func reconcileDiscovery(ctx context.Context, store sourcespawn.ConfigStore, plan *discoveryPlan, file semconfig.ComponentConfigs, opts sourcespawn.Options, logger *slog.Logger) error {
	effective := store.GetConfig().Get().Components
	var added []config.SourceEntry
	addedEntry := map[string]bool{}
	for name, want := range plan.desired {
		have, exists := effective[name]
		if exists && (!have.Enabled || reflect.DeepEqual(have, want)) {
			continue
		}
		if err := store.PutComponentToKV(ctx, name, want); err != nil {
			return fmt.Errorf("admit discovered component %s: %w", name, err)
		}
		entry := plan.entries[name]
		if key := fmt.Sprintf("%+v", entry); !addedEntry[key] {
			addedEntry[key] = true
			added = append(added, entry)
		}
	}
	var removed []string
	for name, have := range effective {
		if !have.Enabled {
			continue
		}
		if _, ok := plan.desired[name]; ok {
			continue
		}
		if _, ok := file[name]; ok {
			continue
		}
		if owned, protected := plan.owned(name); !owned || protected {
			continue
		}
		if err := store.DeleteComponentFromKV(ctx, name); err != nil {
			return fmt.Errorf("retire discovered component %s: %w", name, err)
		}
		removed = append(removed, name)
	}
	if len(added) == 0 && len(removed) == 0 {
		return nil
	}
	logger.Info("discovery reconciled", "admitted", len(added), "retired", removed)
	return sourcemanifest.RewriteDesiredManifest(ctx, store, opts, added, removed)
}

// discoveryInterval is how often the runtime check runs: the shortest
// branch_poll_interval a multi-branch source sets, else the default.
func discoveryInterval(cfg *config.Config) time.Duration {
	var interval time.Duration
	for _, src := range cfg.Sources {
		if len(src.Branches) == 0 {
			continue
		}
		if d, err := time.ParseDuration(src.BranchPollInterval); err == nil && d > 0 && (interval == 0 || d < interval) {
			interval = d
		}
	}
	if interval == 0 {
		return sourcemanifest.DefaultDiscoveryInterval
	}
	return interval
}
