package ontology

import (
	"time"

	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semsource/entityid"
)

// StampClass returns triples with one entity.ontology.class triple appended,
// aligning the entity to its BFO/CCO class (derived from the domain + type
// segments of its 6-part ID via ClassFor). It is a no-op when the kind has no
// mapping, or when a class triple is already present — that pre-existing triple
// is treated as an explicit per-source override and left untouched. The input
// slice is never mutated. One triple per entity: identity-level, low cardinality.
func StampClass(id string, triples []message.Triple, updatedAt time.Time) []message.Triple {
	for i := range triples {
		if triples[i].Predicate == ClassPredicate {
			return triples
		}
	}
	domain, typ := entityid.Parts(id)
	iri, ok := ClassFor(domain, typ)
	if !ok {
		return triples
	}
	out := make([]message.Triple, len(triples), len(triples)+1)
	copy(out, triples)
	return append(out, message.Triple{
		Subject:    id,
		Predicate:  ClassPredicate,
		Object:     iri,
		Source:     entityid.PlatformSemsource,
		Timestamp:  updatedAt,
		Confidence: 1.0,
	})
}
