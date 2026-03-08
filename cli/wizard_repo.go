package cli

import (
	"github.com/c360studio/semsource/config"
)

func init() {
	RegisterSourceWizard(&repoWizard{})
}

type repoWizard struct{}

func (w *repoWizard) Name() string        { return "Repository (all-in-one)" }
func (w *repoWizard) TypeKey() string     { return "repo" }
func (w *repoWizard) Description() string { return "Clone a git repo and analyze code, docs, and config" }
func (w *repoWizard) Available() (bool, string) { return true, "" }

func (w *repoWizard) Prompts(term *Term) (*config.SourceEntry, error) {
	term.Header("Repository (all-in-one) source")

	var url string
	for url == "" {
		url = term.Prompt("Repository URL", "")
		if url == "" {
			term.Info("  A repository URL is required.")
		}
	}

	branch := term.Prompt("Branch (default: remote default)", "")
	language := term.Prompt("Primary language (go, java, python, typescript, or leave blank to auto-detect)", "")
	watch := term.Confirm("Watch for changes?", true)

	return &config.SourceEntry{
		Type:     "repo",
		URL:      url,
		Branch:   branch,
		Language: language,
		Watch:    watch,
	}, nil
}
