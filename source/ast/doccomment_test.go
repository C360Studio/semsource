package ast

import "testing"

func TestCleanDocCommentBlock(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single line javadoc",
			in:   "/** Adds two numbers. */",
			want: "Adds two numbers.",
		},
		{
			name: "multiline javadoc with leading stars",
			in: `/**
 * Computes the sum of a and b.
 *
 * @param a first addend
 * @param b second addend
 * @return the sum
 */`,
			want: "Computes the sum of a and b.\n\n@param a first addend\n@param b second addend\n@return the sum",
		},
		{
			name: "tsdoc with embedded code",
			in: `/**
 * Returns ` + "`true`" + ` when the value is non-empty.
 *
 * @example
 *   isNonEmpty("x") // true
 */`,
			want: "Returns `true` when the value is non-empty.\n\n@example\n  isNonEmpty(\"x\") // true",
		},
		{
			name: "no leading stars",
			in:   "/**\n  Inline body.\n  Second line.\n*/",
			want: "Inline body.\nSecond line.",
		},
		{
			name: "empty body",
			in:   "/** */",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanDocCommentBlock(tt.in)
			if got != tt.want {
				t.Errorf("CleanDocCommentBlock = %q\n want %q", got, tt.want)
			}
		})
	}
}

func TestCombineDocComment(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		metadata string
		want     string
	}{
		{"both", "Doc body.", "static final", "Doc body.\n\nstatic final"},
		{"only body", "Doc body.", "", "Doc body."},
		{"only metadata", "", "@Override", "@Override"},
		{"both empty", "", "", ""},
		{"trims whitespace", "  body  ", "  meta  ", "body\n\nmeta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CombineDocComment(tt.body, tt.metadata)
			if got != tt.want {
				t.Errorf("CombineDocComment = %q, want %q", got, tt.want)
			}
		})
	}
}
