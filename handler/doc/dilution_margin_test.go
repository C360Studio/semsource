package doc_test

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semsource/handler/doc"
)

// The offline dilution harness.
//
// Ranking for NL doc queries is essentially the embedding's own cosine order —
// salience is a deliberately small tie-breaker — so what SemSource controls is
// the body text it emits. This harness measures that directly: it splits a
// document with the REAL splitter, embeds each passage the way graph-embedding
// does, and reports the signed margin between an intended answer and a named
// distractor.
//
// It exists so a candidate split rule can be judged for the cost of an embedding
// call instead of a stack rebuild, and it lives in the repository rather than a
// session workspace because the project already lost one graded set that way.
//
// WHY IT IS ADMISSIBLE. An offline proxy that has never been checked against
// reality is a cheaper way to be confidently wrong. This one reproduced a known
// live ordering at three corpus states, including the SIGN of the margin at the
// crossover — see TestDilutionMarginReproducesLiveOrdering. It earned that only
// after the identity text was prepended the way graph-embedding actually embeds;
// an earlier attempt on raw bodies produced the right trend and the wrong
// ranking. If a prediction here ever disagrees with the stack, the stack wins and
// the disagreement is recorded rather than tuned away.
//
//	SCORECARD_EMBED=http://localhost:8099 go test ./handler/doc/ -run Dilution -v
//
// Stand the embedder up with the digest the deployment pins:
//
//	docker run --rm -d -p 8099:8081 ghcr.io/c360studio/semembed:latest@sha256:<digest>

const (
	// queryPrefix is arctic-embed's asymmetric retrieval prefix. It belongs on
	// the QUERY ONLY; applying it to passages too would measure a different
	// model than the one that serves retrieval.
	queryPrefix = "Represent this sentence for searching relevant passages: "

	// embedModel matches configs/mvp.json's semembed endpoint.
	embedModel = "Snowflake/snowflake-arctic-embed-s"
)

// splitter bounds, matching defaultBounds. Held here so a bounds change is a
// visible edit to the harness rather than a silent change of what it measures.
const (
	boundCeiling = 2000
	boundFloor   = 400
	boundHardMax = 6000
)

// dilutionPair is one protected answer/distractor relationship: a query, the
// document and literal that should answer it, and the passage that must not
// outrank it.
type dilutionPair struct {
	name string
	// query is asked verbatim; the harness adds the retrieval prefix.
	query string
	// answerDoc is the document title used as identity text, and answerFile the
	// fixture (or repo path) holding it.
	answerDoc, answerFile string
	// answerNeedle is a literal that appears in the passage that answers the query.
	answerNeedle string
	// distractorDoc/File/Heading name the passage observed to compete with it.
	distractorDoc, distractorFile, distractorHeading string
	// distractorNeedle, when set, identifies the competing passage by a literal
	// instead of by heading. Required once a heading can yield several passages:
	// isolation splits a section into prose and fence, and heading-matching then
	// returns whichever comes first rather than the one that actually competes.
	// Selecting the wrong passage made this harness report +0.0893 for a pair the
	// live stack graded MISLEADING.
	distractorNeedle string
}

// protectedPairs are the answer/distractor relationships this harness guards.
// A pair here is a claim that the product answers a question correctly; a
// negative margin means it no longer does, and that is a failure whether or not
// a graded run has caught it yet.
var protectedPairs = []dilutionPair{
	{
		name:              "X02 seminstruct port vs ADR-0002 service choices",
		query:             "what port does the seminstruct inference container publish",
		answerDoc:         "SemSource query tiers",
		answerFile:        "../../configs/tiers/README.md",
		answerNeedle:      "-p 8083:8083",
		distractorDoc:     "ADR-0002: Tier Support and Model Registry Passthrough",
		distractorFile:    "testdata/dilution/adr-0002.golden",
		distractorHeading: "Service Choices",
	},
	{
		// X01. Added after fence isolation regressed it from `correct` to
		// MISLEADING: the distractor is ITSELF a fenced `docker compose` block,
		// so isolation sharpened the workaround passage exactly as it sharpened
		// X02's answer. Isolation is symmetric, and this pair is what makes that
		// visible offline instead of costing a stack rebuild.
		name:              "X01 default NATS monitor port vs Quick Start workaround",
		query:             "what is the default host port for the NATS monitor in docker compose",
		answerDoc:         "SemSource",
		answerFile:        "../../README.md",
		answerNeedle:      "NATS_MONITOR_HOST_PORT=8222",
		distractorDoc:     "SemSource",
		distractorFile:    "../../README.md",
		distractorHeading: "Quick Start",
		distractorNeedle:  "NATS_MONITOR_HOST_PORT=28222",
	},
}

