package entityid_test

import (
	"strings"
	"testing"

	semtypes "github.com/c360studio/semstreams/pkg/types"

	"github.com/c360studio/semsource/entityid"
)

// TestSanitizeSegment_ValidPassthrough pins byte-stability: fragments that are
// already contract-valid must pass through unchanged, or every existing entity
// ID would silently change (breaking supersession correspondence).
func TestSanitizeSegment_ValidPassthrough(t *testing.T) {
	valid := []string{
		"entityid-entityid-go-Build",
		"src-routes-page-svelte",
		"a",
		"A9",
		"foo_bar-Baz",
		"ui--eslintrc-cjs", // consecutive dashes are contract-valid and exist in real IDs
		"trailing-",        // trailing dash is contract-valid
	}
	for _, s := range valid {
		if got := entityid.SanitizeSegment(s); got != s {
			t.Errorf("SanitizeSegment(%q) = %q, want unchanged", s, got)
		}
	}
}

// TestSanitizeSegment_InvalidShapes covers the audit's silently-dropped
// shapes: SvelteKit route files, bracketed/grouped dirs, $-identifiers,
// leading underscores.
func TestSanitizeSegment_InvalidShapes(t *testing.T) {
	cases := []string{
		"+page",
		"[slug]",
		"(group)",
		"@modal",
		"clicks$",
		"_examples-demo-go-Demo",
		"名前",  // non-ASCII
		"---", // sanitizes to empty → hash fallback
		"",    // empty stays deterministic
		"a+b", // interior invalid rune
	}
	for _, s := range cases {
		got := entityid.SanitizeSegment(s)
		if got == "" {
			t.Errorf("SanitizeSegment(%q) = empty", s)
			continue
		}
		probe := entityid.Build("org", "semsource", "golang", "sys", "function", got)
		if err := semtypes.ValidateEntityID(probe); err != nil {
			t.Errorf("SanitizeSegment(%q) = %q, still fails graph-ingest contract: %v", s, got, err)
		}
	}
}

// TestSanitizeSegment_Deterministic pins same input → same output.
func TestSanitizeSegment_Deterministic(t *testing.T) {
	for _, s := range []string{"+page", "clicks$", "[slug]", "normal"} {
		if a, b := entityid.SanitizeSegment(s), entityid.SanitizeSegment(s); a != b {
			t.Errorf("SanitizeSegment(%q) nondeterministic: %q vs %q", s, a, b)
		}
	}
}

// TestSanitizeSegment_Idempotent pins f(f(x)) == f(x): sanitized output is
// itself contract-valid and passes through unchanged.
func TestSanitizeSegment_Idempotent(t *testing.T) {
	for _, s := range []string{"+page", "[slug]", "clicks$", "---", "valid-name"} {
		once := entityid.SanitizeSegment(s)
		if twice := entityid.SanitizeSegment(once); twice != once {
			t.Errorf("SanitizeSegment not idempotent for %q: %q -> %q", s, once, twice)
		}
	}
}

// TestSanitizeSegment_DistinctInputsStayDistinct pins the anti-collision
// property: raw inputs that would map to the same base string must not
// produce identical fragments ("+page" vs "page" was the audit's example).
func TestSanitizeSegment_DistinctInputsStayDistinct(t *testing.T) {
	pairs := [][2]string{
		{"+page", "page"},
		{"a+b", "a-b"},
		{"clicks$", "clicks-"},
		{"[slug]", "slug"},
	}
	for _, p := range pairs {
		a, b := entityid.SanitizeSegment(p[0]), entityid.SanitizeSegment(p[1])
		if a == b {
			t.Errorf("collision: SanitizeSegment(%q) == SanitizeSegment(%q) == %q", p[0], p[1], a)
		}
	}
}

// TestSanitizeSegment_PropertyAgainstSubstrateValidator is the property test
// from the change spec: over a corpus of nasty inputs, output always yields a
// segment the substrate validator accepts, and distinct inputs never collide.
func TestSanitizeSegment_PropertyAgainstSubstrateValidator(t *testing.T) {
	corpus := []string{
		"+page", "+layout", "[slug]", "[...rest]", "(group)", "@modal",
		"clicks$", "$store", "_examples", "__init__", "名前", "café",
		"a b c", "a/b/c", "a.b.c", "-lead", "--", "...", "", " ",
		"page", "+page.svelte", "src/routes/+page.svelte",
		"really" + strings.Repeat("-long", 40) + "name",
	}
	seen := map[string]string{}
	for _, s := range corpus {
		got := entityid.SanitizeSegment(s)
		probe := entityid.Build("org", "semsource", "golang", "sys", "function", got)
		if err := semtypes.ValidateEntityID(probe); err != nil {
			t.Errorf("corpus %q -> %q fails substrate validator: %v", s, got, err)
		}
		if prev, dup := seen[got]; dup && prev != s {
			t.Errorf("corpus collision: %q and %q both -> %q", prev, s, got)
		}
		seen[got] = s
	}
}
