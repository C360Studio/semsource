// Package subwatch discovers the submodules of a repo source's checkout at
// runtime and expands each materialized one into scoped ast/docs/config
// component configs, following the branch-watcher precedent: discovery
// cannot happen at spawn time because a remote repo's checkout — and with it
// .gitmodules and the gitlink SHAs — does not exist until git-source clones
// it, and moving the clone into the spawn path would block boot on a
// monorepo-sized fetch (design D1).
//
// Identity is canonical, instances are parent-scoped (design D2/D3): the
// spawned entries carry project = URLToSlug(resolved submodule URL) and
// version = the 12-hex gitlink SHA prefix, so the same (URL, SHA) linked from
// any parent yields byte-identical entity IDs, while instance names carry a
// per-parent suffix so two parents' registrations never overwrite each other.
package subwatch

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/internal/sourcespawn"
	"github.com/c360studio/semsource/workspace"
)

// shaPrefixLen is the fixed truncation of the gitlink SHA used as the
// version qualifier (design D2: fixed, not git's variable-width
// abbreviation, so IDs are reproducible across repos and git versions).
const shaPrefixLen = 12

// ConfigStore is the minimal ConfigManager surface subwatch needs.
type ConfigStore interface {
	PutComponentToKV(ctx context.Context, name string, cfg types.ComponentConfig) error
	DeleteComponentFromKV(ctx context.Context, name string) error
}

// Config configures one Watcher (one per repo source).
type Config struct {
	// RepoPath is the parent repo's local checkout path. It may not exist
	// yet — git-source clones it asynchronously; ticks before that resolve
	// to an empty inventory and the watcher just keeps polling.
	RepoPath string

	// ParentSlug is the instance-name discriminator for this parent (its
	// project or path slug). Entity identity never includes it.
	ParentSlug string

	// Languages are the parent's declared languages, inherited by the
	// spawned ast entries.
	Languages []string

	// Watch propagates the parent's watch mode to spawned entries.
	Watch bool

	// Opts carries deployment-wide sourcespawn options (org, workspace dir,
	// token, media dir).
	Opts sourcespawn.Options

	// Store receives the spawned component configs.
	Store ConfigStore

	// Logger may be nil (slog.Default is used).
	Logger *slog.Logger
}

// spawned tracks the component instances one submodule pin produced.
type spawned struct {
	sha   string
	names []string

	// confirmed marks that the configs were re-put once on a tick AFTER the
	// spawning one. The framework's reactive config watcher has been observed
	// to drop one event out of a same-instant write burst (semstreams ask
	// 2026-08-17: 6 puts in ~20ms deployed 5 components; an identical re-put
	// at a later revision deployed the sixth immediately). A single quiet
	// re-put per pin is cheap insurance until the watcher is fixed.
	confirmed bool
}

// Watcher polls one parent repo's checkout and reconciles spawned
// per-submodule components against the current gitlink pins.
type Watcher struct {
	cfg    Config
	logger *slog.Logger

	// tracked maps submodule root-relative path → its spawned components.
	tracked map[string]*spawned
}

// New creates a Watcher.
func New(cfg Config) *Watcher {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Watcher{cfg: cfg, logger: logger, tracked: map[string]*spawned{}}
}

// Run polls until ctx is done. interval must be positive.
func (w *Watcher) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Immediate first tick so a local (pre-existing) checkout expands
	// without waiting a full interval.
	w.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.Tick(ctx)
		}
	}
}

