// Package sourcespawn translates config.SourceEntry values into the
// component configs the semstreams ServiceManager spawns reactively from
// the shared config KV bucket.
//
// The package is the single source of truth for "what does a source look
// like as a runtime component" — used by the startup loader, the
// branch-watcher, and the programmatic source-add API on source-manifest.
package sourcespawn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

	"github.com/c360studio/semsource/config"
	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/workspace"
	"github.com/c360studio/semstreams/component"
)

// contentHashSlug produces a deterministic 6-hex-character slug from the
// identifying fields of src. Used as a fallback when entityid.SystemSlug
// returns empty for the type's natural identifier (e.g., a path of "." or
// "/" yields no useful slug).
//
// The hash covers all identifying fields, so two SourceEntries that produce
// the same hash describe the same logical source — that's what makes Add
// idempotent (and why Build no longer needs an index parameter to break
// fallback ties; collisions now imply equivalence by construction).
func contentHashSlug(src config.SourceEntry) string {
	var b strings.Builder
	b.WriteString(src.Type)
	b.WriteByte('|')
	b.WriteString(src.URL)
	b.WriteByte('|')
	b.WriteString(src.Path)
	b.WriteByte('|')
	b.WriteString(strings.Join(src.Paths, ","))
	b.WriteByte('|')
	b.WriteString(strings.Join(src.URLs, ","))
	b.WriteByte('|')
	b.WriteString(src.Branch)
	b.WriteByte('|')
	b.WriteString(src.BranchSlug)
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:3]) // 6 hex chars; collision-safe for typical configs
}

// outputPorts returns the standard output port config shared by all source
// components. The flow engine reads ports from the config JSON to discover
// connections — without explicit ports in the config, the flow graph shows
// 0 connections even though components use DefaultConfig at runtime.
func outputPorts() map[string]any {
	return map[string]any{
		"outputs": []component.PortDefinition{
			{
				Name:        "graph.ingest",
				Required:    true,
				Description: "Entity state updates for graph ingestion",
				Config:      component.JetStreamPort{StreamName: "GRAPH", Subjects: []string{"graph.ingest.entity"}},
			},
		},
	}
}

// astComponentConfig builds a component instance name and config map for an
// AST source entry. The instance name is derived from the path so multiple
// AST sources produce distinct component instances.
func astComponentConfig(src config.SourceEntry, org string) (string, map[string]any, error) {
	path := src.Path
	if path == "" {
		path = "."
	}

	languages := src.EffectiveLanguages()
	if len(languages) == 0 {
		languages = []string{"go"}
	}

	slug := entityid.SystemSlug(path)
	// An explicit project overrides the path-derived slug: supersession
	// corresponds entities by project, and version directories slug
	// differently per version (D1 amendment). Slugified for ID-safety.
	if src.Project != "" {
		slug = entityid.SystemSlug(src.Project)
	}
	if slug == "" {
		slug = "src-" + contentHashSlug(src)
	}
	project := entityid.BranchScopedSlug(slug, src.BranchSlug)
	instanceName := fmt.Sprintf("ast-source-%s", project)
	// Two versioned registrations of one project need distinct component
	// instances; the version suffix keeps them unique. Version-less names
	// stay byte-identical to today.
	if src.Version != "" {
		if vs := entityid.SystemSlug(src.Version); vs != "" {
			instanceName = fmt.Sprintf("ast-source-%s-%s", project, vs)
		}
	}
	instanceName = applyInstanceSuffix(instanceName, src.InstanceSuffix)

	// True-freeze snapshots (entity-staleness spec D5): watch:false with no
	// explicit index_interval is a real one-shot snapshot — no watcher, no
	// periodic reindex either. Previously this always defaulted to "60s"
	// regardless of Watch, contradicting the documented "one-shot snapshot"
	// semantics (add_source's tool description) and poisoning the sidecar
	// use case, where a pinned ephemeral worktree is EXPECTED to vanish out
	// from under a frozen source. An explicit IndexInterval is always
	// honored — that is the opt-back-in to periodic tracking even with
	// watch:false.
	indexInterval := src.IndexInterval
	if indexInterval == "" && src.Watch {
		indexInterval = "60s"
	}

	watchPath := map[string]any{
		"path":      path,
		"org":       org,
		"project":   project,
		"languages": languages,
	}
	// An explicit version flows to the component's version-scoped entity IDs
	// and code.artifact.version triples — the supersession/code_changes input
	// (version-registration-surface D1). Absent stays byte-identical.
	if src.Version != "" {
		watchPath["version"] = src.Version
	}
	compCfg := map[string]any{
		"ports":          outputPorts(),
		"watch_paths":    []map[string]any{watchPath},
		"watch_enabled":  src.Watch,
		"index_interval": indexInterval,
		"instance_name":  instanceName,
	}

	return instanceName, compCfg, nil
}

