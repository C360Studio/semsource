package ontology

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/vocabulary/cco"
)

func TestStampClassAppendsClass(t *testing.T) {
	id := "acme.semsource.golang.sys.function.foo" // domain=golang, type=function
	in := []message.Triple{{Subject: id, Predicate: "code.artifact.path", Object: "a.go"}}
	out := StampClass(id, in, time.Unix(0, 0))

	if len(out) != len(in)+1 {
		t.Fatalf("expected one class triple appended, got %d→%d", len(in), len(out))
	}
	last := out[len(out)-1]
	if last.Predicate != ClassPredicate || last.Object != cco.Algorithm || last.Subject != id {
		t.Fatalf("unexpected class triple: %+v", last)
	}
	if last.Confidence != 1.0 {
		t.Errorf("class triple confidence = %v; want 1.0", last.Confidence)
	}
	if len(in) != 1 {
		t.Fatal("input slice was mutated")
	}
}

func TestStampClassRespectsOverride(t *testing.T) {
	id := "acme.semsource.golang.sys.function.foo"
	in := []message.Triple{{Subject: id, Predicate: ClassPredicate, Object: "urn:custom"}}
	out := StampClass(id, in, time.Unix(0, 0))
	if len(out) != 1 || out[0].Object != "urn:custom" {
		t.Fatalf("explicit override must be preserved, got %+v", out)
	}
}

func TestStampClassNoMappingIsNoop(t *testing.T) {
	id := "acme.semsource.golang.sys.mystery.foo" // unknown type
	in := []message.Triple{{Subject: id, Predicate: "x.y.z", Object: "v"}}
	out := StampClass(id, in, time.Unix(0, 0))
	if len(out) != 1 {
		t.Fatalf("unknown kind should be a no-op, got %d triples", len(out))
	}
}
