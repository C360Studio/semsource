package ontology

import (
	"slices"
	"testing"

	"github.com/c360studio/semstreams/vocabulary/bfo"
	"github.com/c360studio/semstreams/vocabulary/cco"
)

func TestClassForCodeTypes(t *testing.T) {
	cases := map[string]string{
		"function": cco.Algorithm,
		"method":   cco.Algorithm,
		"struct":   cco.SoftwareCode,
		"file":     cco.InformationBearingArtifact,
		"repo":     cco.Artifact,
	}
	for typ, want := range cases {
		got, ok := ClassFor("golang", typ)
		if !ok || got != want {
			t.Errorf("ClassFor(golang, %s) = %q,%v; want %q", typ, got, ok, want)
		}
	}
}

func TestClassForSourceKinds(t *testing.T) {
	cases := []struct {
		domain, typ, want string
	}{
		{"git", "author", cco.Person},
		{"git", "commit", cco.Act},
		{"git", "branch", cco.DesignativeInformationContentEntity},
		{"web", "doc", cco.Document},
		{"web", "page", cco.Document},
	}
	for _, c := range cases {
		got, ok := ClassFor(c.domain, c.typ)
		if !ok || got != c.want {
			t.Errorf("ClassFor(%s, %s) = %q,%v; want %q", c.domain, c.typ, got, ok, c.want)
		}
	}
}

// TestClassForCollisionDisambiguation locks in the reason ClassFor keys on
// (domain, type): the same type string means different things per domain.
func TestClassForCollisionDisambiguation(t *testing.T) {
	if got, _ := ClassFor("golang", "package"); got != cco.SoftwareCode {
		t.Errorf("code package should be SoftwareCode, got %q", got)
	}
	if got, _ := ClassFor("config", "package"); got != cco.Specification {
		t.Errorf("config package should be Specification, got %q", got)
	}
	if got, _ := ClassFor("media", "image"); got != cco.InformationBearingArtifact {
		t.Errorf("media image should be InformationBearingArtifact, got %q", got)
	}
	if got, _ := ClassFor("config", "image"); got != cco.Identifier {
		t.Errorf("config image should be Identifier, got %q", got)
	}
}

func TestClassForUnknown(t *testing.T) {
	if _, ok := ClassFor("golang", "nonsense"); ok {
		t.Error("unknown code type should not resolve")
	}
	if _, ok := ClassFor("nope", "thing"); ok {
		t.Error("unknown domain/type should not resolve")
	}
}

func TestDepthAndSpecificity(t *testing.T) {
	if d := Depth(bfo.Entity); d != 0 {
		t.Errorf("Depth(Entity) = %d; want 0", d)
	}
	if Depth(cco.Algorithm) <= Depth(cco.InformationContentEntity) {
		t.Errorf("Algorithm (%d) should be deeper/more specific than ICE (%d)",
			Depth(cco.Algorithm), Depth(cco.InformationContentEntity))
	}
}

func TestDistance(t *testing.T) {
	if d := Distance(cco.Document, cco.Document); d != 0 {
		t.Errorf("Distance to self = %d; want 0", d)
	}
	// Algorithm and SoftwareCode are siblings under DirectiveICE → 2 hops.
	if d := Distance(cco.Algorithm, cco.SoftwareCode); d != 2 {
		t.Errorf("Distance(Algorithm, SoftwareCode) = %d; want 2", d)
	}
	// A Person is far from an Algorithm (continuant spine vs ICE branch).
	if Distance(cco.Person, cco.Algorithm) <= Distance(cco.Algorithm, cco.SoftwareCode) {
		t.Error("Person↔Algorithm should be farther than Algorithm↔SoftwareCode")
	}
	if d := Distance("urn:unknown", cco.Algorithm); d != -1 {
		t.Errorf("Distance with unknown class = %d; want -1", d)
	}
}

func TestAncestorsChainToRoot(t *testing.T) {
	anc := Ancestors(cco.Document)
	for _, want := range []string{cco.Artifact, bfo.Object, bfo.Entity} {
		if !slices.Contains(anc, want) {
			t.Errorf("Ancestors(Document) missing %q; got %v", want, anc)
		}
	}
}

func TestOverridePredicate(t *testing.T) {
	if ClassPredicate != "entity.ontology.class" {
		t.Errorf("class predicate = %q", ClassPredicate)
	}
}

// TestParentMapIsRootedTree guards against a future bad edit that introduces a
// cycle or an orphan: every class in the subclass table must terminate at the
// BFO root.
func TestParentMapIsRootedTree(t *testing.T) {
	for child := range parent {
		anc := Ancestors(child)
		if got := anc[len(anc)-1]; got != bfo.Entity {
			t.Errorf("Ancestors(%s) terminates at %q; want bfo.Entity", child, got)
		}
	}
}

// TestEmittedClassesHaveHierarchy ensures every class ClassFor can emit has a
// position in the subclass table, so no result ranks with zero ontology signal.
func TestEmittedClassesHaveHierarchy(t *testing.T) {
	check := func(iri string) {
		if _, ok := parent[iri]; !ok && iri != bfo.Entity {
			t.Errorf("emitted class %q is missing from the subclass table", iri)
		}
	}
	for _, iri := range sourceClasses {
		check(iri)
	}
	for _, iri := range codeTypeClasses {
		check(iri)
	}
}
