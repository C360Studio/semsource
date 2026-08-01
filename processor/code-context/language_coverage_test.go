package codecontext

import (
	"testing"

	semsourceast "github.com/c360studio/semsource/source/ast"

	// Registering every parser is the point: this test asks the registry what
	// languages exist, so the blank imports here are what make it meaningful.
	_ "github.com/c360studio/semsource/source/ast/c"
	_ "github.com/c360studio/semsource/source/ast/cpp"
	_ "github.com/c360studio/semsource/source/ast/golang"
	_ "github.com/c360studio/semsource/source/ast/java"
	_ "github.com/c360studio/semsource/source/ast/python"
	_ "github.com/c360studio/semsource/source/ast/svelte"
	_ "github.com/c360studio/semsource/source/ast/ts"
)

// languageDomains mirrors handler/ast's mapping. It is duplicated rather than
// imported because that table is unexported, and exporting it to satisfy a test
// would widen a package boundary for no runtime reason. The duplication is safe
// in the direction that matters: this test fails when a language is registered
// and missing here, which is the same signal.
var languageDomains = map[string]string{
	"go":         "golang",
	"ts":         "typescript",
	"typescript": "typescript",
	"javascript": "typescript",
	"java":       "java",
	"python":     "python",
	"svelte":     "svelte",
	"c":          "c",
	"cpp":        "cpp",
}

// TestCodeScopeCoversEveryRegisteredLanguage keeps a parsed language from being
// invisible to code-scoped retrieval.
//
// This is the failure mode that has no symptom: entities are extracted,
// published, and indexed correctly, and then code_context and code_search
// silently skip them because their domain is not in this list. Nothing errors
// and no count looks wrong — the language is simply absent from answers.
func TestCodeScopeCoversEveryRegisteredLanguage(t *testing.T) {
	covered := map[string]bool{}
	for _, d := range codeScopeDomains {
		covered[d] = true
	}

	for _, lang := range semsourceast.DefaultRegistry.ListParsers() {
		domain, ok := languageDomains[lang]
		if !ok {
			t.Errorf("parser %q has no domain in this test's mirror of handler/ast's table; "+
				"add it there and here", lang)
			continue
		}
		if !covered[domain] {
			t.Errorf("language %q produces {domain} = %q, which codeScopeDomains does not list; "+
				"its symbols would be extracted and then invisible to code-scoped retrieval", lang, domain)
		}
	}
}
