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
// its discovery admits this boot. When its discovery failed or was partial (a
// listing error, a missing checkout, or a branch whose worktree could not be
// created), every name recorded for it at the previous boot is kept rather
// than deleted (source-discovery spec): a recorded name says nothing about
// which branch or worktree path produced it. keptPrior reports that such
// names were kept, so this boot's admitted set is not the parent's full one.
type discoveryOwner struct {
	parent, kind string
	failed       bool
	names        map[string]bool
	keptPrior    bool
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
	if err := reconcileDiscovery(ctx, store, plan, file, opts, logger); err != nil {
		return nil, err
	}
	// Owners and watches are appended together, one each per parent.
	for i, o := range plan.owners {
		plan.watches[i].Partial = o.keptPrior
	}
	return plan.watches, nil
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
		failed := state.State == config.DiscoveryListingFailed || state.State == config.DiscoveryAwaitingCheckout ||
			len(state.Skipped) > 0
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
		interval, _ := time.ParseDuration(ref.BranchPollInterval) // invalid or unset: the default
		plan.watches = append(plan.watches, sourcemanifest.DiscoveryWatch{
			DiscoveryState: state, RepoPath: ref.RepoPath, Patterns: ref.Patterns, MaxBranches: ref.MaxBranches,
			Interval: interval,
		})
	}

	// The declared sources, not the expanded ones: expansion drops a repo's
	// languages and project from its git child.
	for _, src := range expand.Declared {
		if (src.Type != "repo" && src.Type != "git") || src.BranchSlug != "" || len(src.Branches) > 0 {
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
				// A recorded name this boot admits anyway (the clone) is
				// known membership; only the others make the set partial.
				if !o.names[name] {
					o.keptPrior = true
				}
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

// pendingRecord is prev with next's names added: the record to hold while
// this boot's puts and deletes are in flight.
func pendingRecord(prev, next []sourcemanifest.DiscoveredRecord) []sourcemanifest.DiscoveredRecord {
	by := map[string]map[string]bool{}
	keyed := map[string]sourcemanifest.DiscoveredRecord{}
	for _, r := range append(slices.Clone(prev), next...) {
		k := r.Kind + " " + r.Parent
		if by[k] == nil {
			by[k], keyed[k] = map[string]bool{}, r
		}
		for _, name := range r.Components {
			by[k][name] = true
		}
	}
	var out []sourcemanifest.DiscoveredRecord
	for _, k := range slices.Sorted(maps.Keys(by)) {
		r := keyed[k]
		out = append(out, sourcemanifest.DiscoveredRecord{Parent: r.Parent, Kind: r.Kind, Components: slices.Sorted(maps.Keys(by[k]))})
	}
	return out
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
	// Record what this boot is about to put before putting it, so a boot that
	// fails partway leaves no component that no record names.
	if pending := pendingRecord(prev, next); !reflect.DeepEqual(pending, prev) {
		if err := sourcemanifest.RewriteDesiredManifest(ctx, store, opts, nil, nil, &pending); err != nil {
			return err
		}
	}
	admitted := 0
	for name, want := range plan.desired {
		have, exists := effective[name]
		if exists && (!have.Enabled || reflect.DeepEqual(have, want)) {
			continue
		}
		if err := store.PutComponentToKV(ctx, name, want); err != nil {
			return fmt.Errorf("admit discovered component %s: %w", name, err)
		}
		admitted++
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
	if admitted > 0 || len(removed) > 0 {
		logger.Info("discovery reconciled", "admitted", admitted, "retired", removed)
	}
	sources, gone := manifestDelta(store.GetConfig().Get().Components, plan, owned)
	return sourcemanifest.RewriteDesiredManifest(ctx, store, opts, sources, gone, &next)
}

// manifestDelta is what the next-boot manifest must hold after reconcile,
// read from the configuration now in effect rather than from this attempt's
// writes: an earlier boot may have committed its puts or deletes and failed
// its manifest write. sources are the entries of the enabled discovered
// components, in a stable order so an unchanged manifest stays byte-equal;
// gone are the recorded names that nothing discovers and the configuration
// no longer holds.
func manifestDelta(now semconfig.ComponentConfigs, plan *discoveryPlan, owned map[string]bool) (sources []config.SourceEntry, gone []string) {
	seen := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(plan.desired)) {
		if !now[name].Enabled {
			continue
		}
		entry := plan.entries[name]
		if key := fmt.Sprintf("%+v", entry); !seen[key] {
			seen[key] = true
			sources = append(sources, entry)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(owned)) {
		_, desired := plan.desired[name]
		_, present := now[name]
		if !desired && !present {
			gone = append(gone, name)
		}
	}
	return sources, gone
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
