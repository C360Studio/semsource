package source

import "strings"

// docExtensions is the canonical set of file extensions SemSource ingests as
// documents.
//
// It lives here because two places need the same answer and had drifted apart:
// the doc handler decides what enters the corpus, and the fusion docs lens
// decides whether a single-token query looks like a document path. They
// disagreed — the lens routed `.rst` that the handler never ingests (so such a
// query could only ever miss) and omitted `.mdx` that it does (so a real
// document path resolved semantically instead of by prefix).
var docExtensions = map[string]bool{
	".adoc": true,
	".md":   true,
	".mdx":  true,
	".txt":  true,
}

// IsDocExtension reports whether ext (with its leading dot, any case) is a
// document extension SemSource ingests.
func IsDocExtension(ext string) bool {
	return docExtensions[strings.ToLower(ext)]
}

// DocExtensions returns the ingested document extensions, for callers that need
// to enumerate rather than test membership.
func DocExtensions() []string {
	out := make([]string, 0, len(docExtensions))
	for ext := range docExtensions {
		out = append(out, ext)
	}
	return out
}
