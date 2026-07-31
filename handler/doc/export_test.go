package doc

// PassageHardMax re-exports the passage size bound to the external test package,
// so the passage-size contract is pinned against the real value instead of a
// copy of it that would silently drift the day the bound is tuned. It reads
// from defaultBounds rather than being a const itself, now that hardMax lives
// as a field of passageBounds instead of a package-level const.
var PassageHardMax = defaultBounds.hardMax

// SplitPassagesBounded re-exports the parameterized splitter to the external
// test package, so tuning/scorecard tests can vary the three passage size
// bounds directly instead of copying the splitting logic.
func SplitPassagesBounded(content []byte, ceiling, floor, hardMax int) []passage {
	return splitPassagesBounded(content, passageBounds{ceiling: ceiling, floor: floor, hardMax: hardMax})
}

// Passage re-exports the passage type to the external test package so a
// harness can name it in helper signatures. An alias rather than a definition:
// SplitPassagesBounded returns the real type, and a distinct named type would
// need conversions at every call site.
type Passage = passage