// Tick runs one inventory-and-reconcile pass. Exported for tests and for
// callers that want an on-demand pass.
func (w *Watcher) Tick(ctx context.Context) {
	inv, err := workspace.ListSubmodules(ctx, w.cfg.RepoPath)
	if err != nil {
		// Expected before git-source's clone materializes the checkout;
		// debug, not warn — the loudness surface for missing trees is the
		// git-source status report, not this loop.
		w.logger.Debug("submodule inventory unavailable",
			"repo", w.cfg.RepoPath, "error", err)
		return
	}

	desired := map[string]workspace.SubmoduleInfo{}
	for _, s := range inv.Submodules {
		// Only materialized pins with a real gitlink are ingestable;
		// unmaterialized and stale declarations stay loud on status.
		if s.Materialized && len(s.SHA) >= shaPrefixLen {
			desired[s.Path] = s
		}
	}

	// Remove: tracked pins that vanished or moved (a moved gitlink is a
	// version transition — spawn-new-then-remove-old, design D8).
	for path, tr := range w.tracked {
		cur, ok := desired[path]
		if ok && cur.SHA == tr.sha {
			continue
		}
		if ok {
			// Pin moved: spawn the new version first so the graph never
			// has a window with neither version registered.
			if newTr, err := w.spawn(ctx, cur); err == nil {
				w.removeInstances(ctx, path, tr)
				w.tracked[path] = newTr
			} else {
				w.logger.Warn("submodule respawn failed; keeping old version",
					"path", path, "error", err)
			}
			continue
		}
		w.removeInstances(ctx, path, tr)
		delete(w.tracked, path)
	}

	// Confirm: one idempotent re-put for pins spawned on an earlier tick
	// (see spawned.confirmed).
	for path, tr := range w.tracked {
		if tr.confirmed {
			continue
		}
		if cur, ok := desired[path]; ok && cur.SHA == tr.sha {
			if _, err := w.spawn(ctx, cur); err != nil {
				w.logger.Warn("submodule confirm re-put failed",
					"path", path, "error", err)
				continue
			}
			tr.confirmed = true
		}
	}

	// Add: new pins.
	for path, cur := range desired {
		if _, ok := w.tracked[path]; ok {
			continue
		}
		tr, err := w.spawn(ctx, cur)
		if err != nil {
			w.logger.Warn("submodule expansion failed",
				"path", path, "error", err)
			continue
		}
		w.logger.Info("submodule expanded",
			"path", path, "project", workspace.URLToSlug(cur.ResolvedURL),
			"version", cur.SHA[:shaPrefixLen], "components", len(tr.names))
		w.tracked[path] = tr
	}
}

// spawn builds and publishes the ast/docs/config component configs for one
// materialized submodule pin.
func (w *Watcher) spawn(ctx context.Context, s workspace.SubmoduleInfo) (*spawned, error) {
	project := workspace.URLToSlug(s.ResolvedURL)
	sha12 := s.SHA[:shaPrefixLen]
	abs := filepath.Join(w.cfg.RepoPath, filepath.FromSlash(s.Path))
	// The suffix is parent+link scoped, not merely parent scoped: one parent
	// can link the same submodule at the same SHA under two paths, and a
	// name shared between those links would make removing one link tear
	// down the other's instance. Entity identity is untouched either way.
	suffix := "via-" + w.cfg.ParentSlug + "-" + strings.ReplaceAll(s.Path, "/", "-")

	entries := []config.SourceEntry{
		{
			Type:           "ast",
			Path:           abs,
			Project:        project,
			Version:        sha12,
			Languages:      w.cfg.Languages,
			Watch:          w.cfg.Watch,
			InstanceSuffix: suffix,
		},
		// docs/config have no separate version surface; the pin rides the
		// project slug so two SHAs of one submodule keep distinct doc and
		// config identity (same reason ast version-scopes its system
		// segment), while identical (URL, SHA) still dedups across parents.
		{
			Type:           "docs",
			Paths:          []string{abs},
			Project:        project + "-" + sha12,
			Watch:          w.cfg.Watch,
			InstanceSuffix: suffix,
		},
		{
			Type:           "config",
			Paths:          []string{abs},
			Project:        project + "-" + sha12,
			Watch:          w.cfg.Watch,
			InstanceSuffix: suffix,
		},
	}

	tr := &spawned{sha: s.SHA}
	for _, entry := range entries {
		built, err := sourcespawn.Build(entry, w.cfg.Opts)
		if err != nil {
			return nil, err
		}
		for name, compCfg := range built {
			if err := w.cfg.Store.PutComponentToKV(ctx, name, compCfg); err != nil {
				return nil, err
			}
			tr.names = append(tr.names, name)
		}
	}
	return tr, nil
}

// removeInstances deletes one pin's spawned component configs.
func (w *Watcher) removeInstances(ctx context.Context, path string, tr *spawned) {
	for _, name := range tr.names {
		if err := w.cfg.Store.DeleteComponentFromKV(ctx, name); err != nil {
			w.logger.Warn("failed to remove submodule component",
				"path", path, "component", name, "error", err)
		}
	}
}
