package doc_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler"
	dochandler "github.com/c360studio/semsource/handler/doc"
	source "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/message"
)

// ---------------------------------------------------------------------------
// Passage fixtures and helpers
//
// A document now yields a parent entity plus one entity per passage. These
// tests pin the passage contract: identity derived from (path, ordinal),
// containment back to the parent, and the section/title facets a retrieval
// result is read through.
// ---------------------------------------------------------------------------

// ingestDocs runs h over dir under the "acme" org, failing the test on error.
// It is the preamble every passage test shares.
func ingestDocs(t *testing.T, h *dochandler.Handler, dir string) []*handler.EntityState {
	t.Helper()
	states, err := h.IngestEntityStates(context.Background(), sourceConfig{typ: "docs", path: dir}, "acme")
	if err != nil {
		t.Fatalf("IngestEntityStates() error: %v", err)
	}
	return states
}

// prose returns paragraphs of filler wording tagged with tag, so no two sections
// of the same shape share a content hash. The paragraph count is chosen against
// the splitter's bounds: four paragraphs clear the merge floor (the section is
// not folded into its neighbour) while staying under the ceiling (it is not
// subdivided), so a fixture's section count is its passage count.
func prose(tag string, paragraphs int) string {
	var b strings.Builder
	for i := 1; i <= paragraphs; i++ {
		fmt.Fprintf(&b, "Paragraph %d of the %s material, written at enough length that its section "+
			"clears the splitter's merge floor and stands as a passage of its own.\n\n", i, tag)
	}
	return b.String()
}

// headedSection returns a markdown H2 section whose body is filler unique to tag.
func headedSection(heading, tag string, paragraphs int) string {
	return "## " + heading + "\n\n" + prose(tag, paragraphs)
}

// parentByFile indexes parent states by their source.DocFilePath.
func parentByFile(t *testing.T, states []*handler.EntityState) map[string]*handler.EntityState {
	t.Helper()
	out := make(map[string]*handler.EntityState)
	for _, state := range parentStates(states) {
		path := tripleValue(t, state, source.DocFilePath)
		if prev, dup := out[path]; dup {
			t.Fatalf("two parent states carry %s %q: %s and %s; a file must yield exactly one parent",
				source.DocFilePath, path, prev.ID, state.ID)
		}
		out[path] = state
	}
	return out
}

// passagesByFile groups passage states by source.DocFilePath, preserving the
// emission (ordinal) order within each file.
func passagesByFile(t *testing.T, states []*handler.EntityState) map[string][]*handler.EntityState {
	t.Helper()
	out := make(map[string][]*handler.EntityState)
	for _, state := range passageStates(states) {
		path := tripleValue(t, state, source.DocFilePath)
		out[path] = append(out[path], state)
	}
	return out
}

// allDigits reports whether s is non-empty and made only of ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Parent / passage split
// ---------------------------------------------------------------------------

// TestDocHandler_Passages_ParentChunkCountMatchesPassages pins the signal the
// staleness pass reads: one file yields exactly one parent, and that parent's
// DocChunkCount is the number of passages it currently has. If the count and
// the emitted passages ever disagree, the retraction rule ("a passage at or
// above the parent's count no longer exists") either retracts live passages or
// leaves dead ones serving as current.
func TestDocHandler_Passages_ParentChunkCountMatchesPassages(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "guide.md", "# Guide\n\n"+prose("intro", 4)+
		headedSection("Usage", "usage", 4)+
		headedSection("Limits", "limits", 4))

	h, _ := docsHandler(t)
	states := ingestDocs(t, h, dir)

	parents := parentStates(states)
	if len(parents) != 1 {
		t.Fatalf("parent state count: got %d, want 1 (guide.md); %d states in total", len(parents), len(states))
	}
	passages := passageStates(states)
	if len(passages) < 2 {
		t.Fatalf("passage state count: got %d, want at least 2 for a three-section document; %d states in total",
			len(passages), len(states))
	}
	if got := len(parents) + len(passages); got != len(states) {
		t.Errorf("classified %d of %d states; every state must be a document or a passage", got, len(states))
	}

	if got := tripleInt(t, parents[0], source.DocChunkCount); got != len(passages) {
		t.Errorf("%s on %s = %d, want %d (the number of passage states emitted for guide.md)",
			source.DocChunkCount, parents[0].ID, got, len(passages))
	}
}

// ---------------------------------------------------------------------------
// Passage identity
// ---------------------------------------------------------------------------

