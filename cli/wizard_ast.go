package cli

import (
	"os"

	"github.com/c360studio/semsource/config"
)

func init() {
	RegisterSourceWizard(&astWizard{})
}

type astWizard struct{}

func (w *astWizard) Name() string              { return "Code (AST)" }
func (w *astWizard) TypeKey() string           { return "ast" }
func (w *astWizard) Description() string       { return "functions, types, imports" }
func (w *astWizard) Available() (bool, string) { return true, "" }

func (w *astWizard) Prompts(term *Term) (*config.SourceEntry, error) {
	term.Header("Code / AST source")

	path := term.Prompt("Root path to scan", ".")

	// Auto-detect language from project files.
	detectedLang := detectLanguage()

	langOptions := []string{"auto-detect", "go", "typescript", "python", "java", "svelte"}
	defaultIdx := 0
	if detectedLang != "" {
		for i, opt := range langOptions {
			if opt == detectedLang {
				defaultIdx = i
				break
			}
		}
	}

	if defaultIdx > 0 {
		term.Info("  (detected: " + langOptions[defaultIdx] + ")")
	}
	idx := term.Select("Language", langOptions)
	language := ""
	if idx > 0 {
		language = langOptions[idx]
	} else if defaultIdx > 0 {
		// auto-detect selected but we know the language — use it
		language = langOptions[defaultIdx]
	}

	watch := term.Confirm("Watch for changes?", true)

	return &config.SourceEntry{
		Type:     "ast",
		Path:     path,
		Language: language,
		Watch:    watch,
	}, nil
}

// detectLanguage checks for common manifest files in the current directory.
func detectLanguage() string {
	manifests := map[string]string{
		"go.mod":       "go",
		"package.json": "typescript",
		"Cargo.toml":   "rust",
		"pom.xml":      "java",
		"build.gradle": "java",
	}
	for file, lang := range manifests {
		if _, err := os.Stat(file); err == nil {
			return lang
		}
	}
	return ""
}
