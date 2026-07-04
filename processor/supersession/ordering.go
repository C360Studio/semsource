package supersession

import (
	"sort"

	semver "github.com/Masterminds/semver/v3"
)

// orderGroup returns the group's members in ascending version order (oldest
// first) using a deterministic total order: semver-comparable members order by
// semver, others fall back to first-index timestamp, with a final stable
// tiebreak on version string then ID. The returned slice is a copy; the input
// is not mutated.
//
// The total order here is only for producing a stable sequence to walk; whether
// an EDGE is actually emitted between two adjacent members is decided
// separately by versionComparable (design D4: never guess a direction).
func orderGroup(group []candidate) []candidate {
	ordered := make([]candidate, len(group))
	copy(ordered, group)
	sort.SliceStable(ordered, func(i, j int) bool {
		return candidateLess(ordered[i], ordered[j])
	})
	return ordered
}

// semverOf parses a candidate's version as a semantic version, returning
// (version, true) when it is valid semver. Centralizing the parse keeps the
// sort order and the edge-comparability decision agreeing on what counts as
// semver (they diverged when each parsed independently).
func semverOf(c candidate) (*semver.Version, bool) {
	v, err := semver.NewVersion(c.version)
	return v, err == nil
}

// candidateLess is a TOTAL, TRANSITIVE order: all semver-valid members sort
// first (ascending by semver), then all non-semver members (ascending by
// first-index timestamp), with stable tiebreaks. Partitioning by scheme is what
// keeps the comparator transitive — deciding some pairs by semver and other
// pairs by timestamp within one sort is not transitive and can corrupt the
// order (and thus edge direction).
func candidateLess(a, b candidate) bool {
	av, aok := semverOf(a)
	bv, bok := semverOf(b)
	if aok != bok {
		return aok // semver-valid sorts before non-semver
	}
	if aok { // both semver
		if !av.Equal(bv) {
			return av.LessThan(bv)
		}
		return a.id < b.id
	}
	// both non-semver: order by first-index timestamp, then version, then ID
	if !a.indexedAt.Equal(b.indexedAt) {
		return a.indexedAt.Before(b.indexedAt)
	}
	if a.version != b.version {
		return a.version < b.version
	}
	return a.id < b.id
}

// versionComparable reports whether a directional supersession edge may be
// emitted between two adjacent (sorted) members. Only SAME-SCHEME pairs are
// comparable: two distinct semver versions, or two non-semver versions with
// different first-index timestamps. A semver/non-semver pair is cross-scheme and
// left incomparable — ordering it by ingest timing would assert a lineage
// direction the versions don't actually carry (design D4: never guess).
func versionComparable(a, b candidate) bool {
	av, aok := semverOf(a)
	bv, bok := semverOf(b)
	if aok != bok {
		return false // cross-scheme (one semver, one not) — never relate
	}
	if aok {
		return !av.Equal(bv)
	}
	return !a.indexedAt.Equal(b.indexedAt)
}
