package doc_test

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler/doc"
)

// TestBoundsSweep prints how candidate passage bounds behave over a real corpus.
// It is a measuring instrument, not an assertion: it exists so the ceiling and
// floor can be chosen from evidence instead of guessed, which is the doc-passage
// chunking change's one open design question.
//
// It is skipped unless SCORECARD_CORPUS points at a corpus directory, because it
// walks a tree that is not part of this repository's test fixtures:
//
//	SCORECARD_CORPUS=/path/to/corpus go test ./handler/doc/ -run TestBoundsSweep -v
//
// What it cannot tell you: whether a given ceiling retrieves better. Passage size
// is a property of the splitter alone, and retrieval quality is a property of the
// whole stack. This narrows the candidates worth spending a live A/B on; the A/B
// still decides. Treating the distribution as the answer would be measuring a
// proxy for the thing rather than the thing.
func TestBoundsSweep(t *testing.T) {
	corpus := os.Getenv("SCORECARD_CORPUS")
	if corpus == "" {
		t.Skip("set SCORECARD_CORPUS to a corpus directory to run the sweep")
	}

	docs := readCorpus(t, corpus)
	t.Logf("corpus: %d ingestable documents, %d bytes total", len(docs), totalBytes(docs))

	candidates := []struct{ ceiling, floor int }{
		{1000, 200}, {1500, 300}, {2000, 400}, {3000, 600}, {4000, 800}, {6000, 1200},
	}

	t.Log("")
	t.Logf("%-13s %8s %8s %8s %8s %8s  %s", "ceiling/floor", "passages", "median", "p90", "max", "<floor", "separates X01 / X02")
	for _, c := range candidates {
		var sizes []int
		for _, d := range docs {
			for _, p := range doc.SplitPassagesBounded(d.content, c.ceiling, c.floor, 6000) {
				sizes = append(sizes, p.End-p.Start)
			}
		}
		sort.Ints(sizes)
		under := 0
		for _, s := range sizes {
			if s < c.floor {
				under++
			}
		}
		t.Logf("%-13s %8d %8d %8d %8d %7.1f%%  %s / %s",
			strconv.Itoa(c.ceiling)+"/"+strconv.Itoa(c.floor), len(sizes),
			pct(sizes, 50), pct(sizes, 90), sizes[len(sizes)-1],
			100*float64(under)/float64(len(sizes)),
			verdict(separates(docs, "README.md", c, "NATS_MONITOR_HOST_PORT=8222", "NATS_MONITOR_HOST_PORT=28222")),
			verdict(separates(docs, "configs/tiers/README.md", c, "-p 8083:8083", "-p 8081:8081")))
	}
	t.Log("")
	t.Log("`separates` means the two literals of a discrimination question land in")
	t.Log("DIFFERENT passages. Where they do not, that question cannot discriminate at")
	t.Log("that ceiling — it would report IMPRECISE regardless of retrieval quality.")
}

type corpusDoc struct {
	rel     string
	content []byte
}

func readCorpus(t *testing.T, root string) []corpusDoc {
	t.Helper()
	var out []corpusDoc
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// The scorecard quotes both literals of every discrimination question
		// side by side, so it must stay out of the measured corpus.
		if strings.Contains(path, "scripts/scorecard") {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".md", ".mdx", ".txt":
		default:
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, corpusDoc{rel: rel, content: b})
		return nil
	})
	if err != nil {
		t.Fatalf("walk corpus: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("no ingestable documents under %s", root)
	}
	return out
}

// separates reports whether two literals land in different passages of one
// document — the property a discrimination question depends on.
func separates(docs []corpusDoc, rel string, c struct{ ceiling, floor int }, good, bad string) bool {
	for _, d := range docs {
		if filepath.ToSlash(d.rel) != rel {
			continue
		}
		var goodAt, badAt = -1, -1
		for _, p := range doc.SplitPassagesBounded(d.content, c.ceiling, c.floor, 6000) {
			if goodAt < 0 && strings.Contains(p.Body, good) {
				goodAt = p.Ordinal
			}
			if badAt < 0 && strings.Contains(p.Body, bad) {
				badAt = p.Ordinal
			}
		}
		return goodAt >= 0 && badAt >= 0 && goodAt != badAt
	}
	return false
}

func verdict(ok bool) string {
	if ok {
		return "yes"
	}
	return "NO"
}

func totalBytes(docs []corpusDoc) int {
	n := 0
	for _, d := range docs {
		n += len(d.content)
	}
	return n
}

func pct(sorted []int, p int) int {
	if len(sorted) == 0 {
		return 0
	}
	i := len(sorted) * p / 100
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}
