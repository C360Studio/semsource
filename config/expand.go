package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/workspace"
)

// BranchWatcherRef describes one multi-branch source's discovery: what boot
// reconcile derives ownership from and the read-only runtime check lists.
// Returned by ExpandRepoSources when a repo source has Branches set.
type BranchWatcherRef struct {
	RepoPath           string
	Patterns           []string
	WorktreeBase       string
	BranchPollInterval string
	MaxBranches        int
	Language           string
	Languages          []string
	Version            string
	Project            string
	Watch              bool
	Org                string // needed for component config building
	// Source is the multi-branch repo entry this ref was expanded from.
	Source SourceEntry
	// Entries are the branch sources this boot admitted for Source (empty
	// unless discovery succeeded); boot reconciliation admits them whatever
	// retained arbitration decided.
	Entries []SourceEntry
}

// Discovery states a discovering source reports (source-discovery spec).
const (
	DiscoveryOK               = "ok"
	DiscoveryAwaitingCheckout = "awaiting_checkout"
	DiscoveryListingFailed    = "listing_failed"
	DiscoveryCapped           = "capped"
)

// DiscoveryState is one discovering source's boot-time discovery outcome.
type DiscoveryState struct {
	Source      string // the repo URL or path
	Kind        string // "branch" or "submodule"
	State       string
	Error       string
	NotAdmitted int // matches left out by max_branches (capped)
	// Admitted is what this boot admitted: branch name to "" for branches,
	// submodule path to gitlink SHA for submodules.
	Admitted map[string]string
	// Skipped are matching branches within the cap whose worktree could not
	// be created at boot; no restart admits them until the cause is fixed.
	Skipped []string
}

