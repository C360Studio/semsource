// Package git implements the SourceHandler for git repositories.
// It clones or opens local repos, walks commit history, and emits
// commit/author/branch RawEntity values for the normalizer.
package git

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	"github.com/c360studio/semsource/workspace"
)

// allowedProtocols lists git URL schemes that are permitted.
var allowedProtocols = map[string]bool{
	"https": true,
	"git":   true,
	"ssh":   true,
}

// Config controls Handler behaviour.
type Config struct {
	// PollInterval is how often Watch polls for new commits.
	// Default: 30s.
	PollInterval time.Duration

	// MaxCommits caps the number of commits ingested per Ingest call.
	// 0 means unlimited.
	MaxCommits int

	// WorkspaceDir is the base directory used when auto-cloning remote
	// repositories. Required when a source provides only a URL (no local path).
	// Defaults to the value inherited from config.Config.WorkspaceDir.
	WorkspaceDir string

	// Token is a personal access token or GitHub App installation token
	// for authenticating HTTPS clones of private repositories. When set,
	// it is passed to the workspace package via GIT_CONFIG environment
	// variables. SSH URLs ignore the token and rely on the user's SSH agent.
	Token string

	// Org is the organisation namespace used when building typed EntityState values
	// via IngestEntityStates. Required for the git-source processor's normalizer-free path.
	Org string

	// BranchSlug, when non-empty, scopes entity IDs to a specific branch.
	// Used in multi-branch mode to prevent entity ID collisions across branches.
	BranchSlug string

	// Logger, when non-nil, receives structured logs from Watch/pollLoop
	// operations. Without a logger, polling errors are invisible — the handler
	// silently skips failed ticks. Pass the component's logger here so poll
	// failures surface in the operational log.
	Logger *slog.Logger
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		PollInterval: 30 * time.Second,
		MaxCommits:   0,
	}
}

// Handler implements handler.SourceHandler for git sources.
// It is safe for concurrent use.
type Handler struct {
	cfg Config

	// watchErrors counts errors observed by the polling loop that would
	// otherwise be swallowed (headCommitSHA failures, Ingest failures during
	// a poll tick). Readable via WatchErrorCount so the owning component can
	// surface the value in its status report.
	watchErrors atomic.Int64
}

// New creates a Handler with the given configuration.
func New(cfg Config) *Handler {
	return &Handler{cfg: cfg}
}

// WatchErrorCount returns the cumulative number of errors encountered by the
// polling watch loop. Returns 0 before Watch() has been called or if watching
// is disabled.
func (h *Handler) WatchErrorCount() int64 {
	return h.watchErrors.Load()
}

// logger returns the configured logger or slog.Default() when none is set.
func (h *Handler) logger() *slog.Logger {
	if h.cfg.Logger != nil {
		return h.cfg.Logger
	}
	return slog.Default()
}

// SourceType implements handler.SourceHandler.
func (h *Handler) SourceType() string { return handler.SourceTypeGit }

// Supports implements handler.SourceHandler.
func (h *Handler) Supports(cfg handler.SourceConfig) bool {
	return cfg.GetType() == handler.SourceTypeGit
}

