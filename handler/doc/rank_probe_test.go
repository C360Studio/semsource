package doc_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semsource/handler/doc"
)

// TestRankProbe is a diagnostic, not a guard: for each protected query it ranks
// EVERY passage of the documents known to hold the live top contenders, so a
// candidate split rule is judged against the whole leaderboard instead of a
// single hand-picked pair. The pair harness (TestDilutionMargin) can miss a rule
// that fixes the named distractor while promoting a third passage; this cannot,
// within the files it covers.
//
// Same admissibility caveat as the harness: live retrieval ranks the whole
// corpus, this ranks only the named files. A win here is necessary, not
// sufficient; the stack remains the referee.
func TestRankProbe(t *testing.T) {
	e := newEmbedderOrSkip(t)

	scenarios := []struct {
		name  string
		query string
		// files maps fixture path -> document title used as identity text.
		files map[string]string
		// mark flags passages worth labeling in the output.
		mark map[string]string // needle -> label
	}{
		{
			name:  "X01 default NATS monitor port",
			query: "what is the default host port for the NATS monitor in docker compose",
			files: map[string]string{"../../README.md": "SemSource"},
			mark: map[string]string{
				"NATS_MONITOR_HOST_PORT=8222":  "ANSWER",
				"NATS_MONITOR_HOST_PORT=28222": "DISTRACTOR",
			},
		},
		{
			name:  "X02 seminstruct port",
			query: "what port does the seminstruct inference container publish",
			files: map[string]string{
				"../../configs/tiers/README.md":     "SemSource query tiers",
				"testdata/dilution/adr-0002.golden": "ADR-0002: Tier Support and Model Registry Passthrough",
			},
			mark: map[string]string{
				"-p 8083:8083": "ANSWER",
				"-p 8081:8081": "DISTRACTOR",
			},
		},
	}

	for _, sc := range scenarios {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			type entry struct {
				title string
				p     doc.Passage
				label string
				score float64
			}
			var entries []entry
			for path, title := range sc.files {
				for _, p := range splitFile(t, path) {
					label := ""
					for needle, l := range sc.mark {
						if strings.Contains(p.Body, needle) {
							if label != "" {
								label += "+"
							}
							label += l
						}
					}
					entries = append(entries, entry{title: title, p: p, label: label})
				}
			}

			inputs := []string{queryPrefix + sc.query}
			for _, en := range entries {
				inputs = append(inputs, identityText(en.title, en.p))
			}
			vecs := e.embed(t, inputs)
			for i := range entries {
				entries[i].score = cosine(vecs[0], vecs[i+1])
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].score > entries[j].score })

			show := 12
			if show > len(entries) {
				show = len(entries)
			}
			for i := 0; i < show; i++ {
				en := entries[i]
				mark := ""
				if en.label != "" {
					mark = "  <-- " + en.label
				}
				t.Logf("%2d  cos=%.4f  %5dB  %-40s %s%s",
					i, en.score, len(en.p.Body), firstLine(en.p.Heading), snippet(en.p.Body), mark)
			}
		})
	}
}

func snippet(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 60 {
		s = s[:60] + "…"
	}
	return fmt.Sprintf("| %s", s)
}
