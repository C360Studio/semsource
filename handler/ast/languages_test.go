package ast

import (
	"context"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler"
	semsourceast "github.com/c360studio/semsource/source/ast"
)

// TestEveryRegisteredParserIsMapped is the guard that makes a half-wired
// language impossible. It is driven off the registry rather than a hand-written
// list precisely so that registering a parser is enough to make it fail — the
// author of the next language does not have to remember this file exists.
//
// Without it, the previous `default:` arms turned "language not wired here" into
// "language silently becomes Go": entities published under {domain} = "golang",
// which is part of the entity ID, so the mislabelling is permanent and invisible
// to every other test.
func TestEveryRegisteredParserIsMapped(t *testing.T) {
	for _, lang := range semsourceast.DefaultRegistry.ListParsers() {
		t.Run(lang, func(t *testing.T) {
			domain, ok := langToDomain(lang)
			if !ok {
				t.Errorf("parser %q is registered but has no domain mapping; add it to "+
					"languageDomains — otherwise its entities would have carried {domain} = golang", lang)
			}
			if ok && domain == "" {
				t.Errorf("parser %q maps to an empty domain", lang)
			}
			exts, ok := langToExtensions(lang)
			if !ok {
				t.Errorf("parser %q is registered but has no extension mapping; add it to "+
					"languageExtensions — otherwise its watcher would have followed .go files", lang)
			}
			if ok && len(exts) == 0 {
				t.Errorf("parser %q maps to no extensions", lang)
			}
		})
	}
}

// TestUnknownLanguageIsNotSilentlyGo pins the specific regression. "rust" has no
// parser and no mapping; the old code answered "golang" and []string{".go"} for
// it without complaint.
func TestUnknownLanguageIsNotSilentlyGo(t *testing.T) {
	if domain, ok := langToDomain("rust"); ok || domain == handler.DomainGolang {
		t.Errorf("unknown language mapped to domain %q (ok=%v); it must not resolve to Go", domain, ok)
	}
	if exts, ok := langToExtensions("rust"); ok || len(exts) > 0 {
		t.Errorf("unknown language mapped to extensions %v (ok=%v); it must not resolve to .go", exts, ok)
	}
	if err := validateLanguage("rust"); err == nil {
		t.Fatal("validateLanguage accepted an unsupported language")
	} else if !strings.Contains(err.Error(), "rust") {
		t.Errorf("error must name the offending language, got %v", err)
	}
}

// TestKnownLanguagesKeepTheirDomains guards the behaviour this refactor had to
// preserve exactly. These mappings are baked into existing entity IDs, so a
// change here would silently re-identify an already-published graph.
func TestKnownLanguagesKeepTheirDomains(t *testing.T) {
	for lang, want := range map[string]string{
		"go":         handler.DomainGolang,
		"ts":         "typescript",
		"typescript": "typescript",
		"javascript": "typescript",
		"java":       "java",
		"python":     "python",
		"svelte":     "svelte",
	} {
		if got, ok := langToDomain(lang); !ok || got != want {
			t.Errorf("langToDomain(%q) = %q (ok=%v), want %q", lang, got, ok, want)
		}
	}
}

// langConfig is the minimum SourceConfig the handler needs to reach language
// validation. (handler_test.go has a fuller stub, but it lives in the external
// ast_test package and these tests need the unexported lookups.)
type langConfig struct{ lang, path string }

func (s langConfig) GetType() string             { return handler.SourceTypeAST }
func (s langConfig) GetPath() string             { return s.path }
func (s langConfig) GetPaths() []string          { return nil }
func (s langConfig) GetURL() string              { return "" }
func (s langConfig) GetBranch() string           { return "" }
func (s langConfig) IsWatchEnabled() bool        { return true }
func (s langConfig) GetKeyframeMode() string     { return "" }
func (s langConfig) GetKeyframeInterval() string { return "" }
func (s langConfig) GetSceneThreshold() float64  { return 0 }
func (s langConfig) GetLanguage() string         { return s.lang }
func (s langConfig) GetOrg() string              { return "acme" }
func (s langConfig) GetProject() string          { return "proj" }

// TestIngestRejectsUnsupportedLanguage checks the failure is loud at the entry
// point rather than surfacing later as mislabelled entities.
func TestIngestRejectsUnsupportedLanguage(t *testing.T) {
	h := New(nil)
	if _, err := h.Ingest(context.Background(), langConfig{lang: "rust", path: t.TempDir()}); err == nil {
		t.Fatal("Ingest accepted an unsupported language")
	} else if !strings.Contains(err.Error(), "rust") {
		t.Errorf("error must name the language, got %v", err)
	}
}

// TestWatchRejectsUnsupportedLanguage covers the same for the watch path, which
// resolves extensions separately and so could have failed differently.
func TestWatchRejectsUnsupportedLanguage(t *testing.T) {
	h := New(nil)
	if _, err := h.Watch(context.Background(), langConfig{lang: "rust", path: t.TempDir()}); err == nil {
		t.Fatal("Watch accepted an unsupported language")
	} else if !strings.Contains(err.Error(), "rust") {
		t.Errorf("error must name the language, got %v", err)
	}
}
