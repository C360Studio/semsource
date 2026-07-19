package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/c360studio/semsource/config"
)

func init() {
	RegisterSourceWizard(&gitWizard{})
}

type gitWizard struct{}

func (w *gitWizard) Name() string              { return "Git history" }
func (w *gitWizard) TypeKey() string           { return "git" }
func (w *gitWizard) Description() string       { return "commits, authors, branches" }
func (w *gitWizard) Available() (bool, string) { return true, "" }

func (w *gitWizard) Prompts(term *Term) (*config.SourceEntry, error) {
	term.Header("Git history source")

	// Default to a RESOLVABLE identity for the current repo: the origin
	// remote, else the absolute path — never "." (empty system segment; the
	// audit's zero-git-history default).
	defaultURL := ""
	if _, err := os.Stat(".git"); err == nil {
		defaultURL = resolvableGitURL(detectGitRemote("."))
	}

	var repoURL string
	for repoURL == "" {
		repoURL = term.Prompt("Repository path or URL", defaultURL)
		if repoURL == "" {
			term.Info("  A repository path or URL is required.")
		}
	}
	// Normalize relative paths (including ".") to absolute so the source
	// identity does not depend on the daemon's working directory.
	if !strings.Contains(repoURL, "://") && !strings.Contains(repoURL, "@") {
		if abs, err := filepath.Abs(repoURL); err == nil {
			repoURL = abs
		}
	}
	branch := term.Prompt("Branch to track", "main")
	watch := term.Confirm("Watch for new commits?", true)

	return &config.SourceEntry{
		Type:   "git",
		URL:    repoURL,
		Branch: branch,
		Watch:  watch,
	}, nil
}
