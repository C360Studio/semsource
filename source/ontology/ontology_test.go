package ontology

import (
	"testing"

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

func TestOverridePredicate(t *testing.T) {
	if ClassPredicate != "entity.ontology.class" {
		t.Errorf("class predicate = %q", ClassPredicate)
	}
}
