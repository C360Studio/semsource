package cli

import (
	"os"

	"github.com/c360studio/semsource/config"
)

func init() {
	RegisterSourceWizard(&cfgWizard{})
}

type cfgWizard struct{}

func (w *cfgWizard) Name() string              { return "Config files" }
func (w *cfgWizard) TypeKey() string           { return "config" }
func (w *cfgWizard) Description() string       { return "go.mod, package.json, Dockerfile" }
func (w *cfgWizard) Available() (bool, string) { return true, "" }

func (w *cfgWizard) Prompts(term *Term) (*config.SourceEntry, error) {
	term.Header("Config files source")
	term.Info("Enter paths to config files (e.g. go.mod, package.json, Dockerfile).")

	// Show detected config files as hints.
	defaults := detectConfigFiles()
	if len(defaults) > 0 {
		term.Info("  (detected: " + joinPaths(defaults) + ")")
		term.Info("  Press Enter on empty line to accept detected files, or enter your own.")
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
		Type:  "config",
		Paths: paths,
		Watch: watch,
	}, nil
}

func detectConfigFiles() []string {
	candidates := []string{
		"go.mod", "go.sum", "package.json", "package-lock.json",
		"Dockerfile", "docker-compose.yml", "docker-compose.yaml",
		"Makefile", "Cargo.toml", "pom.xml", "build.gradle",
	}
	var found []string
	for _, f := range candidates {
		if _, err := os.Stat(f); err == nil {
			found = append(found, f)
		}
	}
	return found
}
