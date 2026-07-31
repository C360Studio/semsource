package doc

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// passageBounds is the set of passage size bounds, in bytes, that govern how
// sections are merged and split into passages.
//
// ceiling is the structural target: a section larger than this is subdivided
// on paragraph, then sentence, boundaries. floor is the merge threshold:
// consecutive sections smaller than this share a passage, so a run of
// one-line headings does not mint a passage each. hardMax is the only bound
// that is never exceeded — it exists because the substrate truncates
// embedding text at 8000 characters with no way to configure it
// (graph-embedding/component.go), so a passage above that limit would be
// silently unindexed past the cut. The gap between ceiling and hard max is
// headroom for the one case where structure beats size: a fenced code block is
// kept whole even when it exceeds the ceiling.
//
// These three numbers are the open question in the change's design: the
// architecture is settled but the values are not, and they should be chosen by
// A/B over the graded interrogation rather than guessed. They live in one
// struct so that tuning is a one-line change, and splitPassagesBounded takes
// one explicitly so an A/B harness (or a test) can vary them without touching
// the shipped defaults.
type passageBounds struct {
	ceiling, floor, hardMax int
}

// defaultBounds are the passage size bounds used by splitPassages, and so by
// every production caller. This is the single place today's shipped values —
// 2000/400/6000 — are recorded.
var defaultBounds = passageBounds{ceiling: 2000, floor: 400, hardMax: 6000}

// passage is one retrievable slice of a document. Start and End are byte
// offsets into the original content, and Body is exactly content[Start:End] —
// passages tile the document with no gap and no overlap, so concatenating every
// Body in ordinal order reproduces the input byte for byte.
//
// Heading is the immediate section heading and feeds everything that must match
// the document verbatim (the DocSection triple, and through it URL anchors).
// HeadingPath is the heading's ancestor chain including itself, outermost
// first, and feeds only the passage title — the identity text the embedding
// ranks by. The distinction is measured, not cosmetic: for the query "default
// host port for the NATS monitor in docker compose", README's NATS settings
// group scored 0.7584 under the title "SemSource § Configuration" and 0.7803
// under "SemSource § Docker Compose § Configuration", flipping a −0.0009 loss
// to the workaround passage into a +0.0209 win. The ancestry was always in the
// document; discarding it made two same-named facts indistinguishable.
type passage struct {
	Ordinal     int
	Heading     string
	HeadingPath []string
	Start       int
	End         int
	Body        string
}

// headingPath is HeadingPath with a fallback for passages built without one
// (frozen fixtures, hand-made test values): a bare Heading is its own
// single-element path.
func (p passage) headingPath() []string {
	if len(p.HeadingPath) > 0 {
		return p.HeadingPath
	}
	if p.Heading != "" {
		return []string{p.Heading}
	}
	return nil
}

// line is one physical line of the document, carrying its byte span and whether
// it sits inside a fenced code block or YAML frontmatter (in either case it is
// not eligible to be a heading).
type line struct {
	start, end int
	text       string
	protected  bool
}

// splitPassages divides content into passages on structural boundaries, using
// the shipped default bounds. It is a pure function of the input bytes: the
// same content always yields the same boundaries, independent of machine, run
// order, or wall clock.
func splitPassages(content []byte) []passage {
	return splitPassagesBounded(content, defaultBounds)
}

// splitPassagesBounded is splitPassages parameterized on passageBounds, so a
// caller can vary the size bounds without touching the shipped defaults.
func splitPassagesBounded(content []byte, b passageBounds) []passage {
	if len(content) == 0 {
		return nil
	}

	lines := scanLines(content)
	sections := sectionsOf(lines, len(content))
	sections = mergeSmallSections(sections, b)

	var out []passage
	for _, s := range sections {
		for _, span := range subdivide(content, lines, s, b) {
			out = append(out, passage{
				Ordinal:     len(out),
				Heading:     s.heading,
				HeadingPath: s.path,
				Start:       span[0],
				End:         span[1],
				Body:        string(content[span[0]:span[1]]),
			})
		}
	}
	return out
}

