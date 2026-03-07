package cli

import "github.com/c360studio/semsource/config"

func init() {
	RegisterSourceWizard(&docWizard{})
}

type docWizard struct{}

func (w *docWizard) Name() string        { return "Documentation" }
func (w *docWizard) TypeKey() string     { return "docs" }
func (w *docWizard) Description() string { return "markdown, text files" }
func (w *docWizard) Available() (bool, string) { return true, "" }

func (w *docWizard) Prompts(term *Term) (*config.SourceEntry, error) {
	term.Header("Documentation source")
	term.Info("Enter paths to documentation files or directories (e.g. docs/, README.md).")

	var paths []string
	for len(paths) == 0 {
		paths = term.MultiLine("Paths")
		if len(paths) == 0 {
			term.Info("  At least one path is required.")
		}
	}
	watch := term.Confirm("Watch for changes?", true)

	return &config.SourceEntry{
		Type:  "docs",
		Paths: paths,
		Watch: watch,
	}, nil
}
