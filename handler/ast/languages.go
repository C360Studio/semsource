package ast

import (
	"fmt"
	"sort"

	"github.com/c360studio/semsource/handler"
)

// Language wiring lives here as data rather than as switch statements so that a
// language cannot be half-added.
//
// Both lookups previously ended in `default:` returning Go — the domain constant
// in one, ".go" in the other. That made a partly-wired language silent instead of
// fatal: a parser registered under a new name, with these tables not updated,
// would walk .go files (finding none, so producing nothing) or, worse, publish
// its symbols under {domain} = "golang". Domain is part of the entity ID, so
// that is not a display bug — it mints permanently mislabelled identities, and
// no existing test would notice because nothing fails.
//
// Adding a language therefore means adding it here, and TestEveryRegisteredParser…
// in languages_test.go fails until that is done.

// languageDomains maps a configured language name to the {domain} segment its
// entities carry. Several names may share a domain: "ts", "typescript", and
// "javascript" all produce "typescript" entities, which is existing behaviour.
var languageDomains = map[string]string{
	"go":         handler.DomainGolang,
	"ts":         "typescript",
	"typescript": "typescript",
	"javascript": "typescript",
	"java":       "java",
	"python":     "python",
	"svelte":     "svelte",
}

// languageExtensions maps a configured language name to the file extensions the
// watcher should follow for it.
var languageExtensions = map[string][]string{
	"go":         {".go"},
	"ts":         {".ts", ".tsx", ".js", ".jsx"},
	"typescript": {".ts", ".tsx", ".js", ".jsx"},
	"javascript": {".ts", ".tsx", ".js", ".jsx"},
	"java":       {".java"},
	"python":     {".py"},
	"svelte":     {".svelte"},
}

// langToDomain returns the {domain} segment for a language, and whether the
// language is known. An unknown language yields no domain rather than defaulting
// to Go — see the note above.
func langToDomain(lang string) (string, bool) {
	d, ok := languageDomains[lang]
	return d, ok
}

// langToExtensions returns the file extensions for a language, and whether the
// language is known.
func langToExtensions(lang string) ([]string, bool) {
	e, ok := languageExtensions[lang]
	return e, ok
}

// validateLanguage rejects a language this package cannot map, naming what it
// does support. Called at the handler's entry points so an unmappable language
// stops ingestion instead of producing mislabelled entities partway through.
func validateLanguage(lang string) error {
	_, hasDomain := languageDomains[lang]
	_, hasExts := languageExtensions[lang]
	if hasDomain && hasExts {
		return nil
	}
	return fmt.Errorf("asthandler: unsupported language %q (supported: %v)", lang, knownLanguages())
}

// knownLanguages lists the languages this package can map, sorted for a stable
// error message.
func knownLanguages() []string {
	out := make([]string, 0, len(languageDomains))
	for l := range languageDomains {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}
