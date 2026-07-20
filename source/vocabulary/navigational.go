package source

import "github.com/c360studio/semstreams/vocabulary"

// EntityRoleNavigational marks an entity that exists to be navigated to, not to
// be read as evidence: it carries identity, provenance and edges, but no
// retrievable body.
//
// The parent document entity is the case this exists for. Passage chunking moved
// document bodies onto passage entities and left the parent as the stable
// navigational node — but a producer cannot opt an entity out of embedding
// (graph-embedding's text_suffixes include ".title", and its indexingEligible is
// lenient by design, ADR-054 Phase 1), so the parent still carries a title-only
// vector. A query whose wording resembles a document's title then scores that
// empty node above the passage that actually answers it, and the caller's first
// citation contains nothing.
//
// That is not hypothetical: measured on this repository, 5 of 11 doc_context
// answers led with an empty-bodied node once passages shipped, where none did
// before (scripts/scorecard/results/SUMMARY.md).
//
// Registered WithWeight(-2.0): the same demotion tier as code.artifact.test and
// code.lineage.superseded-by, and deliberately ABOVE entity.lifecycle.stale's
// -3.0. A navigational node is live and correct — it is simply not evidence — so
// it must not sink below a phantom whose artifact is gone.
//
// Presence-based; the value names what kind of navigational node it is.
const EntityRoleNavigational = "entity.role.navigational"

// Values for EntityRoleNavigational.
const (
	// NavigationalDocument is a parent document whose bodies live on its passages.
	NavigationalDocument = "document"
)

func init() {
	registerNavigationalPredicates()
}

func registerNavigationalPredicates() {
	vocabulary.Register(EntityRoleNavigational,
		vocabulary.WithDescription("Marks an entity that carries identity and edges but no retrievable body, so it ranks below entities that do"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"roleNavigational"),
		vocabulary.WithWeight(-2.0))
}