// TestDocHandler_Passages_IDShape pins the passage ID: a 6-part ID whose type
// segment is "chunk" and whose instance ends in the zero-padded ordinal, with
// ordinals contiguous from 0. The padding is what keeps passage 10 from sorting
// before passage 2, and contiguity is what makes the DocChunkCount comparison a
// plain integer test.
func TestDocHandler_Passages_IDShape(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "guide.md", "# Guide\n\n"+prose("intro", 4)+
		headedSection("Usage", "usage", 4)+
		headedSection("Limits", "limits", 4))

	h, _ := docsHandler(t)
	passages := passageStates(ingestDocs(t, h, dir))
	if len(passages) < 2 {
		t.Fatalf("passage state count: got %d, want at least 2 for a three-section document", len(passages))
	}

	seen := make(map[string]bool, len(passages))
	for i, passage := range passages {
		parts := strings.Split(passage.ID, ".")
		if len(parts) != 6 {
			t.Fatalf("passage ID %q has %d parts, want 6", passage.ID, len(parts))
		}
		if parts[4] != "chunk" {
			t.Errorf("passage ID type segment = %q, want %q (ID %q)", parts[4], "chunk", passage.ID)
		}

		instance := parts[5]
		if len(instance) < 5 || instance[len(instance)-5] != '-' {
			t.Fatalf("passage instance %q does not end in %q, want a 4-digit ordinal after a hyphen (ID %q)",
				instance, "-NNNN", passage.ID)
		}
		ordinal := instance[len(instance)-4:]
		if !allDigits(ordinal) {
			t.Errorf("passage instance ordinal suffix = %q, want 4 digits (ID %q)", ordinal, passage.ID)
		}
		if want := fmt.Sprintf("%04d", i); ordinal != want {
			t.Errorf("passage %d instance ordinal suffix = %q, want %q (ordinals are contiguous from 0; ID %q)",
				i, ordinal, want, passage.ID)
		}
		if got := tripleInt(t, passage, source.DocChunkIndex); got != i {
			t.Errorf("%s on %s = %d, want %d (the index must match the emission ordinal)",
				source.DocChunkIndex, passage.ID, got, i)
		}
		if seen[passage.ID] {
			t.Errorf("duplicate passage ID %q at position %d", passage.ID, i)
		}
		seen[passage.ID] = true
	}
}

// TestDocHandler_Passages_IDsAreDeterministic pins the property the whole
// substrate rests on: re-ingesting an unchanged document reproduces byte-
// identical passage IDs, so a re-scan replaces facts in place instead of
// minting a fresh set of orphaned siblings every pass.
func TestDocHandler_Passages_IDsAreDeterministic(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "guide.md", "# Guide\n\n"+prose("intro", 4)+
		headedSection("Usage", "usage", 4)+
		headedSection("Limits", "limits", 4))

	h, _ := docsHandler(t)
	first := passageStates(ingestDocs(t, h, dir))
	second := passageStates(ingestDocs(t, h, dir))

	if len(first) == 0 {
		t.Fatal("passage state count on the first ingest: got 0, want at least 1")
	}
	if len(first) != len(second) {
		t.Fatalf("passage count changed across two ingests of an unchanged document: %d then %d",
			len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Errorf("passage %d ID differs across two ingests of an unchanged document: %q then %q",
				i, first[i].ID, second[i].ID)
		}
	}
}

// TestDocHandler_Passages_RepeatedHeadingsGetDistinctIDs pins the case a
// heading-slug identity scheme collapses: two sections whose heading text is
// identical are two passages, and they must not share an ID — one would
// overwrite the other and half the document would vanish from retrieval.
func TestDocHandler_Passages_RepeatedHeadingsGetDistinctIDs(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "guide.md", "# Guide\n\n"+prose("intro", 4)+
		headedSection("Usage", "first-usage", 4)+
		headedSection("Usage", "second-usage", 4))

	h, _ := docsHandler(t)
	passages := passageStates(ingestDocs(t, h, dir))

	var usage []*handler.EntityState
	for _, passage := range passages {
		if section, ok := optionalTripleValue(passage, source.DocSection); ok && section == "Usage" {
			usage = append(usage, passage)
		}
	}
	if len(usage) != 2 {
		t.Fatalf("passages with %s = %q: got %d, want 2 (the document has two sections under that heading); %d passages in total",
			source.DocSection, "Usage", len(usage), len(passages))
	}
	if usage[0].ID == usage[1].ID {
		t.Errorf("the two %q sections collapsed onto one passage ID %q; passage identity must come from the ordinal, not the heading",
			"Usage", usage[0].ID)
	}
}

