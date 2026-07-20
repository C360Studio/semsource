package doc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSplitPassages_TilesInputExactly pins the invariant the whole design rests
// on: passages tile the document with no gap and no overlap, so concatenating
// every body in ordinal order reproduces the input byte for byte. If this ever
// fails, some document content is unreachable through retrieval.
func TestSplitPassages_TilesInputExactly(t *testing.T) {
	for name, body := range splitterFixtures() {
		t.Run(name, func(t *testing.T) {
			assertTiles(t, []byte(body))
		})
	}
}

// TestSplitPassages_TilesRealCorpus runs the tiling invariant over every
// Markdown file in this repository. Synthetic fixtures cannot cover the shapes
// real documents take — nested fences, frontmatter, tables, mixed heading
// styles — and this change exists because real documents were being silently
// truncated.
func TestSplitPassages_TilesRealCorpus(t *testing.T) {
	root := repoRoot(t)
	var checked int
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "node_modules" || info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		t.Run(rel, func(t *testing.T) { assertTiles(t, content) })
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if checked < 50 {
		t.Fatalf("expected the repo corpus to cover many documents, only checked %d", checked)
	}
	t.Logf("tiling invariant held over %d real documents", checked)
}

// TestSplitPassages_NeverExceedsHardMax is the requirement that motivates the
// change: the substrate truncates embedding text at 8000 characters with no way
// to configure it, so a passage above the hard max would be silently unindexed
// past the cut.
func TestSplitPassages_NeverExceedsHardMax(t *testing.T) {
	root := repoRoot(t)
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if err == nil && info.IsDir() && (info.Name() == "node_modules" || info.Name() == ".git") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, p := range splitPassages(content) {
			if size := p.End - p.Start; size > passageHardMax {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s passage %d is %d bytes, above the hard max %d — it would be silently truncated",
					rel, p.Ordinal, size, passageHardMax)
			}
		}
		return nil
	})
}

// TestSplitPassages_IsDeterministic pins that splitting is a pure function of
// the bytes. Non-deterministic boundaries would churn every downstream passage
// identity on every ingest.
func TestSplitPassages_IsDeterministic(t *testing.T) {
	for name, body := range splitterFixtures() {
		t.Run(name, func(t *testing.T) {
			first := splitPassages([]byte(body))
			for i := 0; i < 5; i++ {
				again := splitPassages([]byte(body))
				if len(again) != len(first) {
					t.Fatalf("run %d produced %d passages, first run produced %d", i, len(again), len(first))
				}
				for j := range first {
					if again[j] != first[j] {
						t.Fatalf("run %d passage %d differs:\n got %+v\nwant %+v", i, j, again[j], first[j])
					}
				}
			}
		})
	}
}

// TestSplitPassages_SplitsOnHeadings covers the ordinary case and the
// preamble: content above the first heading is a passage of its own, which is
// why passage identity cannot be derived from heading text.
func TestSplitPassages_SplitsOnHeadings(t *testing.T) {
	got := splitPassages([]byte(strings.Repeat("Preamble prose. ", 40) + "\n\n" +
		"# Alpha\n\n" + strings.Repeat("Alpha body. ", 40) + "\n\n" +
		"# Beta\n\n" + strings.Repeat("Beta body. ", 40) + "\n"))

	if len(got) != 3 {
		t.Fatalf("expected preamble + 2 headed sections, got %d passages", len(got))
	}
	if got[0].Heading != "" {
		t.Errorf("preamble heading = %q, want empty", got[0].Heading)
	}
	if got[1].Heading != "Alpha" || got[2].Heading != "Beta" {
		t.Errorf("headings = %q/%q, want Alpha/Beta", got[1].Heading, got[2].Heading)
	}
	for i, p := range got {
		if p.Ordinal != i {
			t.Errorf("passage %d has ordinal %d", i, p.Ordinal)
		}
	}
}