// TestDilutionMargin reports the answer-versus-distractor margin for every
// protected pair against the CURRENT repository content, and fails on a negative
// one. This is the guard: it catches dilution offline, before a graded run has to
// discover it.
func TestDilutionMargin(t *testing.T) {
	e := newEmbedderOrSkip(t)
	t.Logf("embedder=%s model=%s prefix=%q bounds=%d/%d/%d",
		e.base, embedModel, queryPrefix, boundCeiling, boundFloor, boundHardMax)

	for _, p := range protectedPairs {
		p := p
		t.Run(p.name, func(t *testing.T) {
			answer := passageContaining(t, p.answerFile, p.answerNeedle)
			distractor := selectDistractor(t, p)

			margin, aScore, dScore := e.margin(t, p, answer, distractor)
			t.Logf("answer     %5d B  cos=%.4f  (%s)", len(answer.Body), aScore, firstLine(answer.Heading))
			t.Logf("distractor %5d B  cos=%.4f  (%s)", len(distractor.Body), dScore, firstLine(distractor.Heading))
			t.Logf("margin     %+.4f", margin)

			if margin <= 0 {
				t.Errorf("answer no longer outranks its distractor (margin %+.4f): the passage carrying %q "+
					"has been diluted below %q. Fix the splitter, not the document.",
					margin, p.answerNeedle, p.distractorHeading)
			}
		})
	}
}

// TestScorecardAnswerPassagesStayDivided is the splitter-level guard for the
// graded discrimination answers, and unlike the margin tests it needs no
// embedder — it runs in every CI pass. It pins the STRUCTURAL half of the fix:
// each answer literal must sit in a passage divided from the prose that diluted
// it, small enough to be mostly signal. The margin harness measures whether that
// passage then wins; this guard catches the splitter regressing before anyone
// pays for an embedding run.
func TestScorecardAnswerPassagesStayDivided(t *testing.T) {
	cases := []struct {
		name, file, needle string
		maxBytes           int
		mustNotContain     string
	}{
		{
			name:   "X02 answer is the isolated docker run fence",
			file:   "../../configs/tiers/README.md",
			needle: "-p 8083:8083",
			// The isolated fence is 180 B today; 400 B (the floor) is the point
			// past which it is evidently carrying prose again.
			maxBytes:       400,
			mustNotContain: "Requires **seminstruct**",
		},
		{
			name:           "X01 answer is the NATS key group",
			file:           "../../README.md",
			needle:         "NATS_MONITOR_HOST_PORT=8222",
			maxBytes:       400,
			mustNotContain: "SEMSOURCE_CONFIG=",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			p := passageContaining(t, c.file, c.needle)
			if len(p.Body) > c.maxBytes {
				t.Errorf("passage carrying %q is %d B (max %d): it has re-absorbed surrounding prose "+
					"and its vector is diluting again", c.needle, len(p.Body), c.maxBytes)
			}
			if c.mustNotContain != "" && strings.Contains(p.Body, c.mustNotContain) {
				t.Errorf("passage carrying %q also carries %q: the split that separated them has regressed",
					c.needle, c.mustNotContain)
			}
		})
	}
}

// TestDilutionMarginReproducesLiveOrdering is the harness's credibility pin.
//
// Three states of configs/tiers/README.md were observed live against the same
// distractor: the answer ranked 0, then 1, then 4 as prose accumulated. If this
// harness cannot reproduce that ordering — including the sign flip between the
// first and second state — it is measuring a proxy and must not be quoted as
// evidence that a candidate split rule works.
//
// It reads FROZEN passage bodies rather than re-splitting the documents. The
// claim being pinned is about the EMBEDDING model — that cosine over
// identity-text-plus-body predicts live ordering — not about any particular
// splitter. Re-splitting would invalidate the pin the moment the splitter
// changes, which is exactly what happened when fence isolation landed: all three
// states collapsed to the same isolated block and the historical ordering became
// unreproducible. Freezing the bodies keeps the pin meaningful for the life of
// the harness.
func TestDilutionMarginReproducesLiveOrdering(t *testing.T) {
	e := newEmbedderOrSkip(t)

	pair := protectedPairs[0]
	distractor := passageWithHeading(t, pair.distractorFile, pair.distractorHeading)

	states := []struct {
		label        string
		file         string
		wantBytes    int
		liveRank     int
		wantPositive bool
	}{
		{"pre-#109", "testdata/dilution/tiers-pre-109-answer-passage.golden", 839, 0, true},
		{"post-#109", "testdata/dilution/tiers-post-109-answer-passage.golden", 1964, 1, false},
		{"current", "testdata/dilution/tiers-current-answer-passage.golden", 1943, 4, false},
	}

	var margins []float64
	for _, st := range states {
		answer := frozenPassage(t, st.file)
		if len(answer.Body) != st.wantBytes {
			t.Fatalf("%s: frozen passage is %d B, expected %d — the fixture has drifted and the pin "+
				"no longer reproduces the observed states", st.label, len(answer.Body), st.wantBytes)
		}
		margin, aScore, dScore := e.margin(t, pair, answer, distractor)
		margins = append(margins, margin)
		t.Logf("%-10s answer=%5dB cos=%.4f  distractor cos=%.4f  margin=%+.4f  (live rank %d)",
			st.label, len(answer.Body), aScore, dScore, margin, st.liveRank)

		if got := margin > 0; got != st.wantPositive {
			t.Errorf("%s: margin %+.4f is %s, but the live stack ranked the answer %d — "+
				"the harness does not reproduce the observed ordering and is not admissible as evidence",
				st.label, margin, map[bool]string{true: "positive", false: "negative"}[got], st.liveRank)
		}
	}

	// Dilution is monotone here: each state added prose to the same section, so
	// the margin must not improve as the passage grows.
	for i := 1; i < len(margins); i++ {
		if margins[i] > margins[i-1] {
			t.Errorf("margin rose from %+.4f (%s) to %+.4f (%s) though the passage only grew; "+
				"the dilution model does not hold",
				margins[i-1], states[i-1].label, margins[i], states[i].label)
		}
	}
}

