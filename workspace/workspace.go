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

	args := []string{"clone"}
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
