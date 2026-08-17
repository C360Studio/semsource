package e2e

import (
	"strings"
	"testing"
)

func TestExtractQuickstartBlocks_MarkedAndUnmarked(t *testing.T) {
	doc := "# Quickstart\n" +
		"\n" +
		"Prose example, never executed:\n" +
		"\n" +
		"```bash\n" +
		"echo prose-only\n" +
		"```\n" +
		"\n" +
		"## Get the repo\n" +
		"\n" +
		"```bash quickstart:single\n" +
		"git clone https://example.com/repo.git\n" +
		"cd repo\n" +
		"```\n" +
		"\n" +
		"## Start\n" +
		"\n" +
		"```bash quickstart:single\n" +
		"semsource run\n" +
		"```\n"

	blocks, err := ExtractQuickstartBlocks(doc, "single")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2: %+v", len(blocks), blocks)
	}
	if blocks[0].Heading != "Get the repo" {
		t.Errorf("block 0 heading = %q, want 'Get the repo'", blocks[0].Heading)
	}
	if want := "git clone https://example.com/repo.git\ncd repo"; blocks[0].Script != want {
		t.Errorf("block 0 script = %q, want %q", blocks[0].Script, want)
	}
	if blocks[0].Line != 11 {
		t.Errorf("block 0 line = %d, want 11", blocks[0].Line)
	}
	if blocks[1].Heading != "Start" || blocks[1].Script != "semsource run" {
		t.Errorf("block 1 = %+v", blocks[1])
	}
}

func TestExtractQuickstartBlocks_MultiTrackMembership(t *testing.T) {
	doc := "## Shared\n" +
		"```bash quickstart:single,multi\n" +
		"curl -sf http://localhost:8080/source-manifest/status\n" +
		"```\n" +
		"## Multi only\n" +
		"```bash quickstart:multi\n" +
		"semsource validate\n" +
		"```\n"

	single, err := ExtractQuickstartBlocks(doc, "single")
	if err != nil {
		t.Fatalf("single: %v", err)
	}
	if len(single) != 1 || single[0].Heading != "Shared" {
		t.Fatalf("single track = %+v, want just the shared block", single)
	}

	multi, err := ExtractQuickstartBlocks(doc, "multi")
	if err != nil {
		t.Fatalf("multi: %v", err)
	}
	if len(multi) != 2 {
		t.Fatalf("multi track = %d blocks, want 2", len(multi))
	}
	if !multi[0].InTrack("single") || multi[1].InTrack("single") {
		t.Errorf("track membership wrong: %+v", multi)
	}
}

func TestExtractQuickstartBlocks_MalformedMarkersAreLoud(t *testing.T) {
	cases := map[string]string{
		"empty track list": "```bash quickstart:\nx\n```\n",
		"bare word":        "```bash quickstart\nx\n```\n",
		"unknown track":    "```bash quickstart:signle\nx\n```\n",
		"trailing comma":   "```bash quickstart:single,\nx\n```\n",
		"duplicate track":  "```bash quickstart:single,single\nx\n```\n",
		"double marker":    "```bash quickstart:single quickstart:multi\nx\n```\n",
		"unterminated":     "```bash quickstart:single\nx\n",
	}
	for name, doc := range cases {
		if _, err := ExtractQuickstartBlocks(doc, "single"); err == nil {
			t.Errorf("%s: expected a loud error, got none", name)
		}
	}
}

func TestExtractQuickstartBlocks_UnknownTrackRequested(t *testing.T) {
	if _, err := ExtractQuickstartBlocks("", "nope"); err == nil {
		t.Fatal("expected error for unknown requested track")
	}
}

func TestExtractQuickstartBlocks_NestedFencesStayInBody(t *testing.T) {
	// A four-backtick fence may contain a three-backtick run as body.
	doc := "## Docs\n" +
		"````bash quickstart:multi\n" +
		"cat > f.md <<'EOF'\n" +
		"```\n" +
		"inner\n" +
		"```\n" +
		"EOF\n" +
		"````\n"
	blocks, err := ExtractQuickstartBlocks(doc, "multi")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if !strings.Contains(blocks[0].Script, "inner") || !strings.Contains(blocks[0].Script, "```") {
		t.Errorf("nested fence lost from body: %q", blocks[0].Script)
	}
}

func TestExtractQuickstartBlocks_HeadingBeforeFirst(t *testing.T) {
	doc := "```bash quickstart:single\nx\n```\n"
	blocks, err := ExtractQuickstartBlocks(doc, "single")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if blocks[0].Heading != "(before first heading)" {
		t.Errorf("heading = %q", blocks[0].Heading)
	}
}
