package ontology

import "github.com/c360studio/semstreams/vocabulary"

// rdfType is the RDF type predicate IRI. The entity.ontology.class triple is an
// rdf:type assertion (entity → its BFO/CCO class), so it exports cleanly when an
// optional RDF edge is added later (ADR-0005).
const rdfType = "http://www.w3.org/1999/02/22-rdf-syntax-ns#type"

func init() {
	vocabulary.Register(ClassPredicate,
		vocabulary.WithDescription("BFO/CCO upper-ontology class the entity is an instance of"),
		vocabulary.WithIRI(rdfType))
}
