// Package workspace manages local clones of remote git repositories.
// It provides EnsureRepo, which clones a repository on first use and
// pulls updates on subsequent calls, giving handlers a stable local path.
package workspace

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Options configures optional EnsureRepo behaviour.
type Options struct {
	// Token is a personal access token or app installation token for
	// HTTPS authentication. When set and the URL uses HTTPS, the token is
	// injected via the GIT_ASKPASS mechanism so it never appears in process
	// listings or on-disk git config. SSH URLs are unaffected.
	Token string
}

// EnsureRepo clones or pulls a git repository into baseDir/{slug}.
// Returns the local path to the repository.
// If the repository already exists (a .git directory is present), it fetches
// and pulls instead of cloning. The branch parameter is optional; when empty
// the remote's default branch is used.
func EnsureRepo(ctx context.Context, repoURL, branch, baseDir string, opts ...Options) (string, error) {
	if repoURL == "" {
		return "", fmt.Errorf("workspace: repo URL is required")
	}
	if baseDir == "" {
		return "", fmt.Errorf("workspace: base directory is required")
	}

	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}

	slug := URLToSlug(repoURL)
	localPath := filepath.Join(baseDir, slug)

	// If .git exists, pull; otherwise clone.
	if _, err := os.Stat(filepath.Join(localPath, ".git")); err == nil {
		return localPath, pull(ctx, localPath, branch, opt.Token)
	}

	return localPath, clone(ctx, repoURL, branch, localPath, opt.Token)
}

// IsRepoReady checks whether a directory is ready for ingestion.
// It returns nil if the path is usable:
//   - Path has no .git directory (not a git repo — ready as-is)
//   - Path has .git/HEAD (clone complete)
//
// It returns an error if:
//   - Path does not exist
//   - Path has .git but no HEAD (clone in progress)
func IsRepoReady(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path not available: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		// No .git directory — not a git repo, ready to use.
		return nil
	}

	// .git exists — check for HEAD to confirm clone is initialized.
	head := filepath.Join(gitDir, "HEAD")
	if _, err := os.Stat(head); err != nil {
		return fmt.Errorf("git clone in progress: %s (.git exists but HEAD missing)", path)
	}

	// HEAD can exist before checkout completes. Verify the working tree has
	// at least one entry beyond .git to confirm the checkout is done.
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("git clone in progress: %s: %w", path, err)
	}
	for _, e := range entries {
		if e.Name() != ".git" {
			return nil // Working tree has content — checkout complete.
		}
	}
	return fmt.Errorf("git clone in progress: %s (working tree empty)", path)
}

// URLToSlug converts a git URL to a filesystem-safe slug.
// Example: "https://github.com/opensensorhub/osh-core" → "github-com-opensensorhub-osh-core"
func URLToSlug(rawURL string) string {
	// Handle SSH shorthand: git@github.com:owner/repo.git → github.com/owner/repo.git
	if strings.HasPrefix(rawURL, "git@") {
		rawURL = strings.TrimPrefix(rawURL, "git@")
		rawURL = strings.Replace(rawURL, ":", "/", 1)
	}

	// Strip .git suffix before any further processing so it is removed
	// regardless of which parse path we take below.
	rawURL = strings.TrimSuffix(rawURL, ".git")

	// Parse as URL to extract host + path.
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		// Fallback: just slugify the whole string.
		return slugify(rawURL)
	}

	combined := parsed.Host + parsed.Path
	return slugify(combined)
}

func clone(ctx context.Context, repoURL, branch, dest, token string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("workspace: mkdir: %w", err)
	}

	args := []string{"clone", "--depth", "1"}
	if branch != "" {
		if err := validateBranchName(branch); err != nil {
			return err
		}
		args = append(args, "--branch", branch)
	}
	args = append(args, repoURL, dest)

	cmd := exec.CommandContext(ctx, "git", args...)
	applyAuth(cmd, token)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("workspace: git clone: %w\n%s", err, string(out))
	}
	return nil
}

func pull(ctx context.Context, repoPath, branch, token string) error {
	// Fetch latest refs from origin.
	cmd := exec.CommandContext(ctx, "git", "fetch", "origin")
	cmd.Dir = repoPath
	applyAuth(cmd, token)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("workspace: git fetch: %w\n%s", err, string(out))
	}

	// Checkout the requested branch if specified.
	if branch != "" {
		if err := validateBranchName(branch); err != nil {
			return err
		}
		cmd = exec.CommandContext(ctx, "git", "checkout", branch)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("workspace: git checkout %s: %w\n%s", branch, err, string(out))
		}
	}

	// Fast-forward pull; fail loudly if the local branch has diverged.
	cmd = exec.CommandContext(ctx, "git", "pull", "--ff-only")
	cmd.Dir = repoPath
	applyAuth(cmd, token)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("workspace: git pull: %w\n%s", err, string(out))
	}
	return nil
}

// applyAuth configures HTTPS token authentication on a git command.
// The token is injected via GIT_CONFIG environment variables so it never
// appears in on-disk git config. SSH URLs are unaffected.
// Requires Git 2.31+ (March 2021) for GIT_CONFIG_COUNT support.
func applyAuth(cmd *exec.Cmd, token string) {
	if token == "" {
		return
	}
	// Use GIT_CONFIG_COUNT to inject an http.extraheader with the bearer
	// token. This is the same mechanism GitHub Actions uses. The header is
	// only applied to the current command — it does not persist in any
	// on-disk configuration.
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.extraheader",
		"GIT_CONFIG_VALUE_0=Authorization: bearer "+token,
		"GIT_TERMINAL_PROMPT=0",
	)
}

