package mcpgateway

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"

	semsourceast "github.com/c360studio/semsource/source/ast"
	source "github.com/c360studio/semsource/source/vocabulary"
)

func entityWithTriples(id string, pairs ...string) substrateEntity {
	e := substrateEntity{ID: id}
	for i := 0; i+1 < len(pairs); i += 2 {
		e.Triples = append(e.Triples, message.Triple{Predicate: pairs[i], Object: pairs[i+1]})
	}
	return e
}

// textMatch is one substrate word-search match carrying the entity's bytes.
func textMatch(t *testing.T, e substrateEntity, label string, score float64) gtypes.TextSearchMatch {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal entity: %v", err)
	}
	return gtypes.TextSearchMatch{ID: e.ID, Label: label, Score: score, Entity: raw}
}

// TestMatchPropertiesOnEntitiesPath pins the change's reason to exist (#166):
// a config dependency's VALUE facts are answerable from the match itself.
func TestMatchPropertiesOnEntitiesPath(t *testing.T) {
	e := entityWithTriples("acme.semsource.config.osh.dependency.abc123",
		source.DcTitle, "com.google.guava:guava",
		source.ConfigDepName, "com.google.guava:guava",
		source.ConfigDepVersion, "33.0.0-jre",
		source.ConfigDepKind, "gradle",
		source.ConfigDepConfiguration, "implementation",
	)
	text := &gtypes.TextSearchData{Matched: 1, Matches: []gtypes.TextSearchMatch{textMatch(t, e, "", 1.5)}}
	matches, _, _ := deriveMatches(text)
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	props := matches[0].Properties
	want := map[string]string{
		source.ConfigDepName:          "com.google.guava:guava",
		source.ConfigDepVersion:       "33.0.0-jre",
		source.ConfigDepKind:          "gradle",
		source.ConfigDepConfiguration: "implementation",
	}
	if !reflect.DeepEqual(props, want) {
		t.Errorf("properties = %v, want %v", props, want)
	}
	if matches[0].Label != "com.google.guava:guava" {
		t.Errorf("label = %q — properties must not disturb label rendering", matches[0].Label)
	}
}

// TestMatchPropertiesAbsentOnTriplelessPaths pins the no-hydration rule: a
// match whose entity bytes are absent or unreadable renders from the
// substrate's own ID, label, and score, without properties, rather than being
// dropped or fetched.
func TestMatchPropertiesAbsentOnTriplelessPaths(t *testing.T) {
	for name, entity := range map[string]json.RawMessage{"absent": nil, "unreadable": json.RawMessage(`"not an entity"`)} {
		text := &gtypes.TextSearchData{Matched: 1, Matches: []gtypes.TextSearchMatch{
			{ID: "a.b.config.s.dependency.x", Label: "dep", Score: 0.4, Entity: entity},
		}}
		matches, _, _ := deriveMatches(text)
		if len(matches) != 1 {
			t.Fatalf("%s: matches = %d, want 1", name, len(matches))
		}
		if matches[0].Properties != nil {
			t.Errorf("%s: properties = %v, want nil (absence stays absence)", name, matches[0].Properties)
		}
		if matches[0].Label != "dep" || matches[0].Relevance != 0.4 || matches[0].Type != "dependency" {
			t.Errorf("%s: substrate values not preserved: %+v", name, matches[0])
		}
	}
}

// TestSubstrateLabelAndOrderWin pins that the substrate's ranking and label are
// reported as given: matches keep the reply's order (best first), the score is
// the relevance, and the entity title fills in only when the substrate sent no
// label.
func TestSubstrateLabelAndOrderWin(t *testing.T) {
	first := entityWithTriples("a.b.c.golang.function.Second", source.DcTitle, "title-second")
	second := entityWithTriples("a.b.c.golang.function.First", source.DcTitle, "title-first")
	text := &gtypes.TextSearchData{Matched: 2, Matches: []gtypes.TextSearchMatch{
		textMatch(t, first, "Second", 2.5),
		textMatch(t, second, "", 1.25),
	}}
	matches, total, truncated := deriveMatches(text)
	if total != 2 || truncated {
		t.Fatalf("total=%d truncated=%v, want 2/false", total, truncated)
	}
	if matches[0].ID != first.ID || matches[0].Label != "Second" || matches[0].Relevance != 2.5 {
		t.Errorf("first match = %+v, want the substrate's first hit, label, and score", matches[0])
	}
	if matches[1].Label != "title-first" {
		t.Errorf("second label = %q, want the entity title when the substrate sent none", matches[1].Label)
	}
}