// TestSplitPassages_KeepsFencedCodeWhole pins that a fenced block spanning a
// would-be paragraph boundary is not cut in half. Splitting code costs more in
// retrieval quality than an oversized passage does.
func TestSplitPassages_KeepsFencedCodeWhole(t *testing.T) {
	fence := "```go\n" + strings.Repeat("line of code\n\nblank separated\n", 60) + "```\n"
	content := []byte("# Heading\n\n" + strings.Repeat("prose. ", 100) + "\n\n" + fence)

	for _, p := range splitPassages(content) {
		opens := strings.Count(p.Body, "```")
		if opens%2 != 0 {
			t.Fatalf("passage %d splits a fenced block (odd number of fence markers):\n%s", p.Ordinal, p.Body)
		}
	}
}

// TestSplitPassages_MergesTrivialSections pins the floor: a run of one-line
// headings shares a passage instead of minting one each.
func TestSplitPassages_MergesTrivialSections(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 8; i++ {
		b.WriteString("## Tiny\n\nshort.\n\n")
	}
	got := splitPassages([]byte(b.String()))
	if len(got) >= 8 {
		t.Errorf("expected trivial sections to merge, got %d passages for 8 tiny sections", len(got))
	}
	if len(got) == 0 {
		t.Fatal("expected at least one passage")
	}
}

// TestSplitPassages_SubdividesOversizedSection pins that a single huge section
// is broken up rather than emitted whole.
func TestSplitPassages_SubdividesOversizedSection(t *testing.T) {
	body := "# Big\n\n" + strings.Repeat("This is a sentence of prose. ", 400)
	got := splitPassages([]byte(body))
	if len(got) < 2 {
		t.Fatalf("expected an oversized section to subdivide, got %d passages", len(got))
	}
	for _, p := range got {
		if p.Heading != "Big" {
			t.Errorf("subdivided passage %d lost its heading: %q", p.Ordinal, p.Heading)
		}
		if size := p.End - p.Start; size > passageHardMax {
			t.Errorf("passage %d is %d bytes, above hard max", p.Ordinal, size)
		}
	}
}

// TestSplitPassages_FrontMatterIsNotASetextHeading guards a real ambiguity:
// YAML frontmatter's closing "---" follows a text line and would otherwise read
// as a setext H2, making the document's metadata look like a section title.
func TestSplitPassages_FrontMatterIsNotASetextHeading(t *testing.T) {
	content := []byte("---\ntitle: Something\nauthor: Someone\n---\n\n# Real Heading\n\nbody\n")
	got := splitPassages(content)
	for _, p := range got {
		if strings.Contains(p.Heading, "title:") || strings.Contains(p.Heading, "author:") {
			t.Errorf("frontmatter line became a heading: %q", p.Heading)
		}
	}
	if len(got) == 0 {
		t.Fatal("expected at least one passage")
	}
	assertTiles(t, content)
}

// TestSplitPassages_SetextHeadings covers the underlined heading style. The
// bodies are deliberately above the floor: below it the two sections legitimately
// merge and the second heading is absorbed, which is the floor's job, not a bug.
func TestSplitPassages_SetextHeadings(t *testing.T) {
	content := []byte("Title Here\n==========\n\n" + strings.Repeat("body. ", 100) +
		"\n\nSubtitle\n--------\n\n" + strings.Repeat("more. ", 100) + "\n")
	got := splitPassages(content)

	var headings []string
	for _, p := range got {
		headings = append(headings, p.Heading)
	}
	if !contains(headings, "Title Here") {
		t.Errorf("setext H1 not detected; headings = %v", headings)
	}
	if !contains(headings, "Subtitle") {
		t.Errorf("setext H2 not detected; headings = %v", headings)
	}
}

// TestSplitPassages_HeadingInFenceIsNotAHeading pins that a "#" comment inside
// a code block does not create a section.
func TestSplitPassages_HeadingInFenceIsNotAHeading(t *testing.T) {
	content := []byte("# Real\n\n```sh\n# not a heading\necho hi\n```\n\nbody\n")
	for _, p := range splitPassages(content) {
		if p.Heading == "not a heading" {
			t.Fatal("a comment inside a fenced block was treated as a heading")
		}
	}
}