// validateBranchName rejects branch names that could be interpreted as git flags.
func validateBranchName(branch string) error {
	if strings.HasPrefix(branch, "-") {
		return fmt.Errorf("workspace: invalid branch name %q: must not start with '-'", branch)
	}
	return nil
}

// ResolveDefaultBranch returns the remote's default branch by running
// `git ls-remote --symref <url> HEAD` and parsing the symref. Avoids the
// hardcoded "main" assumption that breaks pre-rename repos (master),
// custom defaults (develop, trunk), or repos that have switched defaults.
//
// repoURL may be any form git understands: https://, git://, ssh://,
// git@host:path, or a local filesystem path.
//
// When git is unreachable, the URL is unauthenticated, or the remote's
// HEAD does not resolve to a refs/heads/* target (e.g. a detached default
// or an empty repo), the function returns an error and the caller should
// fall back to the static default rather than blocking the workflow.
func ResolveDefaultBranch(ctx context.Context, repoURL, token string) (string, error) {
	if repoURL == "" {
		return "", fmt.Errorf("workspace: repo URL is required")
	}
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--symref", repoURL, "HEAD")
	applyAuth(cmd, token)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("workspace: git ls-remote --symref %s HEAD: %w\n%s", repoURL, err, string(out))
	}
	return parseSymrefHEAD(string(out))
}

// parseSymrefHEAD extracts the branch name from `git ls-remote --symref`
// output. The first line is shaped like:
//
//	ref: refs/heads/<branch>\tHEAD
//
// Older or unusual servers may put the symref line later, so scan all
// lines rather than assuming line 0.
func parseSymrefHEAD(out string) (string, error) {
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "ref: ")
		if !ok {
			continue
		}
		// Format: "ref: refs/heads/<name>\tHEAD" — split on tab/whitespace,
		// keep the ref target.
		ref := strings.SplitN(rest, "\t", 2)[0]
		ref = strings.Fields(ref)[0]
		branch, ok := strings.CutPrefix(ref, "refs/heads/")
		if !ok {
			// HEAD points somewhere unusual (e.g. refs/tags/* or detached);
			// we cannot map this to a clonable branch.
			return "", fmt.Errorf("workspace: HEAD symref %q is not under refs/heads/", ref)
		}
		return branch, nil
	}
	return "", fmt.Errorf("workspace: no symref for HEAD in ls-remote output")
}

// BranchSlug converts a branch name to a filesystem-safe slug.
// Example: "scenario/auth-flow" → "scenario-auth-flow"
func BranchSlug(branch string) string {
	return slugify(branch)
}

// WorktreeInfo describes a git worktree.
type WorktreeInfo struct {
	Path   string // absolute filesystem path
	Branch string // branch name (empty for detached HEAD)
}

// ListBranches returns all local branch names in the repository at repoPath.
func ListBranches(ctx context.Context, repoPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("workspace: git for-each-ref: %w\n%s", err, string(out))
	}
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

// ListWorktrees returns all worktrees for the repository at repoPath.
func ListWorktrees(ctx context.Context, repoPath string) ([]WorktreeInfo, error) {
	cmd := exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("workspace: git worktree list: %w\n%s", err, string(out))
	}

	var worktrees []WorktreeInfo
	var current WorktreeInfo
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			current = WorktreeInfo{Path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "":
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = WorktreeInfo{}
			}
		}
	}
	// Flush final block if output did not end with a blank line.
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}
	return worktrees, nil
}

// MatchBranches returns branches that match any of the given glob patterns.
// Uses filepath.Match semantics for each pattern.
func MatchBranches(branches []string, patterns []string) []string {
	var matched []string
	for _, branch := range branches {
		for _, pattern := range patterns {
			ok, err := filepath.Match(pattern, branch)
			if err == nil && ok {
				matched = append(matched, branch)
				break
			}
		}
	}
	return matched
}

// EnsureWorktree ensures a git worktree exists for the given branch.
// If a worktree for the branch already exists (created externally or by a
// previous call), returns its path. Otherwise creates one at
// worktreeDir/{BranchSlug(branch)}.
// repoPath is the path to the main repository (or any existing worktree).
func EnsureWorktree(ctx context.Context, repoPath, branch, worktreeDir string) (string, error) {
	if err := validateBranchName(branch); err != nil {
		return "", err
	}

	worktrees, err := ListWorktrees(ctx, repoPath)
	if err != nil {
		return "", err
	}
	for _, wt := range worktrees {
		if wt.Branch == branch {
			return wt.Path, nil
		}
	}

	dest := filepath.Join(worktreeDir, BranchSlug(branch))
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", dest, branch)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("workspace: git worktree add: %w\n%s", err, string(out))
	}
	return dest, nil
}

// slugify converts an arbitrary string into a lowercase, hyphen-separated
// filesystem-safe identifier. Consecutive hyphens are collapsed, and leading
// or trailing hyphens are stripped.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	result := b.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
}
