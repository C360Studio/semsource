package handler

import "path/filepath"

// defaultExcludedDirs are directory names no source ingests, ever.
//
// These are not first-party source under any configuration, and walking them is
// actively harmful rather than merely wasteful:
//
//   - node_modules: a dependency tree, often with minified and generated files.
//     Ingesting semsource's own ui/node_modules produced 36 entity IDs that
//     violated the graph-ingest segment contract (JavaScript destructuring
//     declarations become enormous instance segments), so ast-source never
//     finished its seed and the service reported phase "degraded" — with the
//     cause buried in WARN lines. cfgfile separately pulled 2,484 entities out
//     of node_modules package.json files.
//   - .git: object storage, not source.
//
// This is a FLOOR, not a default: configured excludes are added to it, never
// substituted for it. A non-empty list silently replacing built-in defaults is a
// footgun this codebase has already been bitten by once — see the text_suffixes
// comment in cmd/semsource/run.go, which has to restate the framework's defaults
// because supplying any value drops them all.
//
// Deliberately NOT included: vendor. Go's vendor directory holds genuine
// dependency source that a deployment may legitimately want indexed, so it stays
// a per-source decision. (Some AST parsers exclude it themselves; that
// inconsistency is noted, not resolved here.)
var defaultExcludedDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
}

// IsDefaultExcludedDir reports whether a directory name is one every source
// skips. Callers pass the base name, not a full path.
//
// It exists because the three walking sources had three different answers to the
// same question: doc-source excluded node_modules in code, ast-source excluded it
// only when a config happened to say so (and the shipped default did not), and
// cfgfile-source had no exclusion mechanism at all. One floor, one place.
func IsDefaultExcludedDir(name string) bool {
	return defaultExcludedDirs[name]
}

// IsDefaultExcludedPath reports whether any path segment is a default-excluded
// directory. Use when a walk yields paths rather than per-directory callbacks and
// cannot prune with filepath.SkipDir.
func IsDefaultExcludedPath(path string) bool {
	for _, seg := range splitPathSegments(path) {
		if defaultExcludedDirs[seg] {
			return true
		}
	}
	return false
}

func splitPathSegments(path string) []string {
	var segs []string
	for {
		dir, file := filepath.Split(path)
		if file != "" {
			segs = append(segs, file)
		}
		if dir == "" || dir == path {
			return segs
		}
		path = filepath.Clean(dir)
		if path == "." || path == string(filepath.Separator) {
			return segs
		}
	}
}

// DefaultExcludedDirNames returns the floor as a slice, for callers that build
// their own exclude set. Order is not significant.
func DefaultExcludedDirNames() []string {
	out := make([]string, 0, len(defaultExcludedDirs))
	for name := range defaultExcludedDirs {
		out = append(out, name)
	}
	return out
}
