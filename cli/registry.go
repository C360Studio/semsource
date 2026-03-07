package cli

import "github.com/c360studio/semsource/config"

// SourceWizard defines interactive prompts for one source type.
type SourceWizard interface {
	// Name is the human-readable display name shown in the source menu.
	Name() string

	// TypeKey is the config type identifier (e.g., "ast", "git").
	TypeKey() string

	// Description is a one-line summary shown next to the name.
	Description() string

	// Available reports whether this source type is ready to use.
	// Returns (true, "") when available, or (false, "reason") when not.
	Available() (bool, string)

	// Prompts runs the interactive wizard and returns a populated SourceEntry.
	Prompts(term *Term) (*config.SourceEntry, error)
}

var registry []SourceWizard

// RegisterSourceWizard adds a wizard to the global registry.
// Call this from init() functions in wizard_*.go files.
func RegisterSourceWizard(w SourceWizard) {
	registry = append(registry, w)
}

// Wizards returns all registered source wizards.
func Wizards() []SourceWizard {
	return registry
}
