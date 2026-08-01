package astsource

import (
	"sort"

	semsourceast "github.com/c360studio/semsource/source/ast"
)

// extensionPrecedence resolves file extensions that more than one language
// claims, listing the claimants in winning order.
//
// `.h` is the case this exists for. It is ambiguous across repositories rather
// than within one: a C++ firmware tree's headers declare classes, namespaces and
// templates, while a C project's headers — MAVLink's generated ones, for
// instance — are plain C. A watch path that declares C++ therefore reads headers
// as C++, and one that declares only C reads them as C.
//
// The alternative — deciding per file by looking for `class` or `namespace` in
// the bytes — is deliberately not taken. A header that merely uses a C++ type
// without declaring one would sniff as C, and the language a file is read as
// determines the {domain} segment of every entity ID it produces, so a wrong
// guess is a permanent identity error rather than a recoverable mistake. The
// declared language set already carries the operator's intent; this table only
// says which declaration wins when two of them overlap.
var extensionPrecedence = map[string][]string{
	".h": {"cpp", "c"},
}

// routeExtensions builds the extension → language table a watch path uses to
// decide which parser reads a file.
//
// The result depends only on the SET of declared languages, never on the order
// they appear in configuration and never on map iteration order. That matters
// more than it looks: the language a file is parsed as fixes the {domain}
// segment of its entity IDs, so a routing decision that varied between runs
// would give the same file different identities on different passes — the
// opposite of the intrinsic, reproducible IDs the graph requires.
//
// extensionsFor reports the extensions a language claims; it is a parameter so
// the routing rule can be exercised without a populated global registry.
func routeExtensions(languages []string, extensionsFor func(string) []string) map[string]string {
	// Sort so configuration order cannot influence the outcome. Two watch paths
	// declaring the same languages in a different order must route identically.
	sorted := append([]string(nil), languages...)
	sort.Strings(sorted)

	claimants := map[string][]string{}
	for _, lang := range sorted {
		for _, ext := range extensionsFor(lang) {
			claimants[ext] = append(claimants[ext], lang)
		}
	}

	routes := make(map[string]string, len(claimants))
	for ext, langs := range claimants {
		routes[ext] = resolveExtension(ext, langs)
	}
	return routes
}

// resolveExtension picks the single language that reads ext. langs is already
// sorted and non-empty.
func resolveExtension(ext string, langs []string) string {
	if len(langs) == 1 {
		return langs[0]
	}
	declared := make(map[string]bool, len(langs))
	for _, l := range langs {
		declared[l] = true
	}
	for _, preferred := range extensionPrecedence[ext] {
		if declared[preferred] {
			return preferred
		}
	}
	// No rule covers this overlap. Fall back to the first in sorted order so the
	// choice is still deterministic and the graph stays reproducible — an
	// arbitrary winner is survivable, a varying one is not. TestEveryContested…
	// fails in this case so the pair gets a deliberate rule instead.
	return langs[0]
}

// contestedExtensions reports the extensions claimed by more than one of the
// given languages, mapped to their claimants. Used by the guard test that keeps
// a new overlapping registration from relying on the arbitrary fallback above.
func contestedExtensions(languages []string, extensionsFor func(string) []string) map[string][]string {
	sorted := append([]string(nil), languages...)
	sort.Strings(sorted)

	claimants := map[string][]string{}
	for _, lang := range sorted {
		for _, ext := range extensionsFor(lang) {
			claimants[ext] = append(claimants[ext], lang)
		}
	}
	for ext, langs := range claimants {
		if len(langs) < 2 {
			delete(claimants, ext)
		}
	}
	return claimants
}

// typeRefResolver is implemented by parsers that can only resolve type
// references once every file in a watch path has been parsed.
//
// Most languages do not need it: Go, Java, and Python derive a definition's path
// from its name through package conventions, so their parsers resolve during
// ParseFile. C++ has no such convention — which header defines `class Foo`
// depends on what was included — so its resolution is deferred to the point
// where the full definition set is known. Mirrors the optional ParseDirectory
// interface the doc handler already uses.
type typeRefResolver interface {
	ResolveTypeRefs(results []*semsourceast.ParseResult)
}