// TestMatchPropertiesExcludeNonAllowlisted pins the allowlist boundary:
// relationship edges, bodies, and wall-clock stamps never render, regardless
// of cap headroom.
func TestMatchPropertiesExcludeNonAllowlisted(t *testing.T) {
	e := entityWithTriples("acme.semsource.config.s.gomod.app",
		source.ConfigRequires, "acme.semsource.config.s.dependency.abc",
		source.ConfigDepends, "acme.semsource.config.s.dependency.def",
		semsourceast.DcCreated, "2026-08-13T00:00:00Z",
		"code.doc.comment", "a body-sized comment",
		source.ConfigModulePath, "github.com/example/app",
	)
	props := entityProperties(e)
	if len(props) != 1 || props[source.ConfigModulePath] != "github.com/example/app" {
		t.Errorf("properties = %v, want only the module path", props)
	}
}

// TestMatchPropertiesCapsEnforced pins all three bounds: entry count, value
// size, and WHICH entries survive the cap. The survivors must be exactly the
// first maxMatchProperties predicates in allowlist DECLARATION order — the
// only observable consequence of ordered iteration, so a rewrite that ranges
// a map (order-random selection) fails here rather than passing silently
// (json.Marshal sorts keys, so byte-comparing output can never catch it).
func TestMatchPropertiesCapsEnforced(t *testing.T) {
	long := strings.Repeat("v", maxPropertyValueSize+50)
	pairs := []string{source.ConfigDepVersion, long}
	// Every allowlisted predicate present — more than the cap admits.
	for _, p := range valuePredicates {
		if p != source.ConfigDepVersion {
			pairs = append(pairs, p, "x")
		}
	}
	props := entityProperties(entityWithTriples("a.b.config.s.dependency.x", pairs...))
	if len(props) != maxMatchProperties {
		t.Fatalf("len(properties) = %d, want cap %d", len(props), maxMatchProperties)
	}
	for i, pred := range valuePredicates {
		if _, ok := props[pred]; ok != (i < maxMatchProperties) {
			t.Errorf("predicate %q (allowlist position %d): present=%v, want the FIRST %d in declaration order to survive the cap",
				pred, i, ok, maxMatchProperties)
		}
	}
	got := props[source.ConfigDepVersion]
	if len(got) != maxPropertyValueSize {
		t.Errorf("value length = %d, want exactly the cap %d", len(got), maxPropertyValueSize)
	}
	if want := strings.Repeat("v", maxPropertyValueSize-3) + "…"; got != want {
		t.Errorf("truncated value = %q, want visible marker suffix", got)
	}
}

// TestNullResidueNeverMasksARealValue pins first-RENDERABLE-wins: a null
// residue triple for a predicate must not hide a later real value — the exact
// fact class this surface exists to deliver.
func TestNullResidueNeverMasksARealValue(t *testing.T) {
	e := substrateEntity{ID: "a.b.config.s.dependency.x", Triples: []message.Triple{
		{Predicate: source.ConfigDepVersion, Object: nil},
		{Predicate: source.ConfigDepVersion, Object: "32.1.3-jre"},
	}}
	props := entityProperties(e)
	if props[source.ConfigDepVersion] != "32.1.3-jre" {
		t.Errorf("properties = %v, want the later renderable value, not absence", props)
	}
}

// TestTruncationNeverSplitsARune pins the byte cap against UTF-8 corruption
// (a mid-rune cut would marshal as U+FFFD) and requires the visible marker —
// a truncated value must never look like the substrate's real object.
func TestTruncationNeverSplitsARune(t *testing.T) {
	v := strings.Repeat("a", maxPropertyValueSize-4) + "日本語"
	got := truncateValue(v)
	if len(got) > maxPropertyValueSize {
		t.Errorf("len = %d, want <= %d", len(got), maxPropertyValueSize)
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncated value is not valid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated value %q lacks the visible truncation marker", got)
	}
}

