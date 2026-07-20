package source

import "strings"

// SectionAnchor builds a GitHub-style section anchor from a heading.
//
// It lives here, next to the DocSection predicate it derives from, because both
// the producer (which splits documents on headings) and the fusion docs lens
// (which cites a passage by deep-linking to its section) need the same answer.
// Deriving it in one place is the point: the anchor is a pure function of the
// heading, so storing it as a second fact alongside DocSection would be one more
// thing that can disagree with itself.
func SectionAnchor(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		case r > 127:
			// Keep non-ASCII letters — headings are not always English, and
			// dropping them would collapse distinct sections onto one anchor.
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-")
}
