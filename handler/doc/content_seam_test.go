package doc_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/c360studio/semsource/entityid"
	"github.com/c360studio/semsource/handler"
	dochandler "github.com/c360studio/semsource/handler/doc"
	source "github.com/c360studio/semsource/source/vocabulary"
)

// seamProject is the explicit project override both callers are built with, so
// the system segment is a known constant instead of a temp-dir-derived slug the
// content caller has no way to reproduce.
const seamProject = "fixtures"

// seamHandler builds a doc handler with the body store every ingest requires and
// an explicit project, so the file walk and the content seam agree on the system
// segment without either one deriving it from a path.
func seamHandler(t *testing.T) *dochandler.Handler {
	t.Helper()
	return dochandler.New(
		dochandler.WithBodyStore(newMemStore(), bodyStoreInstance),
		dochandler.WithProject(seamProject),
	)
}

// writeDocAt writes content at a slash-delimited path under dir, creating parent
// directories. writeMD cannot be reused: it joins a bare name and does not
// create intermediate directories, and the nested cases are the interesting ones.
func writeDocAt(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// comparableState is an entity state reduced to the parts both callers must
// agree on. Timestamps are dropped: the two calls happen at different instants
// by construction, and every triple carries one.
type comparableState struct {
	id      string
	profile string
	triples []string
}

func comparableStates(states []*handler.EntityState) []comparableState {
	out := make([]comparableState, 0, len(states))
	for _, state := range states {
		triples := make([]string, 0, len(state.Triples))
		for _, tr := range state.Triples {
			triples = append(triples,
				fmt.Sprintf("%s|%v|%s|%.3f", tr.Predicate, tr.Object, tr.Source, tr.Confidence))
		}
		sort.Strings(triples)
		out = append(out, comparableState{id: state.ID, profile: state.IndexingProfile, triples: triples})
	}
	return out
}

var seamFixtures = []struct {
	name    string
	relPath string
	content string
}{
	{"markdown at the root", "readme.md", "# Hello\n\nContent here.\n"},
	{"markdown nested", "docs/guide/intro.md", "# Intro\n\nFirst section.\n\n## Deeper\n\nMore prose.\n"},
	{"plain text", "notes.txt", "Plain text, no headings at all.\n"},
	{"asciidoc", "spec.adoc", "= Connected Systems\n\nSome body text.\n"},
	{"nested with no heading", "docs/guide/untitled.md", "Body with no heading line.\n"},
}

// TestContentSeam_MatchesFileIngest is the drift guard between the two callers of
// the document entity builder: the filesystem walk and the content seam an object
// store will use. Identical bytes at an identical logical path must produce
// identical entities — same IDs, same triples, same indexing profile, same
// content-addressed body keys.
//
// Body equality is covered by the triple comparison rather than separately: the
// body-key triple carries a hash of the passage body, so two passages agreeing on
// their key agree on their bytes.
func TestContentSeam_MatchesFileIngest(t *testing.T) {
	system := entityid.SystemSlug(seamProject)

	for _, fx := range seamFixtures {
		t.Run(fx.name, func(t *testing.T) {
			dir := t.TempDir()
			writeDocAt(t, dir, fx.relPath, fx.content)

			h := seamHandler(t)
			fileStates, err := h.IngestEntityStates(context.Background(),
				sourceConfig{typ: "docs", path: dir}, "acme")
			if err != nil {
				t.Fatalf("IngestEntityStates() error: %v", err)
			}

			contentStates, err := h.IngestContentEntityStates(context.Background(),
				[]byte(fx.content), fx.relPath, system, "acme", time.Now().UTC())
			if err != nil {
				t.Fatalf("IngestContentEntityStates() error: %v", err)
			}

			if len(fileStates) == 0 {
				t.Fatalf("file ingest produced no states for %q", fx.relPath)
			}
			gotFile, gotContent := comparableStates(fileStates), comparableStates(contentStates)
			if len(gotFile) != len(gotContent) {
				t.Fatalf("state count: file ingest %d, content seam %d", len(gotFile), len(gotContent))
			}
			for i := range gotFile {
				if gotFile[i].id != gotContent[i].id {
					t.Errorf("state %d ID: file %q, content %q", i, gotFile[i].id, gotContent[i].id)
				}
				if gotFile[i].profile != gotContent[i].profile {
					t.Errorf("state %d indexing profile: file %q, content %q",
						i, gotFile[i].profile, gotContent[i].profile)
				}
				if len(gotFile[i].triples) != len(gotContent[i].triples) {
					t.Fatalf("state %d triple count: file %d, content %d",
						i, len(gotFile[i].triples), len(gotContent[i].triples))
				}
				for j := range gotFile[i].triples {
					if gotFile[i].triples[j] != gotContent[i].triples[j] {
						t.Errorf("state %d triple %d:\n  file:    %s\n  content: %s",
							i, j, gotFile[i].triples[j], gotContent[i].triples[j])
					}
				}
			}
		})
	}
}

// TestContentSeam_DerivesFromSlashLogicalPath pins what the seam reads off a
// slash-delimited logical path: the file-path triple carries it verbatim, MIME
// comes from its extension, and the title fallback is its BASENAME minus the
// extension — not the whole path, which is what an implementation that forgot to
// take the base would produce.
//
// One honest limit: on a platform whose separator is "/", the "path" and
// "path/filepath" packages agree, so no test running here can distinguish them.
// The package choice is correctness insurance for a caller whose keys are always
// slash-delimited regardless of host; what this test pins is the observable
// contract, which is what would actually break.
func TestContentSeam_DerivesFromSlashLogicalPath(t *testing.T) {
	cases := []struct {
		name      string
		logical   string
		content   string
		wantMime  string
		wantTitle string
	}{
		{"nested markdown with heading", "docs/guide/intro.md",
			"# Real Heading\n\nBody.\n", "text/markdown", "Real Heading"},
		{"nested markdown without heading", "docs/guide/intro.md",
			"Body with no heading.\n", "text/markdown", "intro"},
		{"nested asciidoc without heading", "reports/2026/q1.adoc",
			"Body with no heading.\n", "text/asciidoc", "q1"},
		{"nested plain text", "notes/daily/standup.txt",
			"Just text.\n", "text/plain", "standup"},
		{"deep path, dotted basename", "a/b/c/release.notes.md",
			"Body.\n", "text/markdown", "release.notes"},
	}

	system := entityid.SystemSlug(seamProject)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := seamHandler(t)
			states, err := h.IngestContentEntityStates(context.Background(),
				[]byte(tc.content), tc.logical, system, "acme", time.Now().UTC())
			if err != nil {
				t.Fatalf("IngestContentEntityStates() error: %v", err)
			}
			parents := parentStates(states)
			if len(parents) != 1 {
				t.Fatalf("parent count: got %d, want 1", len(parents))
			}
			parent := parents[0]

			if got := tripleValue(t, parent, source.DocFilePath); got != tc.logical {
				t.Errorf("file path: got %q, want %q", got, tc.logical)
			}
			if got := tripleValue(t, parent, source.DocMimeType); got != tc.wantMime {
				t.Errorf("mime: got %q, want %q", got, tc.wantMime)
			}
			if got := tripleValue(t, parent, source.DcTitle); got != tc.wantTitle {
				t.Errorf("title: got %q, want %q", got, tc.wantTitle)
			}
			for _, passage := range passageStates(states) {
				if got := tripleValue(t, passage, source.DocFilePath); got != tc.logical {
					t.Errorf("passage file path: got %q, want %q", got, tc.logical)
				}
			}
		})
	}
}
