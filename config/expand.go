package config

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/c360studio/semsource/workspace"
)

// BranchWatcherRef captures the configuration needed to start a BranchWatcher
// at runtime for dynamic branch discovery. Returned by ExpandRepoSources when
// a repo source has Branches set.
type BranchWatcherRef struct {
	RepoPath           string
	Patterns           []string
	WorktreeBase       string
	BranchPollInterval string
	MaxBranches        int
	Language           string
	Watch              bool
	Org                string // needed for component config building
}

// ExpandResult holds the output of ExpandRepoSources.
type ExpandResult struct {
	Sources  []SourceEntry
	Watchers []BranchWatcherRef
}

// ExpandRepoSources expands any "repo" meta-source entries into their
// component sources: git, ast, docs, and config. Non-repo entries are
// passed through unchanged. The git source is always first in each
// expansion group so it clones the repo before other handlers try to
// read the directory.
//
// When a repo source has Branches set (multi-branch mode), each discovered
// branch is expanded into its own set of 4 components with branch-scoped
// paths and entity IDs.
//
// workspaceDir is the base directory where repos will be cloned.
func ExpandRepoSources(ctx context.Context, sources []SourceEntry, workspaceDir string) (*ExpandResult, error) {
	var expanded []SourceEntry
	var watchers []BranchWatcherRef

	for _, src := range sources {
		if src.Type != "repo" {
			expanded = append(expanded, src)
			continue
		}

		if src.URL == "" && src.Path == "" {
			return nil, fmt.Errorf("repo source: url or path is required")
		}

		if len(src.Branches) > 0 {
			entries, watcher, err := expandMultiBranch(ctx, src, workspaceDir)
			if err != nil {
				return nil, fmt.Errorf("multi-branch expansion: %w", err)
			}
			expanded = append(expanded, entries...)
			if watcher != nil {
				watchers = append(watchers, *watcher)
			}
			continue
		}

		entries := expandSingleBranch(src, workspaceDir)
		expanded = append(expanded, entries...)
	}

	return &ExpandResult{Sources: expanded, Watchers: watchers}, nil
}

// expandSingleBranch expands a repo source with a single branch (existing behavior).
// src.BranchSlug propagates onto each child entry so per-branch instance names
// stay scoped (e.g. branch-watcher discoveries don't collide with the default
// branch's KV keys).
func expandSingleBranch(src SourceEntry, workspaceDir string) []SourceEntry {
	localPath := src.Path
	if localPath == "" && src.URL != "" {
		slug := workspace.URLToSlug(src.URL)
		localPath = filepath.Join(workspaceDir, slug)
	}

	var entries []SourceEntry

	// 1. Git source — clones the repo (auto-clone resolves URL → local path)
	entries = append(entries, SourceEntry{
		Type:       "git",
		URL:        src.URL,
		Path:       src.Path,
		Branch:     src.Branch,
		Watch:      src.Watch,
		BranchSlug: src.BranchSlug,
	})

	// 2. AST source — code structure analysis
	astEntry := SourceEntry{
		Type:       "ast",
		Path:       localPath,
		Watch:      src.Watch,
		BranchSlug: src.BranchSlug,
	}
	if src.Language != "" {
		astEntry.Language = src.Language
	}
	entries = append(entries, astEntry)

	// 3. Docs source — markdown/text docs
	entries = append(entries, SourceEntry{
		Type:       "docs",
		Paths:      []string{localPath},
		Watch:      src.Watch,
		BranchSlug: src.BranchSlug,
	})

	// 4. Config source — go.mod, package.json, pom.xml, Dockerfile, etc.
	entries = append(entries, SourceEntry{
		Type:       "config",
		Paths:      []string{localPath},
		Watch:      src.Watch,
		BranchSlug: src.BranchSlug,
	})

	return entries
}

// expandMultiBranch discovers matching branches and expands each into a set
// of 4 source entries with branch-scoped paths and slugs.
func expandMultiBranch(ctx context.Context, src SourceEntry, workspaceDir string) ([]SourceEntry, *BranchWatcherRef, error) {
	repoPath := src.Path
	if repoPath == "" && src.URL != "" {
		slug := workspace.URLToSlug(src.URL)
		repoPath = filepath.Join(workspaceDir, slug)
	}

	worktreeBase := filepath.Join(workspaceDir, "worktrees", workspace.BranchSlug(repoPath))
	if src.Path != "" {
		// For local repos, put worktrees alongside the repo.
		worktreeBase = filepath.Join(filepath.Dir(src.Path), ".semsource-worktrees")
	}

	// Initial discovery of matching branches.
	bw := workspace.NewBranchWatcher(workspace.BranchWatcherConfig{
		RepoPath:     repoPath,
		Patterns:     src.Branches,
		WorktreeBase: worktreeBase,
		MaxBranches:  src.MaxBranches,
	})
	added, _, err := bw.Discover(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("initial branch discovery for %s: %w", repoPath, err)
	}

	// Expand each discovered branch into 4 source entries.
	var entries []SourceEntry
	for _, bs := range added {
		branchEntries := expandBranchSources(src, bs)
		entries = append(entries, branchEntries...)
	}

	// Return watcher ref so run.go can start the poll loop.
	ref := &BranchWatcherRef{
		RepoPath:           repoPath,
		Patterns:           src.Branches,
		WorktreeBase:       worktreeBase,
		BranchPollInterval: src.BranchPollInterval,
		MaxBranches:        src.MaxBranches,
		Language:           src.Language,
		Watch:              src.Watch,
	}

	return entries, ref, nil
}

// expandBranchSources creates 4 source entries for a single branch.
func expandBranchSources(src SourceEntry, bs workspace.BranchState) []SourceEntry {
	var entries []SourceEntry

	// 1. Git source for this branch
	entries = append(entries, SourceEntry{
		Type:       "git",
		URL:        src.URL,
		Path:       bs.WorktreePath,
		Branch:     bs.Branch,
		Watch:      src.Watch,
		BranchSlug: bs.Slug,
	})

	// 2. AST source pointing at worktree
	astEntry := SourceEntry{
		Type:       "ast",
		Path:       bs.WorktreePath,
		Watch:      src.Watch,
		BranchSlug: bs.Slug,
	}
	if src.Language != "" {
		astEntry.Language = src.Language
	}
	entries = append(entries, astEntry)

	// 3. Docs source pointing at worktree
	entries = append(entries, SourceEntry{
		Type:       "docs",
		Paths:      []string{bs.WorktreePath},
		Watch:      src.Watch,
		BranchSlug: bs.Slug,
	})

	// 4. Config source pointing at worktree
	entries = append(entries, SourceEntry{
		Type:       "config",
		Paths:      []string{bs.WorktreePath},
		Watch:      src.Watch,
		BranchSlug: bs.Slug,
	})

	return entries
}
