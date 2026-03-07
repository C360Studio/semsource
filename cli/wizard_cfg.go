package cli

import "github.com/c360studio/semsource/config"

func init() {
	RegisterSourceWizard(&cfgWizard{})
}

type cfgWizard struct{}

func (w *cfgWizard) Name() string        { return "Config files" }
func (w *cfgWizard) TypeKey() string     { return "config" }
func (w *cfgWizard) Description() string { return "go.mod, package.json, Dockerfile" }
func (w *cfgWizard) Available() (bool, string) { return true, "" }

func (w *cfgWizard) Prompts(term *Term) (*config.SourceEntry, error) {
	term.Header("Config files source")
	term.Info("Enter paths to config files (e.g. go.mod, package.json, Dockerfile).")

	var paths []string
	for len(paths) == 0 {
		paths = term.MultiLine("Paths")
		if len(paths) == 0 {
			term.Info("  At least one path is required.")
		}
	}
	watch := term.Confirm("Watch for changes?", true)

	return &config.SourceEntry{
		Type:  "config",
		Paths: paths,
		Watch: watch,
	}, nil
}
