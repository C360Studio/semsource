package mcpgateway

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/c360studio/semstreams/message"

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

// TestMatchPropertiesOnEntitiesPath pins the change's reason to exist (#166):
// a config dependency's VALUE facts are answerable from the match itself.
func TestMatchPropertiesOnEntitiesPath(t *testing.T) {
	body := &graphSearchBody{Entities: []substrateEntity{
		entityWithTriples("acme.semsource.config.osh.dependency.abc123",
			source.DcTitle, "com.google.guava:guava",
			source.ConfigDepName, "com.google.guava:guava",
			source.ConfigDepVersion, "33.0.0-jre",
			source.ConfigDepKind, "gradle",
			source.ConfigDepConfiguration, "implementation",
		),
	}}
	matches, _, _ := deriveMatches(body)
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

// TestMatchPropertiesAbsentOnTriplelessPaths pins the no-hydration rule:
// digest and ID-only paths carry no triples, so matches render without
// properties rather than fetching them.
func TestMatchPropertiesAbsentOnTriplelessPaths(t *testing.T) {
	digest := &graphSearchBody{EntityDigests: []substrateDigest{{ID: "a.b.config.s.dependency.x", Label: "dep"}}}
	ids := &graphSearchBody{EntityIDs: []string{"a.b.config.s.dependency.x"}}
	for name, body := range map[string]*graphSearchBody{"digests": digest, "entity_ids": ids} {
		matches, _, _ := deriveMatches(body)
		if len(matches) != 1 {
			t.Fatalf("%s: matches = %d, want 1", name, len(matches))
		}
		if matches[0].Properties != nil {
			t.Errorf("%s: properties = %v, want nil (absence stays absence)", name, matches[0].Properties)
		}
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
	raw := []byte(`{"entities":[{"id":"acme.semsource.config.s.dependency.abc",
		"triples":[
			{"predicate":"dc.terms.title","object":"com.google.guava:guava"},
			{"predicate":"code.metric.start-line","object":16},
			{"predicate":"source.doc.chunk-index","object":3},
			{"predicate":"source.config.dependency-version","object":"33.0.0-jre"},
			{"predicate":"source.config.dependency-scope","object":null},
			{"predicate":"source.config.dependency-kind","object":{"nested":"composite"}}
		]}],"count":1}`)
	var body graphSearchBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("substrate shape with numeric objects must decode, got: %v", err)
	}
	matches, _, _ := deriveMatches(&body)
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

// TestTotalMatchesHonestyInvariant pins the pairing the shapes must keep: the
// substrate's count wins when it exceeds what the shape carried, and whenever
// fewer matches render than exist, truncated says so — a partial list must
// never read as the complete hit set.
func TestTotalMatchesHonestyInvariant(t *testing.T) {
	body := &graphSearchBody{
		Count:    100,
		Entities: []substrateEntity{entityWithTriples("a.b.c.d.dependency.x", source.DcTitle, "x")},
	}
	matches, total, truncated := deriveMatches(body)
	if total != 100 {
		t.Errorf("total = %d, want the substrate's count 100, not len(entities)", total)
	}
	if len(matches) != 1 || !truncated {
		t.Errorf("matches=%d truncated=%v — rendering fewer than total MUST set truncated", len(matches), truncated)
	}
}
