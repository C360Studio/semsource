package astsource

import (
	"strings"
	"testing"

	semsourceast "github.com/c360studio/semsource/source/ast"
)

// fakeExtensions builds an extensionsFor function from a literal table, so the
// routing rule can be exercised on language pairs the global registry does not
// yet contain.
func fakeExtensions(table map[string][]string) func(string) []string {
	return func(lang string) []string { return table[lang] }
}

// TestRouteExtensions_ContestedExtensionIsStableAcrossRepetitions is the reason
// this routing table exists. The previous implementation chose a parser by
// ranging a map, so with two languages claiming one extension the winner was
// drawn from Go's randomized map order — a single run agreed with itself about
// half the time. Repetition is therefore load-bearing here: one call proves
// nothing.
func TestRouteExtensions_ContestedExtensionIsStableAcrossRepetitions(t *testing.T) {
	exts := fakeExtensions(map[string][]string{
		"c":   {".c", ".h"},
		"cpp": {".cpp", ".cc", ".h"},
	})

	first := routeExtensions([]string{"c", "cpp"}, exts)[".h"]
	if first == "" {
		t.Fatal(`".h" was not routed at all`)
	}
	for i := 0; i < 500; i++ {
		if got := routeExtensions([]string{"c", "cpp"}, exts)[".h"]; got != first {
			t.Fatalf("iteration %d routed .h to %q, first call routed it to %q — "+
				"routing is not deterministic", i, got, first)
		}
	}
}

// TestRouteExtensions_IgnoresDeclarationOrder pins that the routing table is a
// function of the language SET. Two watch paths listing the same languages in a
// different order must produce the same entity IDs, so configuration order must
// not reach the outcome.
func TestRouteExtensions_IgnoresDeclarationOrder(t *testing.T) {
	exts := fakeExtensions(map[string][]string{
		"c":   {".c", ".h"},
		"cpp": {".cpp", ".h"},
	})

	forward := routeExtensions([]string{"c", "cpp"}, exts)
	reverse := routeExtensions([]string{"cpp", "c"}, exts)

	if len(forward) != len(reverse) {
		t.Fatalf("different table sizes: %v vs %v", forward, reverse)
	}
	for ext, lang := range forward {
		if reverse[ext] != lang {
			t.Errorf("%s routed to %q in one order and %q in the other", ext, lang, reverse[ext])
		}
	}
}

// TestRouteExtensions_DeclaredSetDecidesTheHeader covers the rule the ".h"
// precedence exists for: the same extension belongs to a different language
// depending on what the watch path declared, and nothing about the file itself
// is consulted.
func TestRouteExtensions_DeclaredSetDecidesTheHeader(t *testing.T) {
	exts := fakeExtensions(map[string][]string{
		"c":   {".c", ".h"},
		"cpp": {".cpp", ".h"},
	})

	for name, tc := range map[string]struct {
		languages []string
		want      string
	}{
		"C++ declared alongside C claims the header": {[]string{"c", "cpp"}, "cpp"},
		"C++ alone claims the header":                {[]string{"cpp"}, "cpp"},
		"C alone claims the header":                  {[]string{"c"}, "c"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := routeExtensions(tc.languages, exts)[".h"]; got != tc.want {
				t.Errorf("languages %v routed .h to %q, want %q", tc.languages, got, tc.want)
			}
		})
	}
}

// TestRouteExtensions_UndeclaredExtensionIsNotRouted keeps a watch path from
// parsing files it never asked for.
func TestRouteExtensions_UndeclaredExtensionIsNotRouted(t *testing.T) {
	exts := fakeExtensions(map[string][]string{"c": {".c", ".h"}})

	routes := routeExtensions([]string{"c"}, exts)
	if lang, ok := routes[".cpp"]; ok {
		t.Errorf(".cpp routed to %q although only C was declared", lang)
	}
}

// TestContestedExtensions_DetectsOverlap proves the detector the guard below
// depends on. It is separate because the guard runs against the real registry,
// where nothing overlaps yet — so the guard alone would pass whether or not
// contestedExtensions worked at all.
func TestContestedExtensions_DetectsOverlap(t *testing.T) {
	exts := fakeExtensions(map[string][]string{
		"c":    {".c", ".h"},
		"cpp":  {".cpp", ".h"},
		"java": {".java"},
	})

	contested := contestedExtensions([]string{"c", "cpp", "java"}, exts)

	if got, ok := contested[".h"]; !ok || len(got) != 2 {
		t.Errorf(`.h should be reported as contested by two languages, got %v`, got)
	}
	if _, ok := contested[".java"]; ok {
		t.Error(".java is claimed by one language and must not be reported as contested")
	}
	if _, ok := contested[".c"]; ok {
		t.Error(".c is claimed by one language and must not be reported as contested")
	}
}

// TestEveryContestedExtensionHasAnExplicitRule is the durable invariant behind
// this change. Today no two registered languages share an extension — verified:
// the registry's extension sets are disjoint — so the old map-ranging code
// happened to be safe. That safety was never stated anywhere and nothing
// enforced it. This fails the moment a new registration overlaps an existing one
// without a deliberate precedence rule, rather than letting the arbitrary
// sorted-order fallback silently decide which language a file becomes.
//
// It therefore passes vacuously right now and only starts doing work when C and
// C++ land and `.h` becomes contested. TestContestedExtensions_DetectsOverlap
// covers the detector itself so this is not a test that merely looks green.
func TestEveryContestedExtensionHasAnExplicitRule(t *testing.T) {
	languages := semsourceast.DefaultRegistry.ListParsers()
	contested := contestedExtensions(languages, semsourceast.DefaultRegistry.GetExtensionsForParser)

	for ext, claimants := range contested {
		if len(extensionPrecedence[ext]) == 0 {
			t.Errorf("extension %s is claimed by %v but has no entry in extensionPrecedence; "+
				"add one so the winner is a decision rather than sorted-order accident", ext, claimants)
			continue
		}
		// A rule that does not name a claimant would fall through to the same
		// arbitrary fallback it exists to avoid.
		named := map[string]bool{}
		for _, l := range extensionPrecedence[ext] {
			named[l] = true
		}
		for _, c := range claimants {
			if !named[c] {
				t.Errorf("extension %s is claimed by %q, which extensionPrecedence[%s] does not rank",
					ext, c, ext)
			}
		}
	}
}

// TestWatchPathRejectsUnregisteredLanguage confirms the configuration-time guard
// that task 2.4 called for already exists and works, rather than adding a second
// one beside it. A declared language with no parser must fail here, not produce a
// watch path that walks files and extracts nothing.
func TestWatchPathRejectsUnregisteredLanguage(t *testing.T) {
	wp := &WatchPathConfig{
		Path:      "/tmp/x",
		Org:       "acme",
		Project:   "proj",
		Languages: []string{"go", "rust"},
	}
	err := wp.Validate()
	if err == nil {
		t.Fatal("a watch path declaring an unregistered language must not validate")
	}
	if !strings.Contains(err.Error(), "rust") {
		t.Errorf("error must name the offending language, got %v", err)
	}
}
