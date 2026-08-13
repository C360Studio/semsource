package mcpgateway

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	semsourceast "github.com/c360studio/semsource/source/ast"
	source "github.com/c360studio/semsource/source/vocabulary"
)

func entityWithTriples(id string, pairs ...string) substrateEntity {
	e := substrateEntity{ID: id}
	for i := 0; i+1 < len(pairs); i += 2 {
		obj, _ := json.Marshal(pairs[i+1])
		e.Triples = append(e.Triples, struct {
			Predicate string          `json:"predicate"`
			Object    json.RawMessage `json:"object"`
		}{Predicate: pairs[i], Object: obj})
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

// TestMatchPropertiesCapsEnforced pins both bounds: entry count and value size.
func TestMatchPropertiesCapsEnforced(t *testing.T) {
	long := strings.Repeat("v", maxPropertyValueSize+50)
	pairs := []string{source.ConfigDepVersion, long}
	// More allowlisted predicates than the cap admits.
	for _, p := range valuePredicates {
		pairs = append(pairs, p, "x")
	}
	props := entityProperties(entityWithTriples("a.b.config.s.dependency.x", pairs...))
	if len(props) != maxMatchProperties {
		t.Errorf("len(properties) = %d, want cap %d", len(props), maxMatchProperties)
	}
	if got := props[source.ConfigDepVersion]; len(got) != maxPropertyValueSize {
		t.Errorf("value length = %d, want truncated to %d", len(got), maxPropertyValueSize)
	}
}

// TestMatchPropertiesDeterministicAcrossRuns guards the determinism-gate
// class: rendering must not depend on map iteration, so repeated rendering of
// the same entity marshals identically.
func TestMatchPropertiesDeterministicAcrossRuns(t *testing.T) {
	e := entityWithTriples("a.b.config.s.dependency.x",
		source.ConfigDepVersion, "1.2.3",
		source.ConfigDepKind, "gradle",
		source.ConfigDepName, "g:n",
		source.ConfigDepScope, "test",
	)
	first, err := json.Marshal(entityProperties(e))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		again, err := json.Marshal(entityProperties(e))
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(first) {
			t.Fatalf("run %d rendered %s, first rendered %s", i, again, first)
		}
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

// TestCodeVersionLiteralMatchesASTConstant pins the one allowlist entry kept
// as a literal (to spare the gateway a tree-sitter dependency) to the real
// constant it mirrors.
func TestCodeVersionLiteralMatchesASTConstant(t *testing.T) {
	for _, p := range valuePredicates {
		if p == semsourceast.CodeVersion {
			return
		}
	}
	t.Fatalf("valuePredicates does not contain semsourceast.CodeVersion (%q)", semsourceast.CodeVersion)
}