// Ingest resolves the local repo path (cloning if necessary), walks its commit
// log, and returns RawEntity values for commits, authors, and the current branch.
// When a URL is configured without a local path, EnsureRepo clones the repository
// into WorkspaceDir on the first call and pulls on subsequent calls.
func (h *Handler) Ingest(ctx context.Context, cfg handler.SourceConfig) ([]handler.RawEntity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	repoPath, err := h.resolveRepoPath(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Derive a stable system slug — from the URL when available so entity IDs
	// are identical across machines regardless of where the repo was cloned.
	system := h.systemSlug(cfg)

	// Get current branch.
	branch, err := currentBranch(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("git handler: get branch: %w", err)
	}

	// Get HEAD SHA.
	head, err := headCommitSHA(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("git handler: get HEAD: %w", err)
	}

	// Walk commit log.
	commits, err := listCommits(ctx, repoPath, h.cfg.MaxCommits)
	if err != nil {
		return nil, fmt.Errorf("git handler: list commits: %w", err)
	}

	var entities []handler.RawEntity
	seenAuthors := make(map[string]bool)

	for _, c := range commits {
		commitEntity := BuildCommitEntity(c.sha, c.authorFull, c.subject, system)
		// Add file-touch edges.
		for _, f := range c.files {
			commitEntity.Edges = append(commitEntity.Edges, handler.RawEdge{
				FromHint: shortSHA(c.sha),
				ToHint:   f,
				EdgeType: "touches",
			})
		}
		// Add authored-by edge.
		commitEntity.Edges = append(commitEntity.Edges, handler.RawEdge{
			FromHint: shortSHA(c.sha),
			ToHint:   c.authorEmail,
			EdgeType: "authored_by",
		})
		entities = append(entities, commitEntity)

		// One author entity per unique email.
		if !seenAuthors[c.authorEmail] {
			seenAuthors[c.authorEmail] = true
			entities = append(entities, BuildAuthorEntity(c.authorName, c.authorEmail, system))
		}
	}

	// Branch entity.
	entities = append(entities, BuildBranchEntity(branch, head, system))

	return entities, nil
}

// IngestEntityStates resolves the repo path, walks commit history, and returns
// fully-typed entity states that embed vocabulary-predicate triples directly —
// bypassing the normalizer entirely. The org parameter is the organisation
// namespace (e.g. "acme") used in the 6-part entity ID.
func (h *Handler) IngestEntityStates(ctx context.Context, cfg handler.SourceConfig, org string) ([]*handler.EntityState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	repoPath, err := h.resolveRepoPath(ctx, cfg)
	if err != nil {
		return nil, err
	}

	system := h.systemSlug(cfg)
	now := time.Now().UTC()

	branch, err := currentBranch(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("git handler: get branch: %w", err)
	}

	head, err := headCommitSHA(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("git handler: get HEAD: %w", err)
	}

	commits, err := listCommits(ctx, repoPath, h.cfg.MaxCommits)
	if err != nil {
		return nil, fmt.Errorf("git handler: list commits: %w", err)
	}

	var states []*handler.EntityState
	seenAuthors := make(map[string]bool)

	for _, c := range commits {
		ce := newCommitEntity(org, c.sha, c.authorFull, c.subject, system, now)
		ce.TouchedFiles = c.files
		ce.AuthorEmail = c.authorEmail
		states = append(states, ce.EntityState())

		if !seenAuthors[c.authorEmail] {
			seenAuthors[c.authorEmail] = true
			ae := newAuthorEntity(org, c.authorName, c.authorEmail, system, now)
			states = append(states, ae.EntityState())
		}
	}

	be := newBranchEntity(org, branch, head, system, now)
	states = append(states, be.EntityState())

	return states, nil
}

// Watch starts a polling loop that emits a ChangeEvent whenever the HEAD
// commit SHA changes. Returns (nil, nil) when watching is not enabled.
// The returned channel is closed when ctx is cancelled.
func (h *Handler) Watch(ctx context.Context, cfg handler.SourceConfig) (<-chan handler.ChangeEvent, error) {
	if !cfg.IsWatchEnabled() {
		return nil, nil
	}

	repoPath, err := h.resolveRepoPath(ctx, cfg)
	if err != nil {
		return nil, err
	}

	ch := make(chan handler.ChangeEvent, 4)
	go h.pollLoop(ctx, repoPath, ch)
	return ch, nil
}

// pollLoop polls the repo for HEAD changes and sends ChangeEvents.
func (h *Handler) pollLoop(ctx context.Context, repoPath string, ch chan<- handler.ChangeEvent) {
	defer close(ch)

	ticker := time.NewTicker(h.cfg.PollInterval)
	defer ticker.Stop()

	logger := h.logger()

	// Record initial HEAD so we only fire on changes.
	lastSHA, initErr := headCommitSHA(ctx, repoPath)
	if initErr != nil {
		h.watchErrors.Add(1)
		logger.Warn("git watch: failed to read initial HEAD",
			"repo", repoPath, "error", initErr)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentSHA, err := headCommitSHA(ctx, repoPath)
			if err != nil {
				h.watchErrors.Add(1)
				logger.Warn("git watch: failed to read HEAD on tick",
					"repo", repoPath, "error", err)
				continue
			}
			if currentSHA == lastSHA {
				continue
			}
			lastSHA = currentSHA

			// Re-ingest the repo to get updated entities.
			// Populate both Entities (for backward compat) and EntityStates
			// (for the normalizer-free processor path).
			lc := &localCfg{path: repoPath}
			entities, err := h.Ingest(ctx, lc)
			if err != nil {
				h.watchErrors.Add(1)
				logger.Warn("git watch: re-ingest after HEAD change failed",
					"repo", repoPath, "new_sha", currentSHA, "error", err)
				continue
			}
			var entityStates []*handler.EntityState
			if h.cfg.Org != "" {
				entityStates, _ = h.IngestEntityStates(ctx, lc, h.cfg.Org)
			}

			select {
			case ch <- handler.ChangeEvent{
				Path:         repoPath,
				Operation:    handler.OperationModify,
				Timestamp:    time.Now(),
				Entities:     entities,
				EntityStates: entityStates,
			}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// localCfg is a minimal SourceConfig used internally by pollLoop.
type localCfg struct{ path string }

func (c *localCfg) GetType() string             { return handler.SourceTypeGit }
func (c *localCfg) GetPath() string             { return c.path }
func (c *localCfg) GetPaths() []string          { return nil }
func (c *localCfg) GetURL() string              { return "" }
func (c *localCfg) GetBranch() string           { return "" }
func (c *localCfg) IsWatchEnabled() bool        { return false }
func (c *localCfg) GetKeyframeMode() string     { return "" }
func (c *localCfg) GetKeyframeInterval() string { return "" }
func (c *localCfg) GetSceneThreshold() float64  { return 0 }

// resolveRepoPath returns the local filesystem path to the repository.
// When a local path is configured, it is returned directly.
// When only a URL is configured, EnsureRepo clones or pulls the repository
// into WorkspaceDir and returns the resulting path.
func (h *Handler) resolveRepoPath(ctx context.Context, cfg handler.SourceConfig) (string, error) {
	if p := cfg.GetPath(); p != "" {
		return p, nil
	}
	repoURL := cfg.GetURL()
	if repoURL == "" {
		return "", fmt.Errorf("git handler: either path or url is required")
	}
	if h.cfg.WorkspaceDir == "" {
		return "", fmt.Errorf("git handler: workspace_dir required for remote repos (no local path set)")
	}
	opts := workspace.Options{Token: h.cfg.Token}
	return workspace.EnsureRepo(ctx, repoURL, cfg.GetBranch(), h.cfg.WorkspaceDir, opts)
}

// systemSlug returns the system slug for entity ID construction.
// When a URL is configured, the slug is derived from the URL so that
// entity IDs remain stable across machines and clone locations.
// Falls back to the local path when no URL is available.
func (h *Handler) systemSlug(cfg handler.SourceConfig) string {
	var slug string
	if u := cfg.GetURL(); u != "" {
		slug = workspace.URLToSlug(u)
	} else {
		slug = repoSlug(cfg.GetPath())
	}
	return entityid.BranchScopedSlug(slug, h.cfg.BranchSlug)
}

// --------------------------------------------------------------------------
// Git primitives
// --------------------------------------------------------------------------

// ValidateGitURL validates that the URL uses an allowed protocol.
// Exported so tests can call it directly.
func ValidateGitURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("git URL is required")
	}
	// SSH shorthand: git@github.com:owner/repo.git
	if strings.HasPrefix(rawURL, "git@") {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if !allowedProtocols[scheme] {
		return fmt.Errorf("protocol %q not allowed (use https, git, or ssh)", scheme)
	}
	return nil
}

// headCommitSHA returns the full SHA of HEAD.
func headCommitSHA(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// currentBranch returns the current branch name.
func currentBranch(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --abbrev-ref HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// commitRecord holds raw data parsed from git log output.
type commitRecord struct {
	sha         string
	authorName  string
	authorEmail string
	authorFull  string
	subject     string
	files       []string
}

// listCommits walks the repo log and returns commit records.
// limit == 0 means return all commits.
func listCommits(ctx context.Context, repoPath string, limit int) ([]commitRecord, error) {
	// Use a custom format: SHA|AuthorName|AuthorEmail|Subject
	// Followed by a blank line, then the --name-only file list, then a separator.
	args := []string{
		"log",
		"--format=COMMIT:%H|%aN|%aE|%s",
		"--name-only",
		"--no-merges",
	}
	if limit > 0 {
		args = append(args, fmt.Sprintf("-n%d", limit))
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	return parseGitLog(string(out)), nil
}

// parseGitLog parses the output of our custom git log format.
func parseGitLog(output string) []commitRecord {
	var records []commitRecord
	var current *commitRecord

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "COMMIT:") {
			if current != nil {
				records = append(records, *current)
			}
			parts := strings.SplitN(strings.TrimPrefix(line, "COMMIT:"), "|", 4)
			if len(parts) != 4 {
				continue
			}
			current = &commitRecord{
				sha:         parts[0],
				authorName:  parts[1],
				authorEmail: parts[2],
				authorFull:  parts[1] + " <" + parts[2] + ">",
				subject:     parts[3],
			}
			continue
		}
		// Non-empty lines after a COMMIT header are file names.
		if current != nil && strings.TrimSpace(line) != "" {
			current.files = append(current.files, strings.TrimSpace(line))
		}
	}
	if current != nil {
		records = append(records, *current)
	}
	return records
}

// repoSlug converts a local repo path into a NATS-safe system slug.
// Uses the last path component (repo directory name) with slashes replaced.
func repoSlug(repoPath string) string {
	// Use the directory base name as the system slug.
	parts := strings.Split(strings.TrimRight(repoPath, "/"), "/")
	name := parts[len(parts)-1]
	if name == "" {
		return "unknown"
	}
	// Replace path-unsafe characters.
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, ".", "-")
	return name
}

// shortSHA returns the first 7 characters of a SHA.
func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}
