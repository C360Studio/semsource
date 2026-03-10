package cli

import (
	"os"

	"github.com/c360studio/semsource/config"
)

func init() {
	RegisterSourceWizard(&docWizard{})
}

type docWizard struct{}

func (w *docWizard) Name() string              { return "Documentation" }
func (w *docWizard) TypeKey() string           { return "docs" }
func (w *docWizard) Description() string       { return "markdown, text files" }
func (w *docWizard) Available() (bool, string) { return true, "" }

func (w *docWizard) Prompts(term *Term) (*config.SourceEntry, error) {
	term.Header("Documentation source")
	term.Info("Enter paths to documentation files or directories (e.g. docs/, README.md).")

	// Show detected paths as hints.
	defaults := detectDocPaths()
	if len(defaults) > 0 {
		term.Info("  (detected: " + joinPaths(defaults) + ")")
		term.Info("  Press Enter on empty line to accept detected paths, or enter your own.")
	}

	paths := term.MultiLine("Paths")
	if len(paths) == 0 && len(defaults) > 0 {
		paths = defaults
	}
	for len(paths) == 0 {
		term.Info("  At least one path is required.")
		paths = term.MultiLine("Paths")
	}
	watch := term.Confirm("Watch for changes?", true)

	return &config.SourceEntry{
		Type:  "docs",
		Paths: paths,
		Watch: watch,
	}, nil
}

func detectDocPaths() []string {
	var paths []string
	if info, err := os.Stat("docs"); err == nil && info.IsDir() {
		paths = append(paths, "docs/")
	}
	if _, err := os.Stat("README.md"); err == nil {
		paths = append(paths, "README.md")
	}
	return paths
}

func joinPaths(paths []string) string {
	s := ""
	for i, p := range paths {
		if i > 0 {
			s += ", "
		}
		s += p
	}
	return s
}
