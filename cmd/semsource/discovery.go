package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

// discoveryOwner is one discovering parent source and the component names
// its discovery admits this boot. When its discovery failed (a listing error
// or a missing checkout), the names recorded for it at the previous boot are
// kept rather than deleted (source-discovery spec).
type discoveryOwner struct {
	parent, kind string
	failed       bool
	names        map[string]bool
}

// discoveryPlan is what boot-time discovery found: the components it admits
// this boot, the entry each was built from, every discovering parent, and the
// per-parent states.
type discoveryPlan struct {
	desired semconfig.ComponentConfigs
	entries map[string]config.SourceEntry
	owners  []*discoveryOwner
	states  []config.DiscoveryState
	// watches is what the runtime check compares, one per state.
	watches []sourcemanifest.DiscoveryWatch
}

func (p *discoveryPlan) owner(parent, kind string, failed bool) *discoveryOwner {
	o := &discoveryOwner{parent: parent, kind: kind, failed: failed, names: map[string]bool{}}
	p.owners = append(p.owners, o)
	return o
}

func (p *discoveryPlan) add(o *discoveryOwner, entry config.SourceEntry, opts sourcespawn.Options) error {
	built, err := sourcespawn.Build(entry, opts)
	if err != nil {
		return err
	}
	for name, cc := range built {
		p.desired[name] = cc
		p.entries[name] = entry
		o.names[name] = true
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

// planDiscovery collects the branch components expansion found, the clone
// component of every remote multi-branch source, and the materialized
// submodules of every single-branch repo or git source. It fails only when a
// found source cannot be built; a discovery failure is a state, never an
// error.
func planDiscovery(ctx context.Context, cfg *config.Config, expand *config.ExpandResult, opts sourcespawn.Options) (*discoveryPlan, error) {
	plan := &discoveryPlan{desired: semconfig.ComponentConfigs{}, entries: map[string]config.SourceEntry{}, states: expand.Discovery}
	for i, ref := range expand.Watchers {
		if i >= len(expand.Discovery) {
			break
		}
		state := expand.Discovery[i]
		failed := state.State == config.DiscoveryListingFailed || state.State == config.DiscoveryAwaitingCheckout
		o := plan.owner(ref.Source.URL+ref.Source.Path, "branch", failed)
		entries := ref.Entries
		if ref.Source.URL != "" {
			// The clone keeps the checkout current and ingests the default
			// branch; a shallow clone lists no other branch (owner ruling
			// 2026-11-18, remote branch listing is a follow-up).
			entries = append([]config.SourceEntry{config.ParentCloneEntry(ref.Source)}, entries...)
		}
		for _, entry := range entries {
			if err := plan.add(o, entry, opts); err != nil {
				return nil, fmt.Errorf("discovered branch source %s: %w", entry.URL+entry.Path, err)
			}
		}
		plan.watches = append(plan.watches, sourcemanifest.DiscoveryWatch{
			DiscoveryState: state, RepoPath: ref.RepoPath, Patterns: ref.Patterns, MaxBranches: ref.MaxBranches,
		})
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
		var entries []config.SourceEntry
		if _, err := os.Stat(repoPath); errors.Is(err, fs.ErrNotExist) {
			state.State = config.DiscoveryAwaitingCheckout
		} else if expanded, pins, err := subwatch.Expand(ctx, subwatch.Config{
			RepoPath: repoPath, ParentSlug: parentSlug, Languages: src.EffectiveLanguages(), Watch: src.Watch,
		}); err != nil {
			state.State, state.Error = config.DiscoveryListingFailed, err.Error()
		} else {
			state.Admitted, entries = pins, expanded
		}
		o := plan.owner(state.Source, "submodule", state.State != config.DiscoveryOK)
		for _, entry := range entries {
			if err := plan.add(o, entry, opts); err != nil {
				return nil, fmt.Errorf("submodule source %s: %w", entry.Path, err)
			}
		}
		plan.states = append(plan.states, state)
		plan.watches = append(plan.watches, sourcemanifest.DiscoveryWatch{DiscoveryState: state, RepoPath: repoPath})
	}
	return plan, nil
}

// record is the ownership record this boot writes, and the names discovery
// owns and must keep, given the previous boot's record: every recorded name
// is owned; a failed parent keeps its recorded names and records them again.
func (p *discoveryPlan) record(prev []sourcemanifest.DiscoveredRecord) (next []sourcemanifest.DiscoveredRecord, owned, protected map[string]bool) {
	owned, protected = map[string]bool{}, map[string]bool{}
	prevBy := map[string][]string{}
	for _, r := range prev {
		prevBy[r.Kind+" "+r.Parent] = append(prevBy[r.Kind+" "+r.Parent], r.Components...)
		for _, name := range r.Components {
			owned[name] = true
		}
	}
	for _, o := range p.owners {
		names := maps.Clone(o.names)
		if o.failed {
			for _, name := range prevBy[o.kind+" "+o.parent] {
				names[name], protected[name] = true, true
			}
		}
		if len(names) > 0 {
			next = append(next, sourcemanifest.DiscoveredRecord{Parent: o.parent, Kind: o.kind, Components: slices.Sorted(maps.Keys(names))})
		}
	}
	slices.SortFunc(next, func(a, b sourcemanifest.DiscoveredRecord) int {
		return strings.Compare(a.Kind+" "+a.Parent, b.Kind+" "+b.Parent)
	})
	return next, owned, protected
}

// previousRecord reads the ownership record the previous boot wrote into the
// next-boot manifest; none before the first discovering boot.
func previousRecord(effective semconfig.ComponentConfigs) ([]sourcemanifest.DiscoveredRecord, error) {
	envelope, ok := effective["source-manifest"]
	if !ok {
		return nil, nil
	}
	var m sourcemanifest.Config
	if err := json.Unmarshal(envelope.Config, &m); err != nil {
		return nil, fmt.Errorf("decode next-boot source-manifest: %w", err)
	}
	return m.Discovered, nil
}

// reconcileDiscovery makes the configuration in effect hold exactly the
// discovered components, before the service manager snapshots it. It puts a
// discovered component that is missing or differs, unless a runtime removal
// disabled it (owner ruling Q1). It deletes an enabled component the previous
// boot's record names that nothing discovered or the file generates, unless
// its parent's discovery failed. It then rewrites the next-boot manifest so
// its sources, expected count and record match. It touches no other name.
func reconcileDiscovery(ctx context.Context, store sourcespawn.ConfigStore, plan *discoveryPlan, file semconfig.ComponentConfigs, opts sourcespawn.Options, logger *slog.Logger) error {
	effective := store.GetConfig().Get().Components
	prev, err := previousRecord(effective)
	if err != nil {
		return err
	}
	next, owned, protected := plan.record(prev)
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
		if !have.Enabled || !owned[name] || protected[name] {
			continue
		}
		if _, ok := plan.desired[name]; ok {
			continue
		}
		if _, ok := file[name]; ok {
			continue
		}
		if err := store.DeleteComponentFromKV(ctx, name); err != nil {
			return fmt.Errorf("retire discovered component %s: %w", name, err)
		}
		removed = append(removed, name)
	}
	if len(added) == 0 && len(removed) == 0 && reflect.DeepEqual(prev, next) {
		return nil
	}
	logger.Info("discovery reconciled", "admitted", len(added), "retired", removed)
	return sourcemanifest.RewriteDesiredManifest(ctx, store, opts, added, removed, &next)
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
