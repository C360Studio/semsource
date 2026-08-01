package doc

import (
	"strings"
	"testing"
)

// bodyOf builds a document with the given heading markers, padded so each
// section is substantial enough to survive the below-floor merge.
func bodyOf(h1, h2, h3 string) []byte {
	pad := strings.Repeat("This clause defines normative requirements for the resource and its encodings. ", 40)
	var b strings.Builder
	b.WriteString(h1 + " Connected Systems API\n\n" + pad + "\n\n")
	b.WriteString(h2 + " Feature Resources\n\n" + pad + "\n\n")
	b.WriteString(h3 + " Datastreams\n\n" + pad + "\n\n")
	b.WriteString(h2 + " Dynamic Data\n\n" + pad + "\n")
	return []byte(b.String())
}

// TestAsciiDocPassagesCarryHeadings is the regression this change exists for.
// Before it, every .adoc passage carried Heading:"" and HeadingPath:[] — the
// heading-path identity that IS the passage-dilution fix was inert for AsciiDoc,
// so specification corpora (the OGC Connected Systems API among them) produced
// anonymous passages.
func TestAsciiDocPassagesCarryHeadings(t *testing.T) {
	ps := splitPassages(bodyOf("=", "==", "==="), formatASCIIDoc)
	if len(ps) == 0 {
		t.Fatal("no passages")
	}
	for i, p := range ps {
		if p.Heading == "" {
			t.Errorf("passage %d has no heading: %+v", i, p)
		}
		if len(p.HeadingPath) == 0 {
			t.Errorf("passage %d has no heading path: %+v", i, p)
		}
	}
}

// TestEquivalentDocumentsAgreeAcrossFormats pins the cross-format contract: the
// same content expressed in markdown and AsciiDoc must produce the same heading
// ancestry. This is what the marker-count level mapping buys — AsciiDoc's own
// numbering calls "= Title" level 0, which would shift every AsciiDoc passage
// one level relative to its markdown twin and make the ancestries disagree.
func TestEquivalentDocumentsAgreeAcrossFormats(t *testing.T) {
	md := splitPassages(bodyOf("#", "##", "###"), formatMarkdown)
	ad := splitPassages(bodyOf("=", "==", "==="), formatASCIIDoc)

	if len(md) != len(ad) {
		t.Fatalf("passage counts differ: markdown=%d asciidoc=%d", len(md), len(ad))
	}
	for i := range md {
		if md[i].Heading != ad[i].Heading {
			t.Errorf("passage %d heading: markdown=%q asciidoc=%q", i, md[i].Heading, ad[i].Heading)
		}
		if strings.Join(md[i].HeadingPath, " > ") != strings.Join(ad[i].HeadingPath, " > ") {
			t.Errorf("passage %d path: markdown=%v asciidoc=%v", i, md[i].HeadingPath, ad[i].HeadingPath)
		}
	}
}

// TestAsciiDocAncestryIsOutermostFirst checks the nesting order at three levels,
// since the ancestry is the identity text and its order is load-bearing.
func TestAsciiDocAncestryIsOutermostFirst(t *testing.T) {
	ps := splitPassages(bodyOf("=", "==", "==="), formatASCIIDoc)

	var deepest []string
	for _, p := range ps {
		if p.Heading == "Datastreams" && len(p.HeadingPath) > len(deepest) {
			deepest = p.HeadingPath
		}
	}
	want := []string{"Connected Systems API", "Feature Resources", "Datastreams"}
	if strings.Join(deepest, " > ") != strings.Join(want, " > ") {
		t.Errorf("ancestry = %v, want %v", deepest, want)
	}
}

// TestSetextMarkdownIsNotReadAsAsciiDoc pins the risk that motivated taking the
// format from the caller instead of sniffing the bytes: markdown underlines a
// setext heading with "=", the same character AsciiDoc prefixes titles with. A
// sniffer could classify this document as AsciiDoc and lose its headings.
func TestSetextMarkdownIsNotReadAsAsciiDoc(t *testing.T) {
	pad := strings.Repeat("Prose that is long enough to keep the section above the merge floor. ", 40)
	doc := []byte("Connected Systems API\n=====================\n\n" + pad + "\n\nFeature Resources\n-----------------\n\n" + pad + "\n")

	ps := splitPassages(doc, formatMarkdown)
	if len(ps) == 0 {
		t.Fatal("no passages")
	}
	found := false
	for _, p := range ps {
		if p.Heading == "Connected Systems API" {
			found = true
		}
	}
	if !found {
		t.Errorf("setext H1 was not recognized as a markdown heading: %+v", ps)
	}
}

