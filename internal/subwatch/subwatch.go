// Package subwatch expands the materialized submodules of a repo source's
// checkout into scoped ast/docs/config source entries at boot, and lists the
// pins the read-only runtime discovery check compares against (#216: on the
// SemEngine pin composition seals at boot, so nothing is admitted at run
// time).
//
// Identity is canonical, instances are parent-scoped (design D2/D3): the
// entries carry project = URLToSlug(resolved submodule URL) and version = the
// 12-hex gitlink SHA prefix, so the same (URL, SHA) linked from any parent
// yields byte-identical entity IDs, while instance names carry a per-parent
// suffix so two parents' registrations never overwrite each other.
package subwatch

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/workspace"
)

// shaPrefixLen is the fixed truncation of the gitlink SHA used as the
// version qualifier (design D2: fixed, not git's variable-width
// abbreviation, so IDs are reproducible across repos and git versions).
const shaPrefixLen = 12

// Config describes one parent repo source whose submodules expand.
type Config struct {
	// RepoPath is the parent repo's local checkout path.
	RepoPath string

	// ParentSlug is the instance-name discriminator for this parent (its
	// project or path slug). Entity identity never includes it.
	ParentSlug string

	// Languages are the parent's declared languages, inherited by the
	// expanded ast entries.
	Languages []string

	// Watch propagates the parent's watch mode to expanded entries.
	Watch bool
}

// Expand returns the source entries of every materialized submodule of the
// parent checkout at cfg.RepoPath, for boot-time discovery, and the pins they
// were built from (path to gitlink SHA). A checkout that cannot be listed
// returns the inventory error.
func Expand(ctx context.Context, cfg Config) ([]config.SourceEntry, map[string]string, error) {
	inv, err := workspace.ListSubmodules(ctx, cfg.RepoPath)
	if err != nil {
		return nil, nil, err
	}
	var out []config.SourceEntry
	for _, s := range materialized(inv) {
		out = append(out, pinEntries(cfg, s)...)
	}
	return out, pinSHAs(inv), nil
}

// Pins lists the materialized submodule pins of the checkout at repoPath
// (path to pin fingerprint) without building or writing anything: the
// read-only runtime discovery check compares it with what boot admitted.
func Pins(ctx context.Context, repoPath string) (map[string]string, error) {
	inv, err := workspace.ListSubmodules(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	return pinSHAs(inv), nil
}

// materialized is the ingestable pins: a populated tree with a real gitlink.
func materialized(inv *workspace.SubmoduleInventory) []workspace.SubmoduleInfo {
	var out []workspace.SubmoduleInfo
	for _, s := range inv.Submodules {
		if s.Materialized && len(s.SHA) >= shaPrefixLen {
			out = append(out, s)
		}
	}
	return out
}

// pinSHAs maps each materialized pin's path to its fingerprint: everything
// its components' identity derives from, the gitlink SHA and the project the
// resolved URL names, so a URL-only move (a fork at the same commit) is a
// change too.
func pinSHAs(inv *workspace.SubmoduleInventory) map[string]string {
	out := map[string]string{}
	for _, s := range materialized(inv) {
		out[s.Path] = s.SHA + " " + workspace.URLToSlug(s.ResolvedURL)
	}
	return out
}

// pinEntries are the ast/docs/config source entries for one materialized
// submodule pin.
func pinEntries(cfg Config, s workspace.SubmoduleInfo) []config.SourceEntry {
	project := workspace.URLToSlug(s.ResolvedURL)
	sha12 := s.SHA[:shaPrefixLen]
	abs := filepath.Join(cfg.RepoPath, filepath.FromSlash(s.Path))
	// The suffix is parent+link scoped, not merely parent scoped: one parent
	// can link the same submodule at the same SHA under two paths, and a
	// name shared between those links would make removing one link tear
	// down the other's instance. Entity identity is untouched either way.
	suffix := "via-" + cfg.ParentSlug + "-" + strings.ReplaceAll(s.Path, "/", "-")

	return []config.SourceEntry{
		{
			Type:           "ast",
			Path:           abs,
			Project:        project,
			Version:        sha12,
			Languages:      cfg.Languages,
			Watch:          cfg.Watch,
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
			Watch:          cfg.Watch,
			InstanceSuffix: suffix,
		},
		{
			Type:           "config",
			Paths:          []string{abs},
			Project:        project + "-" + sha12,
			Watch:          cfg.Watch,
			InstanceSuffix: suffix,
		},
	}
}
