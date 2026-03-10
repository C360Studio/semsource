package cli

import (
	"os"

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

	// Default to "." (current repo) if we're in a git repo.
	defaultURL := ""
	if _, err := os.Stat(".git"); err == nil {
		defaultURL = "."
	}

	var repoURL string
	for repoURL == "" {
		repoURL = term.Prompt("Repository path or URL", defaultURL)
		if repoURL == "" {
			term.Info("  A repository path or URL is required.")
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