// TestAsciiDocHeadingInFencedBlockIsNotAHeading: the "=" prefix inside a fenced
// or literal block is content, not structure. Fence protection already exists
// for markdown; the AsciiDoc path must reuse it rather than rescan raw bytes.
func TestAsciiDocHeadingInFencedBlockIsNotAHeading(t *testing.T) {
	pad := strings.Repeat("Normative prose about the resource encoding and its constraints. ", 40)
	doc := []byte("= Real Title\n\n" + pad + "\n\n```\n== Not A Heading\n```\n\n" + pad + "\n")

	for _, p := range splitPassages(doc, formatASCIIDoc) {
		for _, h := range p.HeadingPath {
			if h == "Not A Heading" {
				t.Errorf("fenced line was treated as a heading: %+v", p)
			}
		}
	}
}

// TestAsciiDocRequiresSpaceAfterMarkers: AsciiDoc requires the space, so a line
// like "=Foo" or a bare "=" is not a section title.
func TestAsciiDocRequiresSpaceAfterMarkers(t *testing.T) {
	lines := scanLines([]byte("=NotATitle\n"))
	if _, _, ok := adocHeading(lines, 0); ok {
		t.Error("=NotATitle was accepted as a heading")
	}
	lines = scanLines([]byte("=\n"))
	if _, _, ok := adocHeading(lines, 0); ok {
		t.Error("a bare = was accepted as a heading")
	}
}

// TestPlainTextStillProducesAnonymousPassages: a format with no heading syntax
// yields passages with no heading, and that is a valid state rather than an
// error. .txt keeps reading as markdown, so this also pins that defaulting
// choice — a text file with no "#" lines is unaffected either way.
func TestPlainTextStillProducesAnonymousPassages(t *testing.T) {
	pad := strings.Repeat("Just prose with no structural markers of any kind at all. ", 60)
	ps := splitPassages([]byte(pad), formatMarkdown)
	if len(ps) == 0 {
		t.Fatal("no passages produced for headingless text")
	}
	for i, p := range ps {
		if p.Heading != "" {
			t.Errorf("passage %d invented a heading %q", i, p.Heading)
		}
	}
}

// TestFormatForExt pins the extension mapping, including that .txt and unknown
// extensions stay on the markdown path so existing corpora are unchanged.
func TestFormatForExt(t *testing.T) {
	for _, tc := range []struct {
		ext  string
		want docFormat
	}{
		{".adoc", formatASCIIDoc},
		{".ADOC", formatASCIIDoc},
		{".md", formatMarkdown},
		{".mdx", formatMarkdown},
		{".txt", formatMarkdown},
		{".rst", formatMarkdown},
		{"", formatMarkdown},
	} {
		if got := formatForExt(tc.ext); got != tc.want {
			t.Errorf("formatForExt(%q) = %v, want %v", tc.ext, got, tc.want)
		}
	}
}

// TestExtractTitleFollowsFormat: an AsciiDoc document titled "= Connected
// Systems API" previously fell back to its filename, because the title scan
// looked for "# " only.
func TestExtractTitleFollowsFormat(t *testing.T) {
	adoc := []byte("= Connected Systems API\n\nbody\n")
	if got := extractTitle(adoc, "cs-api.adoc", formatASCIIDoc); got != "Connected Systems API" {
		t.Errorf("asciidoc title = %q, want %q", got, "Connected Systems API")
	}
	md := []byte("# Connected Systems API\n\nbody\n")
	if got := extractTitle(md, "cs-api.md", formatMarkdown); got != "Connected Systems API" {
		t.Errorf("markdown title = %q, want %q", got, "Connected Systems API")
	}
	// A markdown document read as markdown must not pick up an AsciiDoc title.
	if got := extractTitle(adoc, "cs-api.md", formatMarkdown); got != "cs-api" {
		t.Errorf("markdown fallback = %q, want filename stem", got)
	}
}