// ExpandResult holds the output of ExpandRepoSources.
type ExpandResult struct {
	Sources  []SourceEntry
	Watchers []BranchWatcherRef
	// Discovery holds one state per multi-branch source.
	Discovery []DiscoveryState
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
// For single-branch repo sources, src.Branch is passed through unchanged;
// default-branch resolution (and any git auth it needs) lives at the leaf
// (sourcespawn.gitComponentConfig) so it covers every git-producing path
// uniformly. See the inline note at the single-branch expansion below.
//
// workspaceDir is the base directory where repos will be cloned.
func ExpandRepoSources(ctx context.Context, sources []SourceEntry, workspaceDir string) (*ExpandResult, error) {
	var expanded []SourceEntry
	var watchers []BranchWatcherRef
	var states []DiscoveryState

	for _, src := range sources {
		if src.Type != "repo" {
			expanded = append(expanded, src)
			continue
		}

		if src.URL == "" && src.Path == "" {
			return nil, fmt.Errorf("repo source: url or path is required")
		}

		if len(src.Branches) > 0 {
			// Discovery never fails boot: a failure is the source's state.
			entries, watcher, state := expandMultiBranch(ctx, src, workspaceDir)
			expanded = append(expanded, entries...)
			if state.State == DiscoveryOK || state.State == DiscoveryCapped {
				watcher.Entries = entries
			}
			watchers = append(watchers, *watcher)
			states = append(states, state)
			continue
		}

		// Default-branch resolution lives at the leaf
		// (sourcespawn.gitComponentConfig) so it covers every git-
		// producing path uniformly: boot expansion, runtime AddRequest
		// for "repo", runtime AddRequest for direct "git". Expansion
		// here just passes src.Branch through; the leaf will resolve.
		entries := expandSingleBranch(src, workspaceDir)
		expanded = append(expanded, entries...)
	}

	return &ExpandResult{Sources: expanded, Watchers: watchers, Discovery: states}, nil
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
		Submodules: src.Submodules,
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
	if len(src.Languages) > 0 {
		astEntry.Languages = src.Languages
	}
	if src.Version != "" {
		astEntry.Version = src.Version
	}
	if src.Project != "" {
		astEntry.Project = src.Project
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
// of 4 source entries with branch-scoped paths and slugs. A remote source with
// no checkout yet expands to its git source alone, which clones it; a listing
// failure expands to nothing. Either is reported in the returned state.
func expandMultiBranch(ctx context.Context, src SourceEntry, workspaceDir string) ([]SourceEntry, *BranchWatcherRef, DiscoveryState) {
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

	ref := &BranchWatcherRef{
		RepoPath:           repoPath,
		Patterns:           src.Branches,
		WorktreeBase:       worktreeBase,
		BranchPollInterval: src.BranchPollInterval,
		MaxBranches:        src.MaxBranches,
		Language:           src.Language,
		Languages:          src.Languages,
		Version:            src.Version,
		Project:            src.Project,
		Watch:              src.Watch,
		Source:             src,
	}
	state := DiscoveryState{Source: src.URL + src.Path, Kind: "branch", State: DiscoveryOK}

	if _, err := os.Stat(repoPath); errors.Is(err, fs.ErrNotExist) && src.URL != "" {
		state.State = DiscoveryAwaitingCheckout
		return []SourceEntry{ParentCloneEntry(src)}, ref, state
	}
	branches, err := workspace.ListBranches(ctx, repoPath)
	if err != nil {
		state.State, state.Error = DiscoveryListingFailed, err.Error()
		return nil, ref, state
	}
	matchedNames := workspace.MatchBranches(branches, src.Branches)
	matched := len(matchedNames)

	bw := workspace.NewBranchWatcher(workspace.BranchWatcherConfig{
		RepoPath:     repoPath,
		Patterns:     src.Branches,
		WorktreeBase: worktreeBase,
		MaxBranches:  src.MaxBranches,
	})
	added, _, err := bw.Discover(ctx)
	if err != nil {
		state.State, state.Error = DiscoveryListingFailed, err.Error()
		return nil, ref, state
	}
	limit := src.MaxBranches
	if limit <= 0 {
		limit = workspace.DefaultMaxBranches
	}
	if len(added) >= limit && matched > len(added) {
		state.State, state.NotAdmitted = DiscoveryCapped, matched-len(added)
	}

	var entries []SourceEntry
	state.Admitted = map[string]string{}
	for _, bs := range added {
		state.Admitted[bs.Branch] = ""
		entries = append(entries, expandBranchSources(src, bs)...)
	}
	state.Skipped = skippedBranches(matchedNames, state.Admitted, limit)
	if state.State == DiscoveryCapped {
		state.NotAdmitted -= len(state.Skipped)
	}
	if len(state.Skipped) > 0 {
		state.Error = fmt.Sprintf("no worktree could be created for %s", strings.Join(state.Skipped, ", "))
	}
	return entries, ref, state
}

// skippedBranches replays the cap the branch watcher applied over matched
// local branches: a branch reached before the cap filled that was not
// admitted failed its worktree.
func skippedBranches(matched []string, admitted map[string]string, limit int) []string {
	var skipped []string
	count := 0
	for _, b := range matched {
		if _, ok := admitted[b]; ok {
			count++
		} else if count < limit {
			skipped = append(skipped, b)
		}
	}
	return skipped
}

// ParentCloneEntry is the git source that clones a remote multi-branch
// source's base checkout, admitted while the checkout does not exist yet.
func ParentCloneEntry(src SourceEntry) SourceEntry {
	return SourceEntry{Type: "git", URL: src.URL, Branch: src.Branch, Watch: src.Watch}
}

// BranchEntries expands one branch of a multi-branch source into its four
// source entries; exported so boot reconciliation derives the component
// names a branch owns.
func BranchEntries(src SourceEntry, bs workspace.BranchState) []SourceEntry {
	return expandBranchSources(src, bs)
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
	if len(src.Languages) > 0 {
		astEntry.Languages = src.Languages
	}
	if src.Version != "" {
		astEntry.Version = src.Version
	}
	if src.Project != "" {
		astEntry.Project = src.Project
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
