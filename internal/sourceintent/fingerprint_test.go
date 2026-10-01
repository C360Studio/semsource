package sourceintent

import (
	"encoding/json"
	"github.com/c360studio/semsource/graph"
	sourcevocab "github.com/c360studio/semsource/source/vocabulary"
	"github.com/c360studio/semstreams/message"
	"testing"
)

func TestSourceFingerprintCanonicalEvidence(t *testing.T) {
	id := "acme.platform.docs.web.doc.one"
	triples := []message.Triple{{Subject: id, Predicate: "source.doc.title", Object: "title"}, {Subject: id, Predicate: sourcevocab.DocChunkCount, Object: json.Number("9007199254740993")}}
	first, err := SourceFingerprint(id, triples)
	if err != nil {
		t.Fatal(err)
	}
	reversed := []message.Triple{triples[1], triples[0], {Subject: id, Predicate: "entity.lifecycle.stale", Object: "source_removed"}}
	second, err := SourceFingerprint(id, reversed)
	if err != nil || first != second {
		t.Fatalf("lifecycle/order changed fingerprint: %v", err)
	}
	reversed[0].Object = json.Number("9007199254740992")
	third, err := SourceFingerprint(id, reversed)
	if err != nil || third == first {
		t.Fatal("large integer precision lost")
	}
	triples[1].Object = 1
	a, _ := SourceFingerprint(id, triples)
	triples[1].Object = json.Number("1.0")
	b, _ := SourceFingerprint(id, triples)
	if a != b {
		t.Fatal("wire numeric representation changed evidence")
	}
	triples[1].Object = "1"
	c, _ := SourceFingerprint(id, triples)
	if c == a {
		t.Fatal("string and numeric evidence collided")
	}
}

func TestClonePayloadIsImmutable(t *testing.T) {
	original := &graph.EntityPayload{ID: "acme.platform.docs.web.doc.one", TripleData: []message.Triple{{Object: map[string]any{"n": json.Number("9007199254740993")}}}}
	cloned, err := ClonePayload(original)
	if err != nil {
		t.Fatal(err)
	}
	original.TripleData[0].Object.(map[string]any)["n"] = "changed"
	got := cloned.TripleData[0].Object.(map[string]any)["n"]
	if got != json.Number("9007199254740993") {
		t.Fatalf("mutable or imprecise snapshot: %#v", got)
	}
}

func TestPublicationFingerprintMatchesRetainedProfileAndStorage(t *testing.T) {
	p := &graph.EntityPayload{ID: "acme.platform.docs.web.doc.one", IndexingProfileHint: graph.IndexingProfileContent, Storage: &message.StorageReference{StorageInstance: "body", Key: "one", Size: 2}}
	published, err := PublicationFingerprint(p)
	if err != nil {
		t.Fatal(err)
	}
	triples := []message.Triple{{Subject: p.ID, Predicate: "entity.indexing.profile", Object: graph.IndexingProfileContent}}
	retained, err := RetainedFingerprint(p.ID, triples, p.Storage)
	if err != nil || published != retained {
		t.Fatalf("retained mismatch: %v", err)
	}
	changed := *p.Storage
	changed.Key = "other"
	different, _ := RetainedFingerprint(p.ID, triples, &changed)
	if different == published {
		t.Fatal("content handle omitted")
	}
	triples[0].Object = graph.IndexingProfileControl
	different, _ = RetainedFingerprint(p.ID, triples, p.Storage)
	if different == published {
		t.Fatal("profile omitted")
	}
	if _, err := RetainedFingerprint(p.ID, nil, p.Storage); err == nil {
		t.Fatal("missing retained profile accepted")
	}
}

func TestSourceEvidenceDefensiveCanonicalization(t *testing.T) {
	id := "acme.platform.docs.web.doc.one"
	predicate := graph.OwnedPredicates()[0]
	for _, object := range []any{nil, true, []any{json.Number("1.0"), "1"}, map[string]any{"array": []any{false, nil}}} {
		triples := []message.Triple{{Subject: id, Predicate: predicate, Object: object}}
		if _, err := SourceFingerprint(id, triples); err != nil {
			t.Fatalf("valid typed fact %T: %v", object, err)
		}
	}
	if _, err := SourceFingerprint(id, []message.Triple{{Subject: "foreign", Predicate: predicate, Object: "value"}}); err == nil {
		t.Fatal("foreign subject accepted")
	}
	if _, err := ClonePayload(nil); err == nil {
		t.Fatal("nil payload accepted")
	}
	if _, err := ClonePayload(&graph.EntityPayload{TripleData: []message.Triple{{Object: make(chan int)}}}); err == nil {
		t.Fatal("non-serializable evidence accepted")
	}
	if _, err := PublicationFingerprint(nil); err == nil {
		t.Fatal("nil publication accepted")
	}
	if _, err := RetainedFingerprint(id, []message.Triple{{Subject: id, Predicate: "entity.indexing.profile", Object: true}}, nil); err == nil {
		t.Fatal("non-string profile accepted")
	}
	if _, err := RetainedFingerprint(id, []message.Triple{{Subject: id, Predicate: "entity.indexing.profile", Object: "content"}, {Subject: id, Predicate: "entity.indexing.profile", Object: "control"}}, nil); err == nil {
		t.Fatal("conflicting profiles accepted")
	}
}

func TestManifestDigestExactCurrentSet(t *testing.T) {
	a := ManifestEntry{EntityID: "a", Fingerprint: "one"}
	b := ManifestEntry{EntityID: "b", Fingerprint: "two"}
	first, err := ManifestDigest([]ManifestEntry{a, b})
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := ManifestDigest([]ManifestEntry{b, a})
	if err != nil || first != reversed {
		t.Fatal("manifest depends on enumeration order")
	}
	if _, err := ManifestDigest([]ManifestEntry{a, a}); err == nil {
		t.Fatal("duplicate entity accepted")
	}
	if _, err := ManifestDigest([]ManifestEntry{{EntityID: "a"}}); err == nil {
		t.Fatal("missing fingerprint accepted")
	}
	empty, err := ManifestDigest(nil)
	if err != nil || empty == first {
		t.Fatal("explicit empty manifest not distinguished")
	}
}