// gitComponentConfig builds a component instance name and config map for a
// git source entry. ctx flows in for the `git ls-remote --symref` call
// that resolves the remote's default branch when src.Branch is empty.
func gitComponentConfig(ctx context.Context, src config.SourceEntry, org string, opts Options) (string, map[string]any, error) {
	identifier := src.URL
	if identifier == "" {
		identifier = src.Path
	}
	slug := entityid.SystemSlug(identifier)
	if slug == "" {
		slug = "git-" + contentHashSlug(src)
	}
	scopedSlug := entityid.BranchScopedSlug(slug, src.BranchSlug)
	instanceName := fmt.Sprintf("git-source-%s", scopedSlug)

	// Resolve the remote's default branch via `git ls-remote --symref`
	// when the caller didn't specify one. This is the load-bearing fix
	// for the curator workflow (ADR-040 add_source_repo) — without it,
	// pre-rename repos like osh-core (master), Apache-era projects
	// (master/trunk), and custom defaults (develop) silently broke.
	// Resolution runs once here per spawn, covering every code path that
	// reaches a git component (boot expansion, runtime AddRequest for
	// "repo", runtime AddRequest for direct "git"), so the hardcoded
	// "main" fallback at this layer has been removed entirely. On
	// resolution failure, branch stays empty; workspace.clone then omits
	// --branch, and git uses the remote's actual default.
	branch := src.Branch
	if branch == "" && src.URL != "" {
		if resolved, err := workspace.ResolveDefaultBranch(ctx, src.URL, opts.GitToken); err == nil {
			branch = resolved
			slog.Debug("resolved remote default branch", "url", src.URL, "branch", resolved)
		} else {
			slog.Warn("default-branch resolution failed; git-source will clone the remote's actual default",
				"url", src.URL, "error", err)
		}
	}

	pollInterval := src.PollInterval
	if pollInterval == "" {
		pollInterval = "60s"
	}

	compCfg := map[string]any{
		"ports":         outputPorts(),
		"org":           org,
		"repo_path":     src.Path,
		"repo_url":      src.URL,
		"branch":        branch,
		"poll_interval": pollInterval,
		"watch_enabled": src.Watch,
		"workspace_dir": opts.WorkspaceDir,
		"git_token":     opts.GitToken,
		"branch_slug":   src.BranchSlug,
		"instance_name": instanceName,
	}
	if src.Submodules != nil {
		compCfg["submodules"] = *src.Submodules
	}

	return instanceName, compCfg, nil
}

// applyInstanceSuffix appends a slugified instance-name discriminator (set by
// submodule expansion — see SourceEntry.InstanceSuffix). Entity identity in
// the component's config is untouched; only the instance name changes.
func applyInstanceSuffix(name, suffix string) string {
	if suffix == "" {
		return name
	}
	if s := entityid.SystemSlug(suffix); s != "" {
		return name + "-" + s
	}
	return name
}