// frozenPassage reads a passage frozen as "heading\x00body", so the pin measures
// the exact text the live stack embedded rather than whatever the current
// splitter would produce from the same document.
func frozenPassage(t *testing.T, path string) doc.Passage {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	head, body, ok := strings.Cut(string(raw), "\x00")
	if !ok {
		t.Fatalf("%s: frozen passage is missing its heading separator", filepath.Base(path))
	}
	return doc.Passage{Heading: head, Body: body}
}

// --- harness internals ------------------------------------------------------

type embedder struct {
	base   string
	client *http.Client
}

func newEmbedderOrSkip(t *testing.T) *embedder {
	t.Helper()
	base := os.Getenv("SCORECARD_EMBED")
	if base == "" {
		t.Skip("set SCORECARD_EMBED to an OpenAI-compatible embeddings base URL (e.g. http://localhost:8099)")
	}
	return &embedder{base: strings.TrimSuffix(base, "/"), client: &http.Client{Timeout: 3 * time.Minute}}
}

// margin returns (answerScore - distractorScore, answerScore, distractorScore).
func (e *embedder) margin(t *testing.T, p dilutionPair, answer, distractor doc.Passage) (float64, float64, float64) {
	t.Helper()
	vecs := e.embed(t, []string{
		queryPrefix + p.query,
		identityText(p.answerDoc, answer),
		identityText(p.distractorDoc, distractor),
	})
	a := cosine(vecs[0], vecs[1])
	d := cosine(vecs[0], vecs[2])
	return a - d, a, d
}

// identityText mirrors what graph-embedding embeds: the entity's identity text
// ahead of the fetched body. Measuring the bare body instead produced the right
// trend and the wrong ranking, which is precisely the error this reproduces.
// The title comes from the real passageTitle so this cannot drift from what
// production actually emits — including the heading ancestry it now carries.
func identityText(docTitle string, p doc.Passage) string {
	return doc.PassageTitle(docTitle, p) + "\n\n" + p.Body
}

func (e *embedder) embed(t *testing.T, inputs []string) [][]float64 {
	t.Helper()
	body, err := json.Marshal(map[string]any{"model": embedModel, "input": inputs})
	if err != nil {
		t.Fatalf("marshal embed request: %v", err)
	}
	resp, err := e.client.Post(e.base+"/v1/embeddings", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("embed request to %s: %v", e.base, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("embedder returned %s", resp.Status)
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode embed response: %v", err)
	}
	if len(out.Data) != len(inputs) {
		t.Fatalf("embedder returned %d vectors for %d inputs", len(out.Data), len(inputs))
	}
	sort.Slice(out.Data, func(i, j int) bool { return out.Data[i].Index < out.Data[j].Index })
	vecs := make([][]float64, len(out.Data))
	for i, d := range out.Data {
		vecs[i] = d.Embedding
	}
	return vecs
}

func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// passageContaining returns the single passage holding needle, failing if the
// document yields none or more than one — an ambiguous needle would silently
// measure the wrong passage.
func passageContaining(t *testing.T, path, needle string) doc.Passage {
	t.Helper()
	var hits []doc.Passage
	for _, p := range splitFile(t, path) {
		if strings.Contains(p.Body, needle) {
			hits = append(hits, p)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0]
	case 0:
		t.Fatalf("%s: no passage contains %q", filepath.Base(path), needle)
	default:
		t.Fatalf("%s: %d passages contain %q; the needle must identify exactly one",
			filepath.Base(path), len(hits), needle)
	}
	return doc.Passage{}
}

// selectDistractor prefers a literal over a heading, because a heading is no
// longer unique once a section can split into several passages.
func selectDistractor(t *testing.T, p dilutionPair) doc.Passage {
	t.Helper()
	if p.distractorNeedle != "" {
		return passageContaining(t, p.distractorFile, p.distractorNeedle)
	}
	return passageWithHeading(t, p.distractorFile, p.distractorHeading)
}

func passageWithHeading(t *testing.T, path, heading string) doc.Passage {
	t.Helper()
	for _, p := range splitFile(t, path) {
		if strings.Contains(p.Heading, heading) {
			return p
		}
	}
	t.Fatalf("%s: no passage with heading containing %q", filepath.Base(path), heading)
	return doc.Passage{}
}

func splitFile(t *testing.T, path string) []doc.Passage {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return doc.SplitPassagesBounded(content, boundCeiling, boundFloor, boundHardMax, false)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 70 {
		s = s[:70] + "…"
	}
	return s
}