// TestDeriveMatchesSurvivesNonStringObjects pins the decode-fragility
// regression found live (#166 PASS-after run 1): real entities carry NUMERIC
// triple objects (code.metric.start-line, source.doc.chunk-index), and a
// string-typed Object field failed the whole response unmarshal — every match
// silently collapsed into the disclosure-only fallback. The body below is the
// substrate shape verbatim; it must decode, render the title, and render only
// scalar allowlisted values.
func TestDeriveMatchesSurvivesNonStringObjects(t *testing.T) {
	raw := []byte(`{"data":{"text":{"level":0,"groups":[],"matched":1,"missing":0,"bodies_not_indexed":0,
		"truncated":false,"unreadable_groups":0,"answer":"",
		"matches":[{"id":"acme.semsource.config.s.dependency.abc","score":1,
		"entity":{"id":"acme.semsource.config.s.dependency.abc","triples":[
			{"predicate":"dc.terms.title","object":"com.google.guava:guava"},
			{"predicate":"code.metric.start-line","object":16},
			{"predicate":"source.doc.chunk-index","object":3},
			{"predicate":"source.config.dependency-version","object":"33.0.0-jre"},
			{"predicate":"source.config.dependency-scope","object":null},
			{"predicate":"source.config.dependency-kind","object":{"nested":"composite"}}
		]}}]},"hints":[]},"indexed_revision":7,"producer":"graph-query"}`)
	var reply graphSearchReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("substrate shape with numeric objects must decode, got: %v", err)
	}
	matches, _, _ := deriveMatches(reply.Data.Text)
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	if matches[0].Label != "com.google.guava:guava" {
		t.Errorf("label = %q, want the title", matches[0].Label)
	}
	want := map[string]string{source.ConfigDepVersion: "33.0.0-jre"}
	if !reflect.DeepEqual(matches[0].Properties, want) {
		t.Errorf("properties = %v, want %v (numeric non-allowlisted skipped, composite skipped, null never renders as the string \"null\")", matches[0].Properties, want)
	}
}

// TestTotalMatchesHonestyInvariant pins the pairing the reply must keep: the
// substrate's matched count wins when it exceeds what the reply carried, and
// whenever fewer matches render than exist — or the substrate itself says it
// left matches out — truncated says so. A partial list must never read as the
// complete hit set.
func TestTotalMatchesHonestyInvariant(t *testing.T) {
	one := []gtypes.TextSearchMatch{{ID: "a.b.c.d.dependency.x", Label: "x"}}
	matches, total, truncated := deriveMatches(&gtypes.TextSearchData{Matched: 100, Matches: one})
	if total != 100 {
		t.Errorf("total = %d, want the substrate's matched 100, not len(matches)", total)
	}
	if len(matches) != 1 || !truncated {
		t.Errorf("matches=%d truncated=%v — rendering fewer than total MUST set truncated", len(matches), truncated)
	}

	// A hit gone on read-back: matched equals what came, but the substrate
	// reports truncated. Its flag is carried, never recomputed away.
	if _, _, truncated := deriveMatches(&gtypes.TextSearchData{Matched: 1, Matches: one, Missing: 1, Truncated: true}); !truncated {
		t.Error("the substrate's truncated flag was dropped")
	}

	many := make([]gtypes.TextSearchMatch, maxGraphMatches+5)
	for i := range many {
		many[i] = gtypes.TextSearchMatch{ID: "a.b.c.d.dependency.x" + strconv.Itoa(i)}
	}
	matches, total, truncated = deriveMatches(&gtypes.TextSearchData{Matched: len(many), Matches: many})
	if len(matches) != maxGraphMatches || total != len(many) || !truncated {
		t.Errorf("cap: matches=%d total=%d truncated=%v, want %d/%d/true", len(matches), total, truncated, maxGraphMatches, len(many))
	}
}