// scanLines splits content into lines and marks the ones that cannot host a
// heading: everything inside a fenced code block (including the fence
// delimiters) and everything inside leading YAML frontmatter. Frontmatter needs
// its own guard because its closing "---" follows a text line and would
// otherwise read as a setext H2.
func scanLines(content []byte) []line {
	var lines []line
	for off := 0; off < len(content); {
		end := off
		for end < len(content) && content[end] != '\n' {
			end++
		}
		if end < len(content) {
			end++ // include the newline
		}
		lines = append(lines, line{start: off, end: end, text: strings.TrimRight(string(content[off:end]), "\r\n")})
		off = end
	}

	inFence := false
	inFrontMatter := len(lines) > 0 && strings.TrimSpace(lines[0].text) == "---"
	for i := range lines {
		trimmed := strings.TrimSpace(lines[i].text)
		if inFrontMatter {
			lines[i].protected = true
			if i > 0 && trimmed == "---" {
				inFrontMatter = false
			}
			continue
		}
		if isFenceDelimiter(trimmed) {
			inFence = !inFence
			lines[i].protected = true
			continue
		}
		lines[i].protected = inFence
	}
	return lines
}

func isFenceDelimiter(trimmed string) bool {
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

// section is a heading and the byte span it governs. path is the heading's
// ancestor chain including itself, outermost first — the structural context the
// document declared with heading levels, preserved here so passage identity can
// carry it.
type section struct {
	heading    string
	path       []string
	start, end int
}

// sectionsOf splits the document at heading boundaries. Content before the
// first heading becomes a section with an empty heading — that content is why
// passage identity cannot be heading-derived.
func sectionsOf(lines []line, total int) []section {
	type mark struct {
		lineIdx int
		level   int
		heading string
	}
	var marks []mark
	for i := range lines {
		if text, level, ok := atxHeading(lines, i); ok {
			marks = append(marks, mark{lineIdx: i, level: level, heading: text})
			continue
		}
		if text, level, ok := setextHeading(lines, i); ok {
			marks = append(marks, mark{lineIdx: i, level: level, heading: text})
		}
	}

	if len(marks) == 0 {
		return []section{{start: 0, end: total}}
	}

	// ancestors is the stack of open headings, strictly increasing in level. A
	// new mark pops everything at its own level or deeper: a heading's ancestors
	// are exactly the shallower headings still open above it. Skipped levels
	// (an H4 directly under an H2) keep whatever is genuinely open — the path
	// records the document's actual structure, not an idealized outline.
	var ancestors []mark
	pathOf := func(m mark) []string {
		for len(ancestors) > 0 && ancestors[len(ancestors)-1].level >= m.level {
			ancestors = ancestors[:len(ancestors)-1]
		}
		path := make([]string, 0, len(ancestors)+1)
		for _, a := range ancestors {
			path = append(path, a.heading)
		}
		path = append(path, m.heading)
		ancestors = append(ancestors, m)
		return path
	}

	var out []section
	if marks[0].lineIdx > 0 {
		out = append(out, section{start: 0, end: lines[marks[0].lineIdx].start})
	}
	for i, m := range marks {
		end := total
		if i+1 < len(marks) {
			end = lines[marks[i+1].lineIdx].start
		}
		out = append(out, section{heading: m.heading, path: pathOf(m), start: lines[m.lineIdx].start, end: end})
	}
	return out
}

// atxHeading reports a "# Heading" style heading on line i, with its level.
func atxHeading(lines []line, i int) (string, int, bool) {
	if lines[i].protected {
		return "", 0, false
	}
	t := strings.TrimSpace(lines[i].text)
	hashes := 0
	for hashes < len(t) && t[hashes] == '#' {
		hashes++
	}
	if hashes == 0 || hashes > 6 {
		return "", 0, false
	}
	rest := t[hashes:]
	if rest != "" && !strings.HasPrefix(rest, " ") {
		return "", 0, false
	}
	return strings.TrimSpace(strings.TrimRight(rest, "#")), hashes, true
}

// setextHeading reports a heading underlined by "===" or "---" on the next
// line, with its level: "===" is H1 and "---" is H2, per CommonMark. The text
// line must be ordinary prose: a list marker, block quote, or another heading
// underneath a rule is a thematic break, not a heading.
func setextHeading(lines []line, i int) (string, int, bool) {
	if lines[i].protected || i+1 >= len(lines) || lines[i+1].protected {
		return "", 0, false
	}
	text := strings.TrimSpace(lines[i].text)
	if text == "" || strings.HasPrefix(text, "#") {
		return "", 0, false
	}
	switch text[0] {
	case '-', '*', '+', '>', '|', '=':
		return "", 0, false
	}
	under := strings.TrimSpace(lines[i+1].text)
	if len(under) < 2 {
		return "", 0, false
	}
	c := under[0]
	if c != '=' && c != '-' {
		return "", 0, false
	}
	if strings.Trim(under, string(c)) != "" {
		return "", 0, false
	}
	level := 1
	if c == '-' {
		level = 2
	}
	return text, level, true
}

// mergeSmallSections folds consecutive below-floor sections together so a run of
// terse headings does not mint a passage each. The merged span keeps the first
// section's heading, and a merge never pushes a passage past the ceiling.
func mergeSmallSections(in []section, b passageBounds) []section {
	if len(in) == 0 {
		return in
	}
	out := []section{in[0]}
	for _, s := range in[1:] {
		prev := &out[len(out)-1]
		prevSize := prev.end - prev.start
		if prevSize < b.floor && (s.end-prev.start) <= b.ceiling {
			prev.end = s.end
			continue
		}
		out = append(out, s)
	}
	return out
}

// subdivide breaks one section into byte spans no larger than the ceiling,
// cutting on paragraph boundaries first and sentence boundaries second. It
// returns [start,end) pairs that exactly tile the section.
func subdivide(content []byte, lines []line, s section, b passageBounds) [][2]int {
	// Homogeneity is checked BEFORE size, and is the only split reason that does
	// not depend on it. A list of independent settings dilutes its own vector at
	// any size: measured on this repository, README § Configuration is 1363 bytes
	// — under every ceiling ever tested — and the fact a query asked for scored
	// 0.6569 inside it against 0.8133 alone, losing to a workaround value in a
	// different section. A size-gated path can never reach that block, which is
	// why a 4x sweep of the ceiling moved no graded outcome.
	if spans := splitHomogeneousBlocks(lines, s); spans != nil {
		return spans
	}
	// Isolation is the second size-independent reason, and it is about a
	// different competition than homogeneity. Homogeneity stops a block's entries
	// diluting EACH OTHER; isolation stops a block's facts competing with the
	// PROSE AROUND IT. X02 is the case: its answer is a two-line `docker run`
	// block, so it yields too few groups for the homogeneous path to touch, and
	// its section sits under the ceiling so the size path never runs. Measured on
	// the unmodified document, the section scored 0.6116 against a distractor's
	// 0.6304 while the isolated block scored 0.6535 — a losing margin of -0.0188
	// becoming a winning +0.0231.
	if spans := isolateFencedBlocks(lines, s, b); spans != nil {
		return spans
	}
	if s.end-s.start <= b.ceiling {
		return [][2]int{{s.start, s.end}}
	}

	var out [][2]int
	cur := [2]int{s.start, s.start}
	flush := func() {
		if cur[1] > cur[0] {
			out = append(out, cur)
		}
	}
	for _, blk := range blocksOf(lines, s) {
		size := blk[1] - blk[0]
		if cur[1] > cur[0] && (cur[1]-cur[0])+size > b.ceiling {
			flush()
			cur = [2]int{blk[0], blk[0]}
		}
		if size > b.ceiling {
			flush()
			for _, piece := range splitOversized(content, lines, blk, b) {
				out = append(out, piece)
			}
			cur = [2]int{blk[1], blk[1]}
			continue
		}
		cur[1] = blk[1]
	}
	flush()
	if len(out) == 0 {
		return [][2]int{{s.start, s.end}}
	}
	return out
}

// blocksOf splits a section into paragraph blocks on blank lines. A fenced code
// block is one atomic block regardless of the blank lines inside it, which is
// what keeps a fence from being cut in half.
func blocksOf(lines []line, s section) [][2]int {
	var out [][2]int
	cur := [2]int{-1, -1}
	for i := range lines {
		ln := lines[i]
		if ln.start < s.start || ln.end > s.end {
			continue
		}
		blank := strings.TrimSpace(ln.text) == "" && !ln.protected
		if blank {
			if cur[0] >= 0 {
				cur[1] = ln.end
				out = append(out, cur)
				cur = [2]int{-1, -1}
			} else if len(out) > 0 {
				out[len(out)-1][1] = ln.end
			}
			continue
		}
		if cur[0] < 0 {
			cur = [2]int{ln.start, ln.end}
		} else {
			cur[1] = ln.end
		}
	}
	if cur[0] >= 0 {
		out = append(out, cur)
	}
	if len(out) == 0 {
		return [][2]int{{s.start, s.end}}
	}
	out[0][0] = s.start
	out[len(out)-1][1] = s.end
	return out
}

// splitOversized cuts a single block that is larger than the ceiling. A fenced
// block is preserved whole up to the hard max, because splitting code hurts
// retrieval more than an oversized passage does; past the hard max it is cut on
// line boundaries so nothing is ever silently truncated by the substrate.
// Ordinary prose is cut on sentence boundaries, then hard-cut as a last resort.
func splitOversized(content []byte, lines []line, blk [2]int, b passageBounds) [][2]int {
	if blockIsFenced(lines, blk) {
		if blk[1]-blk[0] <= b.hardMax {
			return [][2]int{blk}
		}
		return cutOnLines(lines, blk, b)
	}
	return cutOnSentences(content, blk, b)
}

func blockIsFenced(lines []line, blk [2]int) bool {
	for i := range lines {
		if lines[i].start >= blk[0] && lines[i].end <= blk[1] && lines[i].protected {
			return true
		}
	}
	return false
}

func cutOnLines(lines []line, blk [2]int, b passageBounds) [][2]int {
	var out [][2]int
	cur := [2]int{blk[0], blk[0]}
	for i := range lines {
		ln := lines[i]
		if ln.start < blk[0] || ln.end > blk[1] {
			continue
		}
		if cur[1] > cur[0] && (ln.end-cur[0]) > b.hardMax {
			out = append(out, cur)
			cur = [2]int{ln.start, ln.end}
			continue
		}
		cur[1] = ln.end
	}
	if cur[1] > cur[0] {
		out = append(out, cur)
	}
	return out
}

// cutOnSentences accumulates sentences up to the ceiling. A single sentence
// longer than the hard max is cut at a rune boundary — never mid-rune.
func cutOnSentences(content []byte, blk [2]int, b passageBounds) [][2]int {
	var out [][2]int
	start := blk[0]
	last := blk[0]
	for _, end := range sentenceEnds(content, blk) {
		if end-start > b.ceiling && last > start {
			out = append(out, [2]int{start, last})
			start = last
		}
		for end-start > b.hardMax {
			cut := runeBoundary(content, start+b.hardMax)
			out = append(out, [2]int{start, cut})
			start = cut
		}
		last = end
	}
	if blk[1] > start {
		for blk[1]-start > b.hardMax {
			cut := runeBoundary(content, start+b.hardMax)
			out = append(out, [2]int{start, cut})
			start = cut
		}
		out = append(out, [2]int{start, blk[1]})
	}
	return out
}

// sentenceEnds returns the byte offsets just past each sentence terminator in
// the block.
func sentenceEnds(content []byte, blk [2]int) []int {
	var ends []int
	for i := blk[0]; i < blk[1]; i++ {
		switch content[i] {
		case '.', '!', '?':
			j := i + 1
			if j < blk[1] && (content[j] == '"' || content[j] == '\'' || content[j] == ')') {
				j++
			}
			if j >= blk[1] || unicode.IsSpace(rune(content[j])) {
				ends = append(ends, j)
			}
		}
	}
	return ends
}

// runeBoundary backs off to the nearest rune start at or before i so a hard cut
// never splits a multi-byte character.
func runeBoundary(content []byte, i int) int {
	if i >= len(content) {
		return len(content)
	}
	for i > 0 && !utf8.RuneStart(content[i]) {
		i--
	}
	return i
}

// minKeyGroups is the number of distinct key groups a homogeneous block must
// yield before it is worth dividing. Two related settings are not diluting each
// other in any way this can measure, and splitting them would mint passages for
// nothing.
const minKeyGroups = 3

// minKeyValueShare is the fraction of a fenced block's content lines that must
// be KEY=VALUE assignments for it to count as a homogeneous key/value list.
// Below this the block is prose or code that merely contains an assignment, and
// splitting it would cut something whose parts are not independent.
const minKeyValueShare = 0.6

// splitHomogeneousBlocks divides a section when it contains a fenced block that
// is a homogeneous list of KEY=VALUE settings, returning spans that tile the
// section exactly. It returns nil when no block qualifies, leaving the size-based
// path untouched.
//
// The qualifying block is divided on key-group boundaries; everything around it
// (heading, prose, the fence delimiters) tiles alongside as its own spans. The
// section heading is deliberately NOT repeated into each group: passages must
// concatenate back to the document byte for byte, and the heading is worth
// nothing to the vector anyway — measured, the same fact scored 0.8127 with its
// heading and 0.8133 without.
func splitHomogeneousBlocks(lines []line, s section) [][2]int {
	var out [][2]int
	cursor := s.start
	found := false
	for _, blk := range blocksOf(lines, s) {
		groups := keyValueGroups(lines, blk)
		if groups == nil {
			continue
		}
		found = true
		if groups[0][0] > cursor {
			out = append(out, [2]int{cursor, groups[0][0]})
		}
		out = append(out, groups...)
		cursor = groups[len(groups)-1][1]
	}
	if !found {
		return nil
	}
	if cursor < s.end {
		out = append(out, [2]int{cursor, s.end})
	}
	return out
}

// isolateFencedBlocks gives each fenced block in a section its own span, leaving
// the prose around it as its own spans, so a command or code sample is not
// embedded together with paragraphs that dilute it.
//
// It NEVER divides a fenced block — a block stays whole, which is what the
// splitting contract requires and what makes this safe for continuous
// constructs. It only decides where the block's passage begins and ends.
//
// The trigger is prose VOLUME, not the ceiling. A section whose non-fence
// content is below the floor has nothing substantial to dilute the block, and
// isolating there would mint tiny passages for no retrieval gain. The ceiling is
// deliberately not consulted: a 4x sweep of it moved no graded outcome, because
// dilution is a property of what a passage contains rather than how large it is.
//
// Returns nil when the section has no fenced block, or when the surrounding
// prose is too small to be worth separating — leaving every other path untouched.
func isolateFencedBlocks(lines []line, s section, b passageBounds) [][2]int {
	var fenced [][2]int
	for _, blk := range blocksOf(lines, s) {
		if blockIsFenced(lines, blk) {
			fenced = append(fenced, blk)
		}
	}
	if len(fenced) == 0 {
		return nil
	}

	var fencedBytes int
	for _, f := range fenced {
		fencedBytes += f[1] - f[0]
	}
	// Non-fence content is what would dilute the block. Below the floor there is
	// not enough of it to matter.
	if (s.end-s.start)-fencedBytes < b.floor {
		return nil
	}

	var out [][2]int
	cursor := s.start
	for _, f := range fenced {
		if f[0] > cursor {
			out = append(out, [2]int{cursor, f[0]})
		}
		out = append(out, f)
		cursor = f[1]
	}
	if cursor < s.end {
		out = append(out, [2]int{cursor, s.end})
	}
	return out
}

// keyValueGroups returns one span per run of consecutive KEY=VALUE lines sharing
// a leading token, for a fenced block that qualifies as a homogeneous key/value
// list. It returns nil when the block does not qualify.
//
// Grouping is purely lexical — the token up to the first underscore, so
// NATS_URL, NATS_HOST_PORT and NATS_MONITOR_HOST_PORT group together. Nothing
// here knows what NATS is. That is the point: deciding which lines belong
// together by reading them would be the prose classification this deliberately
// avoids, and it would be far more fragile than the key names already are.
//
// Only CONSECUTIVE lines group, so a key that reappears later in the block forms
// its own group. That keeps spans contiguous and the tiling trivially correct.
func keyValueGroups(lines []line, blk [2]int) [][2]int {
	if !blockIsFenced(lines, blk) {
		return nil
	}
	var content, assignments int
	var groups [][2]int
	prefix := ""
	for i := range lines {
		ln := lines[i]
		if ln.start < blk[0] || ln.end > blk[1] {
			continue
		}
		trimmed := strings.TrimSpace(ln.text)
		if trimmed == "" || isFenceDelimiter(trimmed) {
			continue
		}
		content++
		key, ok := assignmentPrefix(trimmed)
		if !ok {
			prefix = ""
			continue
		}
		assignments++
		if key == prefix && len(groups) > 0 {
			groups[len(groups)-1][1] = ln.end
			continue
		}
		groups = append(groups, [2]int{ln.start, ln.end})
		prefix = key
	}
	if content == 0 || len(groups) < minKeyGroups {
		return nil
	}
	if float64(assignments)/float64(content) < minKeyValueShare {
		return nil
	}
	return groups
}

// assignmentPrefix reports whether a line is a KEY=VALUE assignment and, if so,
// the grouping token: the key up to its first underscore.
func assignmentPrefix(trimmed string) (string, bool) {
	eq := strings.IndexByte(trimmed, '=')
	if eq <= 0 {
		return "", false
	}
	key := trimmed[:eq]
	for i, r := range key {
		valid := r == '_' || unicode.IsDigit(r) || unicode.IsLetter(r)
		if !valid || (i == 0 && unicode.IsDigit(r)) {
			return "", false
		}
	}
	if under := strings.IndexByte(key, '_'); under > 0 {
		return key[:under], true
	}
	return key, true
}