// TestDocHandler_Passages_HeadingRenameKeepsIDs is the direct statement of why
// identity is (path, ordinal) rather than a heading slug: editing a heading's
// text without moving it leaves every passage ID untouched, so the rename is an
// in-place fact update rather than a wholesale retract-and-recreate of the
// document's passages.
func TestDocHandler_Passages_HeadingRenameKeepsIDs(t *testing.T) {
	dir := t.TempDir()
	body := "# Guide\n\n" + prose("intro", 4) +
		headedSection("Usage", "usage", 4) +
		headedSection("Limits", "limits", 4)
	path := writeMD(t, dir, "guide.md", body)

	h, _ := docsHandler(t)
	before := passageStates(ingestDocs(t, h, dir))
	if len(before) < 2 {
		t.Fatalf("passage state count before the rename: got %d, want at least 2", len(before))
	}

	renamed := strings.Replace(body, "## Usage\n", "## Getting Started\n", 1)
	if renamed == body {
		t.Fatal("fixture error: the heading rename did not change the document")
	}
	if err := os.WriteFile(path, []byte(renamed), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	after := passageStates(ingestDocs(t, h, dir))
	if len(before) != len(after) {
		t.Fatalf("passage count changed on a heading rename: %d then %d (renaming a heading must not restructure the document)",
			len(before), len(after))
	}
	for i := range before {
		if before[i].ID != after[i].ID {
			t.Errorf("passage %d ID changed when only a heading's text changed: %q then %q; identity is (path, ordinal), not the heading slug",
				i, before[i].ID, after[i].ID)
		}
	}

	// Guard against a vacuous pass: the rename must actually have reached the
	// passage's facts, otherwise the IDs above are stable for the wrong reason.
	if got := tripleValue(t, before[1], source.DocSection); got != "Usage" {
		t.Fatalf("%s on passage 1 before the rename = %q, want %q", source.DocSection, got, "Usage")
	}
	if got := tripleValue(t, after[1], source.DocSection); got != "Getting Started" {
		t.Errorf("%s on passage 1 after the rename = %q, want %q (the new heading text must land on the same entity)",
			source.DocSection, got, "Getting Started")
	}
}

// ---------------------------------------------------------------------------
// Containment and facets
// ---------------------------------------------------------------------------

// TestDocHandler_Passages_BelongToTheirParent pins the containment edge: every
// passage carries exactly one CodeBelongs triple, pointing at ITS OWN parent as
// an entity reference rather than a literal. Two documents are ingested so that
// a handler wiring every passage to the first parent it built is caught.
func TestDocHandler_Passages_BelongToTheirParent(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "alpha.md", "# Alpha\n\n"+prose("alpha-intro", 4)+headedSection("Usage", "alpha-usage", 4))
	writeMD(t, dir, "beta.md", "# Beta\n\n"+prose("beta-intro", 4)+headedSection("Usage", "beta-usage", 4))

	h, _ := docsHandler(t)
	states := ingestDocs(t, h, dir)
	parents := parentByFile(t, states)
	grouped := passagesByFile(t, states)

	if len(parents) != 2 {
		t.Fatalf("parent state count: got %d, want 2 (alpha.md + beta.md)", len(parents))
	}
	for _, file := range []string{"alpha.md", "beta.md"} {
		parent, ok := parents[file]
		if !ok {
			t.Fatalf("no parent state for %q; parents: %v", file, parents)
		}
		passages := grouped[file]
		if len(passages) == 0 {
			t.Fatalf("passage state count for %q: got 0, want at least 1", file)
		}

		for _, passage := range passages {
			var belongs []message.Triple
			for _, tr := range passage.Triples {
				if tr.Predicate == source.CodeBelongs {
					belongs = append(belongs, tr)
				}
			}
			if len(belongs) != 1 {
				t.Fatalf("passage %s carries %d %s triples, want exactly 1",
					passage.ID, len(belongs), source.CodeBelongs)
			}
			object, ok := belongs[0].Object.(string)
			if !ok {
				t.Fatalf("passage %s %s object is %T (%v), want string",
					passage.ID, source.CodeBelongs, belongs[0].Object, belongs[0].Object)
			}
			if object != parent.ID {
				t.Errorf("passage %s %s = %q, want the parent of %q, which is %q",
					passage.ID, source.CodeBelongs, object, file, parent.ID)
			}
			if belongs[0].Datatype != message.EntityReferenceDatatype {
				t.Errorf("passage %s %s Datatype = %q, want %q (an entity reference, not a literal)",
					passage.ID, source.CodeBelongs, belongs[0].Datatype, message.EntityReferenceDatatype)
			}
		}
	}
}