// TestSplitPassages_EmptyInput pins the degenerate case.
func TestSplitPassages_EmptyInput(t *testing.T) {
	if got := splitPassages(nil); got != nil {
		t.Errorf("expected nil for empty content, got %d passages", len(got))
	}
	if got := splitPassages([]byte("")); got != nil {
		t.Errorf("expected nil for empty content, got %d passages", len(got))
	}
}

// TestSplitPassages_ShortDocumentIsOnePassage pins that a document below the
// floor is not fragmented.
func TestSplitPassages_ShortDocumentIsOnePassage(t *testing.T) {
	got := splitPassages([]byte("# Small\n\nJust a little prose.\n"))
	if len(got) != 1 {
		t.Fatalf("expected one passage for a short document, got %d", len(got))
	}
}

// TestSplitPassages_MultiByteRunesAreNotCut pins that a hard cut lands on a
// rune boundary, so no passage ends mid-character.
func TestSplitPassages_MultiByteRunesAreNotCut(t *testing.T) {
	// One enormous unbroken "sentence" of multi-byte runes forces the hard cut.
	content := []byte("# Unicode\n\n" + strings.Repeat("日本語テキスト", 2000))
	got := splitPassages(content)
	if len(got) < 2 {
		t.Fatalf("expected the hard cut to engage, got %d passages", len(got))
	}
	for _, p := range got {
		if !utf8ValidString(p.Body) {
			t.Errorf("passage %d ends mid-rune", p.Ordinal)
		}
	}
	assertTiles(t, content)
}

// --- helpers ---

func assertTiles(t *testing.T, content []byte) {
	t.Helper()
	got := splitPassages(content)
	if len(content) == 0 {
		return
	}
	if len(got) == 0 {
		t.Fatal("non-empty content produced no passages")
	}

	var b strings.Builder
	prevEnd := 0
	for _, p := range got {
		if p.Start != prevEnd {
			t.Fatalf("passage %d starts at %d, previous ended at %d — %s",
				p.Ordinal, p.Start, prevEnd,
				map[bool]string{true: "gap", false: "overlap"}[p.Start > prevEnd])
		}
		if p.Body != string(content[p.Start:p.End]) {
			t.Fatalf("passage %d body does not match its byte span", p.Ordinal)
		}
		b.WriteString(p.Body)
		prevEnd = p.End
	}
	if prevEnd != len(content) {
		t.Fatalf("passages end at %d, content is %d bytes — the tail is unreachable", prevEnd, len(content))
	}
	if b.String() != string(content) {
		t.Fatal("concatenated passages do not reproduce the input")
	}
}

func splitterFixtures() map[string]string {
	return map[string]string{
		"empty-ish":       "\n",
		"no-headings":     strings.Repeat("Just prose with no structure at all. ", 200),
		"single-heading":  "# Only\n\nbody\n",
		"preamble":        "intro\n\n# One\n\nbody\n",
		"trailing-blanks": "# One\n\nbody\n\n\n\n",
		"no-trailing-nl":  "# One\n\nbody without a trailing newline",
		"crlf":            "# One\r\n\r\nbody\r\n",
		"frontmatter":     "---\ntitle: X\n---\n\n# One\n\nbody\n",
		"fence":           "# One\n\n```go\nfunc main() {}\n```\n\nafter\n",
		"nested-fence":    "# One\n\n````md\n```go\ncode\n```\n````\n\nafter\n",
		"consecutive-h":   "# A\n## B\n### C\n\nbody\n",
		"huge-paragraph":  "# Big\n\n" + strings.Repeat("word ", 3000),
		"huge-fence":      "# Big\n\n```\n" + strings.Repeat("code line\n", 1500) + "```\n",
		"unicode":         "# 日本語\n\n" + strings.Repeat("テキストの段落。", 500),
		"tabs-and-spaces": "#\tTabbed\n\n\tindented block\n\nbody\n",
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 6; i++ {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate repository root")
	return ""
}
