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

// candidateLess is the deterministic total order used for sorting. It prefers
// semver, then first-index timestamp, then version string, then ID — so the
// sequence is stable and reproducible even for non-orderable versions (which
// simply get an arbitrary-but-fixed position and no edge).
func candidateLess(a, b candidate) bool {
	av, aerr := semver.NewVersion(a.version)
	bv, berr := semver.NewVersion(b.version)
	if aerr == nil && berr == nil {
		if !av.Equal(bv) {
			return av.LessThan(bv)
		}
		return a.id < b.id
	}
	if !a.indexedAt.Equal(b.indexedAt) {
		return a.indexedAt.Before(b.indexedAt)
	}
	if a.version != b.version {
		return a.version < b.version
	}
	return a.id < b.id
}

// versionComparable reports whether a directional supersession edge may be
// emitted between two adjacent (sorted) members. Distinct semver versions are
// comparable; otherwise the pair is comparable only if their first-index
// timestamps differ. Equal semver or equal timestamps → incomparable → no edge
// (design D4: versions with no total order coexist without an edge).
func versionComparable(a, b candidate) bool {
	av, aerr := semver.NewVersion(a.version)
	bv, berr := semver.NewVersion(b.version)
	if aerr == nil && berr == nil {
		return !av.Equal(bv)
	}
	return !a.indexedAt.Equal(b.indexedAt)
}
