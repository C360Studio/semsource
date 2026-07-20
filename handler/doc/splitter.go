package doc

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Passage size bounds, in bytes.
//
// passageCeiling is the structural target: a section larger than this is
// subdivided on paragraph, then sentence, boundaries. passageFloor is the
// merge threshold: consecutive sections smaller than this share a passage, so a
// run of one-line headings does not mint a passage each. passageHardMax is the
// only bound that is never exceeded — it exists because the substrate truncates
// embedding text at 8000 characters with no way to configure it
// (graph-embedding/component.go), so a passage above that limit would be
// silently unindexed past the cut. The gap between ceiling and hard max is
// headroom for the one case where structure beats size: a fenced code block is
// kept whole even when it exceeds the ceiling.
//
// These three numbers are the open question in the change's design: the
// architecture is settled but the values are not, and they should be chosen by
// A/B over the graded interrogation rather than guessed. They are deliberately
// consts in one place so that tuning is a one-line change.
const (
	passageCeiling = 2000
	passageFloor   = 400
	passageHardMax = 6000
)

// passage is one retrievable slice of a document. Start and End are byte
// offsets into the original content, and Body is exactly content[Start:End] —
// passages tile the document with no gap and no overlap, so concatenating every
// Body in ordinal order reproduces the input byte for byte.
type passage struct {
	Ordinal int
	Heading string
	Start   int
	End     int
	Body    string
}

// line is one physical line of the document, carrying its byte span and whether
// it sits inside a fenced code block or YAML frontmatter (in either case it is
// not eligible to be a heading).
type line struct {
	start, end int
	text       string
	protected  bool
}

// splitPassages divides content into passages on structural boundaries. It is a
// pure function of the input bytes: the same content always yields the same
// boundaries, independent of machine, run order, or wall clock.
func splitPassages(content []byte) []passage {
	if len(content) == 0 {
		return nil
	}

	lines := scanLines(content)
	sections := sectionsOf(lines, len(content))
	sections = mergeSmallSections(sections)

	var out []passage
	for _, s := range sections {
		for _, span := range subdivide(content, lines, s) {
			out = append(out, passage{
				Ordinal: len(out),
				Heading: s.heading,
				Start:   span[0],
				End:     span[1],
				Body:    string(content[span[0]:span[1]]),
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

// section is a heading and the byte span it governs.
type section struct {
	heading    string
	start, end int
}

// sectionsOf splits the document at heading boundaries. Content before the
// first heading becomes a section with an empty heading — that content is why
// passage identity cannot be heading-derived.
func sectionsOf(lines []line, total int) []section {
	type mark struct {
		lineIdx int
		heading string
	}
	var marks []mark
	for i := range lines {
		if text, ok := atxHeading(lines, i); ok {
			marks = append(marks, mark{lineIdx: i, heading: text})
			continue
		}
		if text, ok := setextHeading(lines, i); ok {
			marks = append(marks, mark{lineIdx: i, heading: text})
		}
	}

	if len(marks) == 0 {
		return []section{{start: 0, end: total}}
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
		out = append(out, section{heading: m.heading, start: lines[m.lineIdx].start, end: end})
	}
	return out
}

// atxHeading reports a "# Heading" style heading on line i.
func atxHeading(lines []line, i int) (string, bool) {
	if lines[i].protected {
		return "", false
	}
	t := strings.TrimSpace(lines[i].text)
	hashes := 0
	for hashes < len(t) && t[hashes] == '#' {
		hashes++
	}
	if hashes == 0 || hashes > 6 {
		return "", false
	}
	rest := t[hashes:]
	if rest != "" && !strings.HasPrefix(rest, " ") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimRight(rest, "#")), true
}

// setextHeading reports a heading underlined by "===" or "---" on the next
// line. The text line must be ordinary prose: a list marker, block quote, or
// another heading underneath a rule is a thematic break, not a heading.
func setextHeading(lines []line, i int) (string, bool) {
	if lines[i].protected || i+1 >= len(lines) || lines[i+1].protected {
		return "", false
	}
	text := strings.TrimSpace(lines[i].text)
	if text == "" || strings.HasPrefix(text, "#") {
		return "", false
	}
	switch text[0] {
	case '-', '*', '+', '>', '|', '=':
		return "", false
	}
	under := strings.TrimSpace(lines[i+1].text)
	if len(under) < 2 {
		return "", false
	}
	c := under[0]
	if c != '=' && c != '-' {
		return "", false
	}
	if strings.Trim(under, string(c)) != "" {
		return "", false
	}
	return text, true
}

// mergeSmallSections folds consecutive below-floor sections together so a run of
// terse headings does not mint a passage each. The merged span keeps the first
// section's heading, and a merge never pushes a passage past the ceiling.
func mergeSmallSections(in []section) []section {
	if len(in) == 0 {
		return in
	}
	out := []section{in[0]}
	for _, s := range in[1:] {
		prev := &out[len(out)-1]
		prevSize := prev.end - prev.start
		if prevSize < passageFloor && (s.end-prev.start) <= passageCeiling {
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
func subdivide(content []byte, lines []line, s section) [][2]int {
	if s.end-s.start <= passageCeiling {
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
		if cur[1] > cur[0] && (cur[1]-cur[0])+size > passageCeiling {
			flush()
			cur = [2]int{blk[0], blk[0]}
		}
		if size > passageCeiling {
			flush()
			for _, piece := range splitOversized(content, lines, blk) {
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
func splitOversized(content []byte, lines []line, blk [2]int) [][2]int {
	if blockIsFenced(lines, blk) {
		if blk[1]-blk[0] <= passageHardMax {
			return [][2]int{blk}
		}
		return cutOnLines(lines, blk)
	}
	return cutOnSentences(content, blk)
}

func blockIsFenced(lines []line, blk [2]int) bool {
	for i := range lines {
		if lines[i].start >= blk[0] && lines[i].end <= blk[1] && lines[i].protected {
			return true
		}
	}
	return false
}

func cutOnLines(lines []line, blk [2]int) [][2]int {
	var out [][2]int
	cur := [2]int{blk[0], blk[0]}
	for i := range lines {
		ln := lines[i]
		if ln.start < blk[0] || ln.end > blk[1] {
			continue
		}
		if cur[1] > cur[0] && (ln.end-cur[0]) > passageHardMax {
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
func cutOnSentences(content []byte, blk [2]int) [][2]int {
	var out [][2]int
	start := blk[0]
	last := blk[0]
	for _, end := range sentenceEnds(content, blk) {
		if end-start > passageCeiling && last > start {
			out = append(out, [2]int{start, last})
			start = last
		}
		for end-start > passageHardMax {
			cut := runeBoundary(content, start+passageHardMax)
			out = append(out, [2]int{start, cut})
			start = cut
		}
		last = end
	}
	if blk[1] > start {
		for blk[1]-start > passageHardMax {
			cut := runeBoundary(content, start+passageHardMax)
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