// docComponentConfig builds config for a docs source entry.
func docComponentConfig(src config.SourceEntry, org string) (string, map[string]any) {
	paths := src.Paths
	if len(paths) == 0 && src.Path != "" {
		paths = []string{src.Path}
	}
	slug := "docs-" + contentHashSlug(src)
	if len(paths) > 0 {
		if s := entityid.SystemSlug(paths[0]); s != "" {
			slug = s
		}
	}
	// An explicit project overrides the path-derived slug, mirroring ast:
	// submodule expansion registers the same canonical project from every
	// consumer checkout path (D9), and path slugs would fork identity.
	if src.Project != "" {
		if s := entityid.SystemSlug(src.Project); s != "" {
			slug = s
		}
	}
	scopedSlug := entityid.BranchScopedSlug(slug, src.BranchSlug)
	instanceName := applyInstanceSuffix(fmt.Sprintf("doc-source-%s", scopedSlug), src.InstanceSuffix)
	cfg := map[string]any{
		"ports":         outputPorts(),
		"org":           org,
		"paths":         paths,
		"watch_enabled": src.Watch,
		"instance_name": instanceName,
	}
	if src.Project != "" {
		cfg["project"] = src.Project
	}
	return instanceName, cfg
}

// cfgfileComponentConfig builds config for a config-file source entry.
func cfgfileComponentConfig(src config.SourceEntry, org string) (string, map[string]any) {
	paths := src.Paths
	if len(paths) == 0 && src.Path != "" {
		paths = []string{src.Path}
	}
	slug := "config-" + contentHashSlug(src)
	if len(paths) > 0 {
		if s := entityid.SystemSlug(paths[0]); s != "" {
			slug = s
		}
	}
	// Explicit project overrides the path-derived slug (see docComponentConfig).
	if src.Project != "" {
		if s := entityid.SystemSlug(src.Project); s != "" {
			slug = s
		}
	}
	scopedSlug := entityid.BranchScopedSlug(slug, src.BranchSlug)
	instanceName := applyInstanceSuffix(fmt.Sprintf("cfgfile-source-%s", scopedSlug), src.InstanceSuffix)
	cfg := map[string]any{
		"ports":         outputPorts(),
		"org":           org,
		"paths":         paths,
		"watch_enabled": src.Watch,
		"instance_name": instanceName,
	}
	if src.Project != "" {
		cfg["project"] = src.Project
	}
	return instanceName, cfg
}

// urlComponentConfig builds config for a URL source entry.
func urlComponentConfig(src config.SourceEntry, org string) (string, map[string]any) {
	urls := src.URLs
	if len(urls) == 0 && src.URL != "" {
		urls = []string{src.URL}
	}
	slug := "url-" + contentHashSlug(src)
	if len(urls) > 0 {
		if s := entityid.SystemSlug(urls[0]); s != "" {
			slug = s
		}
	}
	instanceName := fmt.Sprintf("url-source-%s", slug)
	pollInterval := src.PollInterval
	if pollInterval == "" {
		pollInterval = "300s"
	}
	return instanceName, map[string]any{
		"ports":         outputPorts(),
		"org":           org,
		"urls":          urls,
		"poll_interval": pollInterval,
		"instance_name": instanceName,
	}
}

// mediaComponentConfig builds config for image, video, or audio source entries.
func mediaComponentConfig(src config.SourceEntry, org string, opts Options) (string, map[string]any) {
	paths := src.Paths
	if len(paths) == 0 && src.Path != "" {
		paths = []string{src.Path}
	}
	slug := src.Type + "-" + contentHashSlug(src)
	if len(paths) > 0 {
		if s := entityid.SystemSlug(paths[0]); s != "" {
			slug = s
		}
	}
	instanceName := fmt.Sprintf("%s-source-%s", src.Type, slug)

	compCfg := map[string]any{
		"ports":         outputPorts(),
		"org":           org,
		"paths":         paths,
		"watch_enabled": src.Watch,
		"instance_name": instanceName,
	}

	if opts.MediaStoreDir != "" {
		compCfg["file_store_root"] = opts.MediaStoreDir
	}

	return instanceName, compCfg
}
