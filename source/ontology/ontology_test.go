package ontology

import (
	"testing"

	"github.com/c360studio/semvocab/catalog/bfo"
	"github.com/c360studio/semvocab/catalog/cco"
)

func TestClassForCodeTypes(t *testing.T) {
	cases := map[string]string{
		"function": cco.Algorithm,
		"method":   cco.Algorithm,
		"struct":   cco.PrescriptiveInformationContentEntity,
		"file":     cco.InformationBearingArtifact,
		"repo":     cco.MaterialArtifact,
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
		{"web", "doc", cco.MaterialCopyOfDocument},
		{"web", "page", cco.MaterialCopyOfDocument},
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
	if got, _ := ClassFor("golang", "package"); got != cco.PrescriptiveInformationContentEntity {
		t.Errorf("code package should be PrescriptiveInformationContentEntity, got %q", got)
	}
	if got, _ := ClassFor("config", "package"); got != cco.PrescriptiveInformationContentEntity {
		t.Errorf("config package should be PrescriptiveInformationContentEntity, got %q", got)
	}
	if got, _ := ClassFor("media", "image"); got != cco.InformationBearingArtifact {
		t.Errorf("media image should be InformationBearingArtifact, got %q", got)
	}
	if got, _ := ClassFor("config", "image"); got != cco.DesignativeInformationContentEntity {
		t.Errorf("config image should be DesignativeInformationContentEntity, got %q", got)
	}
}

// TestAllMappedClassesResolveCCOParents verifies every class IRI stamped by
// source/ontology resolves a non-empty superclass chain in semvocab/catalog/cco
// terminating at BFO 2020 Entity (BFO_0000001), and that Algorithm is strictly
// deeper than PrescriptiveInformationContentEntity for fusion specificity.
func TestAllMappedClassesResolveCCOParents(t *testing.T) {
	for k, iri := range sourceClasses {
		parents := cco.Parents(iri)
		if len(parents) == 0 {
			t.Errorf("sourceClasses[%v] (%s): cco.Parents returned empty", k, iri)
			continue
		}
		if root := parents[len(parents)-1]; root != bfo.Entity {
			t.Errorf("sourceClasses[%v] (%s): root parent = %q, want %q", k, iri, root, bfo.Entity)
		}
	}
	for k, iri := range codeTypeClasses {
		parents := cco.Parents(iri)
		if len(parents) == 0 {
			t.Errorf("codeTypeClasses[%v] (%s): cco.Parents returned empty", k, iri)
			continue
		}
		if root := parents[len(parents)-1]; root != bfo.Entity {
			t.Errorf("codeTypeClasses[%v] (%s): root parent = %q, want %q", k, iri, root, bfo.Entity)
		}
	}
	algDepth := len(cco.Parents(cco.Algorithm))
	prescriptiveDepth := len(cco.Parents(cco.PrescriptiveInformationContentEntity))
	if algDepth <= prescriptiveDepth {
		t.Errorf("expected Algorithm depth (%d) > PrescriptiveInformationContentEntity depth (%d)", algDepth, prescriptiveDepth)
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

func TestOverridePredicate(t *testing.T) {
	if ClassPredicate != "entity.ontology.class" {
		t.Errorf("class predicate = %q", ClassPredicate)
	}
}
