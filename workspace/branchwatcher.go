package workspace

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// BranchState describes a discovered branch and its worktree.
type BranchState struct {
	Branch       string // full branch name, e.g. "scenario/auth-flow"
	Slug         string // filesystem-safe slug, e.g. "scenario-auth-flow"
	WorktreePath string // absolute path to worktree directory
}

// BranchWatcher periodically discovers branches matching glob patterns
// and ensures each has a worktree. It reports added and removed branches
// as deltas on each Discover call.
type BranchWatcher struct {
	repoPath     string
	patterns     []string
	worktreeBase string
	maxBranches  int
	logger       *slog.Logger

	mu    sync.Mutex
	known map[string]BranchState // branch name → state
}

// BranchWatcherConfig configures a BranchWatcher.
type BranchWatcherConfig struct {
	RepoPath     string   // path to the git repository
	Patterns     []string // branch name glob patterns
	WorktreeBase string   // base directory for managed worktrees
	MaxBranches  int      // safety cap (0 = default 50)
	Logger       *slog.Logger
}

// NewBranchWatcher creates a new BranchWatcher.
func NewBranchWatcher(cfg BranchWatcherConfig) *BranchWatcher {
	max := cfg.MaxBranches
	if max <= 0 {
		max = 50
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &BranchWatcher{
		repoPath:     cfg.RepoPath,
		patterns:     cfg.Patterns,
		worktreeBase: cfg.WorktreeBase,
		maxBranches:  max,
		logger:       logger,
		known:        make(map[string]BranchState),
	}
}

// Discover checks for branches matching the configured patterns and ensures
// each has a worktree. Returns branches that were added or removed since
// the last call.
func (bw *BranchWatcher) Discover(ctx context.Context) (added []BranchState, removed []string, err error) {
	branches, err := ListBranches(ctx, bw.repoPath)
	if err != nil {
		return nil, nil, err
	}

	matched := MatchBranches(branches, bw.patterns)

	// Also discover existing worktrees that match patterns (created externally).
	worktrees, wtErr := ListWorktrees(ctx, bw.repoPath)
	if wtErr != nil {
		bw.logger.Debug("failed to list worktrees", "error", wtErr)
	}
	wtByBranch := make(map[string]string, len(worktrees))
	for _, wt := range worktrees {
		if wt.Branch != "" {
			wtByBranch[wt.Branch] = wt.Path
		}
	}

	// Add any branches that have external worktrees but weren't in the
	// local branch list (e.g. detached worktrees).
	for branch := range wtByBranch {
		if len(MatchBranches([]string{branch}, bw.patterns)) > 0 {
			found := false
			for _, m := range matched {
				if m == branch {
					found = true
					break
				}
			}
			if !found {
				matched = append(matched, branch)
			}
		}
	}

	bw.mu.Lock()
	defer bw.mu.Unlock()

	// Detect added branches.
	current := make(map[string]bool, len(matched))
	for _, branch := range matched {
		current[branch] = true
		if _, exists := bw.known[branch]; exists {
			continue
		}
		if len(bw.known) >= bw.maxBranches {
			bw.logger.Warn("max branches reached, skipping",
				"branch", branch,
				"max", bw.maxBranches)
			continue
		}

		// Use existing worktree if available, otherwise create one.
		var wtPath string
		if existing, ok := wtByBranch[branch]; ok {
			wtPath = existing
		} else {
			wtPath, err = EnsureWorktree(ctx, bw.repoPath, branch, bw.worktreeBase)
			if err != nil {
				bw.logger.Warn("failed to ensure worktree",
					"branch", branch,
					"error", err)
				continue
			}
		}

		state := BranchState{
			Branch:       branch,
			Slug:         BranchSlug(branch),
			WorktreePath: wtPath,
		}
		bw.known[branch] = state
		added = append(added, state)
	}

	// Detect removed branches.
	for branch := range bw.known {
		if !current[branch] {
			removed = append(removed, branch)
		}
	}
	for _, branch := range removed {
		delete(bw.known, branch)
	}

	return added, removed, nil
}

// Known returns a snapshot of all currently known branches.
func (bw *BranchWatcher) Known() []BranchState {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	states := make([]BranchState, 0, len(bw.known))
	for _, s := range bw.known {
		states = append(states, s)
	}
	return states
}

// Run starts the periodic discovery loop. It blocks until ctx is cancelled.
// The onChange callback is invoked whenever branches are added or removed.
func (bw *BranchWatcher) Run(ctx context.Context, interval time.Duration, onChange func(added []BranchState, removed []string)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			added, removed, err := bw.Discover(ctx)
			if err != nil {
				bw.logger.Warn("branch discovery failed", "error", err)
				continue
			}
			if len(added) > 0 || len(removed) > 0 {
				onChange(added, removed)
			}
		}
	}
}
