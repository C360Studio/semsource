package source

import "testing"

// TestSectionAnchor pins the locator fragment the docs lens uses to deep-link a
// citation to its section. It is derived from the heading rather than stored, so
// this is the single place the convention is defined.
func TestSectionAnchor(t *testing.T) {
	cases := map[string]string{
		"Build & Test Commands": "build--test-commands",
		"Simple":                "simple",
		"With-Dash":             "with-dash",
		"With_Underscore":       "with-underscore",
		"Trailing punctuation!": "trailing-punctuation",
		"  Padded  ":            "padded",
		"UPPER Case":            "upper-case",
		"日本語 見出し":               "日本語-見出し",
		"":                      "",
		"!!!":                   "",
	}
	for heading, want := range cases {
		if got := SectionAnchor(heading); got != want {
			t.Errorf("SectionAnchor(%q) = %q, want %q", heading, got, want)
		}
	}
}

// TestSectionAnchor_DistinctHeadingsDoNotCollide guards the non-ASCII branch:
// dropping non-ASCII letters would collapse genuinely different sections onto
// one anchor, so two distinct headings must not produce the same fragment.
func TestSectionAnchor_DistinctHeadingsDoNotCollide(t *testing.T) {
	a, b := SectionAnchor("概要"), SectionAnchor("詳細")
	if a == b {
		t.Errorf("distinct non-ASCII headings collapsed onto the same anchor %q", a)
	}
	if a == "" || b == "" {
		t.Errorf("non-ASCII heading produced an empty anchor: %q / %q", a, b)
	}
}