// TestDocHandler_Passages_HeadinglessProseIsCovered pins the content that a
// heading-anchored splitter loses: prose sitting above a document's first
// heading is still a passage, still titled, and still carries its text. It is
// also the shape that makes heading-derived identity impossible — there is no
// heading to derive from.
func TestDocHandler_Passages_HeadinglessProseIsCovered(t *testing.T) {
	dir := t.TempDir()
	// No H1: the file opens straight into prose, so the first passage governs a
	// headingless span. The title falls back to the filename stem.
	writeMD(t, dir, "notes.md", prose("preamble", 4)+headedSection("Usage", "usage", 4))

	h, store := docsHandler(t)
	states := ingestDocs(t, h, dir)
	passages := passageStates(states)
	if len(passages) < 2 {
		t.Fatalf("passage state count: got %d, want at least 2 (headingless preamble + Usage section); %d states in total",
			len(passages), len(states))
	}

	first := passages[0]
	if got := passageBody(t, store, first); !strings.Contains(got, "preamble") {
		t.Errorf("first passage %s does not carry the prose above the first heading; its offloaded body is %q",
			first.ID, got)
	}
	title := tripleValue(t, first, source.DcTitle)
	if title == "" {
		t.Errorf("%s on the headingless passage %s is empty; a passage with no section must still be nameable in a result list",
			source.DcTitle, first.ID)
	}
	if parentTitle := tripleValue(t, parentStates(states)[0], source.DcTitle); !strings.Contains(title, parentTitle) {
		t.Errorf("%s on the headingless passage = %q, want it qualified by the parent title %q",
			source.DcTitle, title, parentTitle)
	}
}

// TestDocHandler_Passages_SectionPresence pins DocSection as a conditional
// facet: a headed passage carries the heading text, and a headingless one
// carries no DocSection triple at all rather than an empty-string one. An empty
// object would be indexed and ranked as if it were a real section name.
func TestDocHandler_Passages_SectionPresence(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "notes.md", prose("preamble", 4)+headedSection("Usage", "usage", 4))

	h, _ := docsHandler(t)
	passages := passageStates(ingestDocs(t, h, dir))
	if len(passages) < 2 {
		t.Fatalf("passage state count: got %d, want at least 2 (headingless preamble + Usage section)", len(passages))
	}

	if section, ok := optionalTripleValue(passages[0], source.DocSection); ok {
		t.Errorf("headingless passage %s carries a %s triple with object %q; the triple must be absent entirely",
			passages[0].ID, source.DocSection, section)
	}
	if got := tripleValue(t, passages[1], source.DocSection); got != "Usage" {
		t.Errorf("%s on the headed passage %s = %q, want %q (the heading text)",
			source.DocSection, passages[1].ID, got, "Usage")
	}
}

// TestDocHandler_Passages_TitlesAreQualifiedPerDocument pins the retrieval
// ergonomics: a heading like "Usage" recurs across a corpus, so a passage title
// is qualified by its parent's. Two documents sharing a heading must not
// produce two identically-titled results.
func TestDocHandler_Passages_TitlesAreQualifiedPerDocument(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "alpha.md", "# Alpha\n\n"+prose("alpha-intro", 4)+headedSection("Usage", "alpha-usage", 4))
	writeMD(t, dir, "beta.md", "# Beta\n\n"+prose("beta-intro", 4)+headedSection("Usage", "beta-usage", 4))

	h, _ := docsHandler(t)
	states := ingestDocs(t, h, dir)
	parents := parentByFile(t, states)
	grouped := passagesByFile(t, states)

	titles := make(map[string]string, 2) // file -> title of its "Usage" passage
	for _, file := range []string{"alpha.md", "beta.md"} {
		var found *handler.EntityState
		for _, passage := range grouped[file] {
			if section, ok := optionalTripleValue(passage, source.DocSection); ok && section == "Usage" {
				found = passage
				break
			}
		}
		if found == nil {
			t.Fatalf("no passage with %s = %q for %q; %d passages for that file",
				source.DocSection, "Usage", file, len(grouped[file]))
		}

		title := tripleValue(t, found, source.DcTitle)
		parentTitle := tripleValue(t, parents[file], source.DcTitle)
		if !strings.Contains(title, parentTitle) {
			t.Errorf("%s on %s = %q, want it qualified by the parent title %q",
				source.DcTitle, found.ID, title, parentTitle)
		}
		if !strings.Contains(title, "Usage") {
			t.Errorf("%s on %s = %q, want it to name the section %q", source.DcTitle, found.ID, title, "Usage")
		}
		titles[file] = title
	}

	if titles["alpha.md"] == titles["beta.md"] {
		t.Errorf("the %q passages of alpha.md and beta.md share the title %q; passages from different documents must be distinguishable",
			"Usage", titles["alpha.md"])
	}
}

