package cli

import "github.com/c360studio/semsource/config"

func init() {
	RegisterSourceWizard(&astWizard{})
}

type astWizard struct{}

func (w *astWizard) Name() string        { return "Code (AST)" }
func (w *astWizard) TypeKey() string     { return "ast" }
func (w *astWizard) Description() string { return "functions, types, imports" }
func (w *astWizard) Available() (bool, string) { return true, "" }

func (w *astWizard) Prompts(term *Term) (*config.SourceEntry, error) {
	term.Header("Code / AST source")

	path := term.Prompt("Root path to scan", ".")

	langOptions := []string{"auto-detect", "go", "typescript", "python", "java", "svelte"}
	term.Info("Language:")
	idx := term.Select("Language", langOptions)
	language := ""
	if idx > 0 {
		language = langOptions[idx]
	}

	watch := term.Confirm("Watch for changes?", true)

	return &config.SourceEntry{
		Type:     "ast",
		Path:     path,
		Language: language,
		Watch:    watch,
	}, nil
}
