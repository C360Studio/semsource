package doc

import (
	"strings"
	"testing"
)

// A fenced block competes with the prose around it, not only with its own
// entries. X02 is the case the homogeneous path cannot reach: a two-line
// `docker run` block yields too few key groups to qualify, and its section sits
// under the ceiling so no size path runs either. As prose accumulated around it
// the section's cosine fell 0.6481 -> 0.6386 -> 0.6116 against a distractor
// holding at 0.6407, and the graded answer went from rank 0 to rank 4.
//
// Isolating the block scored 0.6535 — a losing margin of -0.0188 becoming a
// winning +0.0231. These tests pin that trigger and, more importantly, pin what
// it must NOT do.
func TestIsolateFencedBlocks(t *testing.T) {
	prose := strings.Repeat("Explanatory prose that dilutes the block beside it. ", 12) // ~600 B

	tests := []struct {
		name      string
		content   string
		wantFence string
		why       string
	}{
		{
			name:      "fence is isolated from substantial prose",
			content:   "## Tier 2\n\n" + prose + "\n\n```bash\ndocker run -d -p 8083:8083 img\nsemsource run\n```\n",
			wantFence: "docker run -d -p 8083:8083 img\nsemsource run",
			why:       "the command must not share a passage with 600 B of prose",
		},
		{
			name:      "fence stays with prose below the floor",
			content:   "## Tiny\n\nShort note.\n\n```bash\ndocker run -d img\n```\n",
			wantFence: "",
			why:       "nothing substantial to dilute it; isolating would mint a tiny passage for nothing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := splitPassages([]byte(tt.content), formatMarkdown)
			if tt.wantFence == "" {
				if len(ps) != 1 {
					t.Fatalf("%s: got %d passages, want 1 — %s", tt.name, len(ps), tt.why)
				}
				return
			}
			var isolated bool
			for _, p := range ps {
				body := strings.TrimSpace(p.Body)
				if strings.HasPrefix(body, "```") && strings.Contains(body, tt.wantFence) {
					isolated = true
					if strings.Contains(body, "Explanatory prose") {
						t.Errorf("isolated passage still carries prose: %q", body)
					}
				}
			}
			if !isolated {
				t.Errorf("%s: no passage is the fence alone — %s\ngot: %#v", tt.name, tt.why, bodies(ps))
			}
		})
	}
}

// TestIsolateFencedBlocks_NeverDividesABlock is the guard that matters most.
// Isolation decides where a block's passage BEGINS and ENDS; it must never cut
// inside one. A Go function body or a JSON document split across passages would
// produce individually meaningless fragments — strictly worse than the dilution
// this change exists to fix.
func TestIsolateFencedBlocks_NeverDividesABlock(t *testing.T) {
	prose := strings.Repeat("Surrounding prose that is comfortably above the floor. ", 12)

	continuous := []struct{ name, block string }{
		{"go function body", "```go\nfunc Handle(w http.ResponseWriter, r *http.Request) {\n\tif r.Method != \"GET\" {\n\t\thttp.Error(w, \"no\", 405)\n\t\treturn\n\t}\n\tfmt.Fprintln(w, \"ok\")\n}\n```"},
		{"json document", "```json\n{\n  \"namespace\": \"c360\",\n  \"sources\": [\n    {\"type\": \"ast\", \"path\": \"/workspace\"}\n  ]\n}\n```"},
		{"multi-line pipeline", "```bash\ndocker compose config --services \\\n  | sort \\\n  | grep -v ui\n```"},
		{"here-document", "```bash\ncat > out.txt <<'EOF'\nline one\nline two\nEOF\n```"},
	}

	for _, c := range continuous {
		t.Run(c.name, func(t *testing.T) {
			content := "## Section\n\n" + prose + "\n\n" + c.block + "\n"
			ps := splitPassages([]byte(content), formatMarkdown)

			var fenceParts int
			for _, p := range ps {
				if strings.Contains(p.Body, "```") {
					fenceParts++
				}
			}
			if fenceParts != 1 {
				t.Errorf("%s: fenced block appears in %d passages; a block must never be divided\n%#v",
					c.name, fenceParts, bodies(ps))
			}
			// And the block must survive intact, not merely unsplit.
			var whole bool
			for _, p := range ps {
				if strings.Contains(p.Body, c.block) {
					whole = true
				}
			}
			if !whole {
				t.Errorf("%s: block was altered or cut; it must appear verbatim in one passage", c.name)
			}
		})
	}
}

// TestIsolateFencedBlocks_TilesExactly keeps the byte-for-byte tiling contract
// under the new path: isolation reshapes boundaries, so it is exactly the kind of
// change that can silently drop or duplicate text.
func TestIsolateFencedBlocks_TilesExactly(t *testing.T) {
	prose := strings.Repeat("Prose above the floor so isolation triggers. ", 15)
	content := "## One\n\n" + prose + "\n\n```bash\ndocker run -d -p 8083:8083 img\n```\n\nTrailing prose after the block.\n"

	ps := splitPassages([]byte(content), formatMarkdown)
	var rebuilt strings.Builder
	for _, p := range ps {
		rebuilt.WriteString(content[p.Start:p.End])
	}
	if rebuilt.String() != content {
		t.Errorf("passages do not tile the document exactly\n got %q\nwant %q", rebuilt.String(), content)
	}
}

func bodies(ps []passage) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		b := p.Body
		if len(b) > 60 {
			b = b[:60] + "…"
		}
		out = append(out, b)
	}
	return out
}