// ---------------------------------------------------------------------------
// Size bounds
// ---------------------------------------------------------------------------

// TestDocHandler_Passages_LargeDocumentSplitsUnderHardMax is the reason
// passages exist: the substrate embeds one vector per entity from text truncated
// at 8000 characters, so a document past that cap was being silently unindexed
// beyond the cut. A document well over the cap must split into several
// passages, each offloaded to its own blob and each under the hard max.
func TestDocHandler_Passages_LargeDocumentSplitsUnderHardMax(t *testing.T) {
	const sections = 8

	dir := t.TempDir()
	var body strings.Builder
	body.WriteString("# Large Guide\n\n")
	for i := 1; i <= sections; i++ {
		body.WriteString(headedSection(fmt.Sprintf("Section %d", i), fmt.Sprintf("section-%d", i), 8))
	}
	content := body.String()
	if len(content) <= 8000 {
		t.Fatalf("fixture error: document is %d bytes, want well over the 8000-character embedding cap", len(content))
	}
	writeMD(t, dir, "large.md", content)

	h, store := docsHandler(t)
	states := ingestDocs(t, h, dir)
	passages := passageStates(states)
	if len(passages) < 2 {
		t.Fatalf("passage state count for a %d-byte document: got %d, want at least 2 (a document over the cap must split)",
			len(content), len(passages))
	}

	keys := make(map[string]string, len(passages)) // body key -> first passage ID that claimed it
	var joined strings.Builder
	for _, passage := range passages {
		key := assertOffloadedBody(t, passage)
		if prev, dup := keys[key]; dup {
			t.Errorf("passages %s and %s share the body key %q; each passage must have its own body blob",
				prev, passage.ID, key)
		}
		keys[key] = passage.ID

		blob, err := store.Get(context.Background(), key)
		if err != nil {
			t.Fatalf("passage %s body key %q not in the store: %v", passage.ID, key, err)
		}
		if len(blob) > dochandler.PassageHardMax {
			t.Errorf("passage %s body is %d bytes, want at most %d (passageHardMax); anything larger is silently truncated at embedding time",
				passage.ID, len(blob), dochandler.PassageHardMax)
		}
		joined.Write(blob)
	}

	// Splitting must not lose any of the document it was meant to make reachable.
	if joined.String() != content {
		t.Errorf("passage bodies concatenated to %d bytes, want the whole %d-byte document",
			joined.Len(), len(content))
	}
}

// TestDocHandler_Passages_TitleNotDuplicatedWhenH1EqualsTitle pins the cosmetic
// defect where a document whose H1 repeats its own title produced a passage
// named "CLAUDE.md § CLAUDE.md". Qualifying a section with its parent exists to
// disambiguate six "Usage" sections across six documents; when the section IS
// the document, the qualifier adds nothing and the repetition reads as a bug in
// every result list the passage appears in.
func TestDocHandler_Passages_TitleNotDuplicatedWhenH1EqualsTitle(t *testing.T) {
	dir := t.TempDir()
	// The real shape from this repository: a file whose H1 is its own filename,
	// so the derived document title and the first section heading are identical.
	writeMD(t, dir, "CLAUDE.md", "# CLAUDE.md\n\n"+prose("guidance", 4)+headedSection("Testing", "testing", 4))

	h, _ := docsHandler(t)
	states := ingestDocs(t, h, dir)

	parentTitle := tripleValue(t, parentStates(states)[0], source.DcTitle)
	for _, p := range passageStates(states) {
		title := tripleValue(t, p, source.DcTitle)
		if title == parentTitle+" § "+parentTitle {
			t.Errorf("passage %s title = %q; a section identical to its document title must not "+
				"be qualified with itself", p.ID, title)
		}
	}

	// The qualifier must survive where it still earns its place: a genuinely
	// different section is still disambiguated by its parent.
	var sawQualified bool
	for _, p := range passageStates(states) {
		if tripleValue(t, p, source.DcTitle) == parentTitle+" § Testing" {
			sawQualified = true
		}
	}
	if !sawQualified {
		t.Errorf("no passage carries the qualified title %q; suppressing the duplicate must not "+
			"disable qualification for distinct sections", parentTitle+" § Testing")
	}
}
