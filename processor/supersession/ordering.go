package supersession

import (
	"sort"

	semver "github.com/Masterminds/semver/v3"
)

// orderGroup returns the group's members in ascending version order (oldest
// first) using a deterministic total order: semver-comparable members order by
// semver, others by natural (numeric-aware) comparison of the version strings,
// with a final stable tiebreak on ID. The returned slice is a copy; the input
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
// natural version-string comparison), with a stable ID tiebreak. Partitioning
// by scheme is what keeps the comparator transitive — deciding some pairs by
// semver and other pairs by another key within one sort is not transitive and
// can corrupt the order (and thus edge direction).
//
// The non-semver key is a pure function of the version STRINGS: the audit
// found the previous first-index-timestamp key inverts across restarts
// (dc.terms.created is rewritten on re-ingest), which could flip lineage
// direction and demote both versions (version-registration-surface D2).
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
	// both non-semver: natural version order, then ID
	if c := naturalCompare(a.version, b.version); c != 0 {
		return c < 0
	}
	return a.id < b.id
}

// naturalCompare orders two strings numeric-aware: digit runs compare by value
// (v9 < v10), non-digit runs compare lexically (r2023b < r2024a). Returns
// -1/0/+1. Pure function of its inputs — restart-stable by construction — and
// derived from a canonical run decomposition, so it is total and transitive.
func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		ar, arest, aNum := nextRun(a)
		br, brest, bNum := nextRun(b)
		switch {
		case aNum && bNum:
			// Compare digit runs by value: longer (trimmed) run is larger.
			at, bt := trimLeadingZeros(ar), trimLeadingZeros(br)
			if len(at) != len(bt) {
				if len(at) < len(bt) {
					return -1
				}
				return 1
			}
			if at != bt {
				if at < bt {
					return -1
				}
				return 1
			}
		case !aNum && !bNum:
			if ar != br {
				if ar < br {
					return -1
				}
				return 1
			}
		case aNum:
			return -1 // a digit run sorts before a text run at the same position
		default:
			return 1
		}
		a, b = arest, brest
	}
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	default:
		return 1
	}
}

// nextRun splits off the leading maximal digit or non-digit run.
func nextRun(s string) (run, rest string, numeric bool) {
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	numeric = isDigit(s[0])
	i := 1
	for i < len(s) && isDigit(s[i]) == numeric {
		i++
	}
	return s[:i], s[i:], numeric
}

// trimLeadingZeros normalizes a digit run for value comparison ("007" == "7").
func trimLeadingZeros(s string) string {
	i := 0
	for i < len(s)-1 && s[i] == '0' {
		i++
	}
	return s[i:]
}

// versionComparable reports whether a directional supersession edge may be
// emitted between two adjacent (sorted) members. Only SAME-SCHEME pairs are
// comparable: two distinct semver versions, or two non-semver versions with
// different version strings. A semver/non-semver pair is cross-scheme and left
// incomparable — asserting a direction between them would guess lineage the
// versions don't actually carry (design D4: never guess).
func versionComparable(a, b candidate) bool {
	av, aok := semverOf(a)
	bv, bok := semverOf(b)
	if aok != bok {
		return false // cross-scheme (one semver, one not) — never relate
	}
	if aok {
		return !av.Equal(bv)
	}
	return a.version != b.version
}
