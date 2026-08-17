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

	// SkipSubmodules disables submodule materialization. The default
	// (false) materializes declared submodule trees after every clone and
	// pull — silent absence is the failure mode this package exists to
	// prevent, so recursion is opt-out, not opt-in.
	SkipSubmodules bool
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
		if err := pull(ctx, localPath, branch, repoURL, opt.Token); err != nil {
			return localPath, err
		}
	} else if err := clone(ctx, repoURL, branch, localPath, opt.Token); err != nil {
		return localPath, err
	}

	if opt.SkipSubmodules {
		return localPath, nil
	}
	return localPath, ensureSubmodules(ctx, localPath, repoURL, opt.Token)
}

// ensureSubmodules materializes the working trees of every submodule the
// checkout declares, recursively, at the commits its gitlinks pin. It runs
// after both clone and pull so a moved gitlink is synced before ingestion.
//
// The first attempt is shallow (--depth 1). A shallow fetch of a pinned
// commit FAILS when that commit is not the remote branch tip and the server
// does not allow direct SHA fetches — a routine situation for pinned
// submodules. On any failure the whole update is retried without --depth:
// already-materialized submodules are no-ops, so the retry only deepens the
// ones that failed.
func ensureSubmodules(ctx context.Context, repoPath, repoURL, token string) error {
	if _, err := os.Stat(filepath.Join(repoPath, ".gitmodules")); err != nil {
		return nil // no declared submodules — nothing to materialize
	}

	shallow := exec.CommandContext(ctx, "git", "submodule", "update", "--init", "--recursive", "--depth", "1")
	shallow.Dir = repoPath
	applyAuth(shallow, token, repoURL)
	shallowOut, shallowErr := shallow.CombinedOutput()
	if shallowErr == nil {
		return nil
	}

	full := exec.CommandContext(ctx, "git", "submodule", "update", "--init", "--recursive")
	full.Dir = repoPath
	applyAuth(full, token, repoURL)
	if out, err := full.CombinedOutput(); err != nil {
		return fmt.Errorf("workspace: git submodule update: %w\n%s\n(shallow attempt: %v\n%s)",
			err, string(out), shallowErr, string(shallowOut))
	}
	return nil
}

// IsRepoReady checks whether a directory is ready for ingestion.
// It returns nil if the path is usable:
//   - Path has no .git at all (not a git repo — ready as-is)
//   - Path has a .git DIRECTORY containing HEAD (clone complete)
//   - Path has a .git FILE (a worktree or submodule gitlink — see below)
//
// It returns an error if:
//   - Path does not exist
//   - Path has a .git directory but no HEAD (clone in progress)
//   - The working tree is still empty
//
// The .git-as-a-file case is the one worth spelling out. In a git worktree or a
// submodule, .git is not a directory but a one-line file pointing at the real
// gitdir elsewhere ("gitdir: /path/to/.git/worktrees/name"). Probing .git/HEAD
// there can never succeed — it is a path inside a regular file — so treating a
// missing HEAD as "clone in progress" reports a permanently in-flight clone for
// a checkout that is already complete. Callers retry persistently, so the source
// never ingests and the service sits at phase "seeding" forever with the reason
// only visible at debug level.
//
// Git writes the gitlink after the checkout populates, so its presence is itself
// evidence the tree is ready; the working-tree content check below still applies.
func IsRepoReady(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path not available: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	gitDir := filepath.Join(path, ".git")
	gitInfo, err := os.Stat(gitDir)
	if err != nil {
		// No .git — not a git repo, ready to use.
		return nil
	}

	if gitInfo.IsDir() {
		// A real .git directory — HEAD confirms the clone is initialized.
		head := filepath.Join(gitDir, "HEAD")
		if _, err := os.Stat(head); err != nil {
			return fmt.Errorf("git clone in progress: %s (.git exists but HEAD missing)", path)
		}
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
	applyAuth(cmd, token, repoURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("workspace: git clone: %w\n%s", err, string(out))
	}
	return nil
}

func pull(ctx context.Context, repoPath, branch, repoURL, token string) error {
	// Fetch latest refs from origin.
	cmd := exec.CommandContext(ctx, "git", "fetch", "origin")
	cmd.Dir = repoPath
	applyAuth(cmd, token, repoURL)
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
	applyAuth(cmd, token, repoURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("workspace: git pull: %w\n%s", err, string(out))
	}
	return nil
}

// applyAuth configures HTTPS token authentication on a git command.
// The token is injected via GIT_CONFIG environment variables so it never
// appears in on-disk git config. SSH URLs are unaffected.
// Requires Git 2.31+ (March 2021) for GIT_CONFIG_COUNT support.
//
// The extraheader key is scoped to repoURL's origin when it is an http(s)
// URL: submodule recursion means one git command can fetch from several
// hosts, and an unscoped header would send the parent repo's token to all
// of them. When repoURL is not http(s) the key falls back to the unscoped
// form (ssh transports ignore http headers entirely).
func applyAuth(cmd *exec.Cmd, token, repoURL string) {
	if token == "" {
		return
	}
	key := "http.extraheader"
	if u, err := url.Parse(repoURL); err == nil &&
		(u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		key = fmt.Sprintf("http.%s://%s/.extraheader", u.Scheme, u.Host)
	}
	// Use GIT_CONFIG_COUNT to inject an http.extraheader with the bearer
	// token. This is the same mechanism GitHub Actions uses. The header is
	// only applied to the current command — it does not persist in any
	// on-disk configuration.
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0="+key,
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
	applyAuth(cmd, token, repoURL)
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

// IsPathReady reports whether an ingestion path is ready to read, for sources
// whose configuration legitimately names FILES as well as directories
// (doc-source takes "README.md", cfgfile-source takes "go.mod").
//
// A directory delegates to IsRepoReady, because a directory may be a git
// checkout still being populated. A regular file that exists is ready by
// definition: git writes a file's content as part of the checkout, so its
// presence is already the evidence IsRepoReady's directory probes go looking
// for.
//
// Why this exists as a separate function rather than a relaxation of
// IsRepoReady: IsRepoReady is documented as a directory check and ast-source and
// git-source depend on that — for them a non-directory IS the error. Passing a
// file path to it was a category error at the call site, and it was silent
// because the resulting failure only appeared at debug level while the caller
// retried persistently. Under semstreams beta.158 the manager still came up, so
// the source merely never ingested; beta.159's component-start barrier makes the
// same condition fail the whole boot (#719, fail-closed boot).
func IsPathReady(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path not available: %w", err)
	}
	if info.IsDir() {
		return IsRepoReady(path)
	}
	return nil
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