// TestHonouredHintsKeepTheirAnswers pins the review finding on #292: a
// one-word reference or a path question can be answered entirely beside the
// word matches, with none of them. The honoured hint's answer must reach the
// agent, and no honoured claim may survive without it.
// spec: openspec/specs/graphrag-access/spec.md#Requirement: An answer discloses how far it escalated
func TestHonouredHintsKeepTheirAnswers(t *testing.T) {
	const id = "acme.semsource.repo.config.dependency.guava"
	entity, err := json.Marshal(entityWithTriples(id, source.DcTitle, "guava", source.ConfigDepVersion, "33.0.0-jre"))
	if err != nil {
		t.Fatal(err)
	}
	path := &gtypes.PathSearchData{
		Entities: []gtypes.PathEntity{{EntityID: id}, {EntityID: "acme.semsource.repo.config.gomod.app", Depth: 1}},
		Edges:    []gtypes.RelationshipEdge{{Predicate: source.ConfigRequires, FromEntityID: "acme.semsource.repo.config.gomod.app", ToEntityID: id}},
	}
	empty := &gtypes.TextSearchData{}

	t.Run("entity with zero word matches", func(t *testing.T) {
		data := &gtypes.GlobalSearchData{Text: empty, Entity: entity,
			Hints: []gtypes.GlobalSearchHint{{Hint: hintEntity, Detail: "guava", Honoured: true}}}
		got, _, hints := deriveHintAnswers(data)
		if got == nil || got.ID != id || got.Label != "guava" || got.Properties[source.ConfigDepVersion] != "33.0.0-jre" {
			t.Fatalf("entity answer = %+v, want the honoured entity rendered as a bounded match", got)
		}
		if !hints[0].Honoured {
			t.Errorf("a delivered answer's hint was downgraded: %+v", hints[0])
		}
	})

	t.Run("path with zero word matches", func(t *testing.T) {
		data := &gtypes.GlobalSearchData{Text: empty, Path: path,
			Hints: []gtypes.GlobalSearchHint{{Hint: hintPath, Detail: id, Honoured: true}}}
		_, got, hints := deriveHintAnswers(data)
		if !jsonEqual(got, path) || !hints[0].Honoured {
			t.Errorf("path = %+v hint = %+v, want the substrate's walk carried and the hint honoured", got, hints[0])
		}
	})

	t.Run("no honoured claim without its answer", func(t *testing.T) {
		data := &gtypes.GlobalSearchData{Text: empty, Entity: json.RawMessage(`"not an entity"`),
			Hints: []gtypes.GlobalSearchHint{
				{Hint: hintEntity, Detail: "guava", Honoured: true},
				{Hint: hintPath, Detail: id, Honoured: true},
				{Hint: "time_range", Detail: "last week", Reason: "words only"},
			}}
		_, _, hints := deriveHintAnswers(data)
		for _, h := range hints[:2] {
			if h.Honoured || h.Code != gatewayUnreadableCode || h.Reason == "" {
				t.Errorf("hint %+v survived honoured without its answer, or lost the gateway's attribution", h)
			}
		}
		if hints[2].Code != "" || hints[2].Reason != "words only" {
			t.Errorf("an unhonoured substrate hint was rewritten: %+v", hints[2])
		}
		if !data.Hints[0].Honoured {
			t.Error("the substrate's reply was mutated in place")
		}
	})
}

// TestEmbeddingOnlyReplyRendersVectorOrderBounded: an embedding-only answer
// renders at most maxGraphMatches hits in vector order, and a full list of
// MaxSimilarityLimit hits reads as truncated, since searchGraph asked for no
// more. A null word list with no such merge stays the unknown rung.
// spec: openspec/specs/tier1-consumer-qualification/spec.md#Requirement: graph_search discloses the merged answer at Tier 1
func TestEmbeddingOnlyReplyRendersVectorOrderBounded(t *testing.T) {
	hits := make([]gtypes.SimilarityHit, gtypes.MaxSimilarityLimit)
	for i := range hits {
		hits[i] = gtypes.SimilarityHit{EntityID: "acme.semsource.repo.golang.function.f" + strconv.Itoa(i), Similarity: 0.9}
	}
	semantic, err := json.Marshal(gtypes.SemanticData{Results: hits})
	if err != nil {
		t.Fatal(err)
	}
	merge := &gtypes.GlobalSearchHint{Hint: "semantic", Detail: mergeDetailIndexNotReady, Honoured: true}
	got := buildGraphSearchResult(&gtypes.GlobalSearchData{Semantic: semantic, Merge: merge, Hints: []gtypes.GlobalSearchHint{}})
	if len(got.Matches) != maxGraphMatches || got.TotalMatches != gtypes.MaxSimilarityLimit || !got.Truncated {
		t.Fatalf("matches=%d total=%d truncated=%v, want %d/%d/true",
			len(got.Matches), got.TotalMatches, got.Truncated, maxGraphMatches, gtypes.MaxSimilarityLimit)
	}
	for i, m := range got.Matches {
		if m.VectorRank != i+1 || m.ID != hits[i].EntityID || m.Relevance != 0 {
			t.Fatalf("match %d = %+v, want vector order with no relevance", i, m)
		}
	}
	if got.Retrieval.Rung != rungEntitiesOnly || got.Merge != merge {
		t.Errorf("retrieval=%+v merge=%+v", got.Retrieval, got.Merge)
	}

	unknown := buildGraphSearchResult(&gtypes.GlobalSearchData{Hints: []gtypes.GlobalSearchHint{}})
	if unknown.Retrieval.Rung != rungUnknown || unknown.Matches != nil {
		t.Errorf("null word list with no merge = %+v, want the unknown rung and no matches", unknown)
	}
}
