// Package source provides the vocabulary predicates SemSource stamps onto
// document, repository, config, media, git, and web entities.
//
// # Semstreams Integration
//
// This package follows semstreams vocabulary patterns:
//   - Predicates use three-level dotted notation (domain.category.property)
//   - Predicates are registered in init() using vocabulary.Register()
//   - IRI mappings use vocabulary.WithIRI() for RDF export compatibility
//   - Metadata includes description, data type, and range where applicable
//
// Every predicate registered here is emitted by a live producer. A predicate
// with no emitter is deleted rather than reserved: an unemitted registration is
// indistinguishable from a capability, and reserving vocabulary "for later" is
// how this package accumulated ~40 dead predicates and a documented entity model
// that no code ever implemented.
//
// # Document Entity Model
//
// A document is ingested as a PARENT entity plus one PASSAGE entity per
// structural section. The split exists because the substrate embeds one vector
// per entity from text truncated at 8000 characters: a whole-file entity both
// dilutes that vector across everything the document says and silently loses
// everything past the cut.
//
//	Parent:  {org}.semsource.web.{system}.doc.{path-slug}
//	  - DocType("document"), DcTitle, DocFilePath, DocMimeType, DocFileHash
//	  - DocChunkCount — how many passages the document currently has
//	  - NO body: a whole-file body would restore the diluted vector and would
//	    return the same prose twice, once as the document and again as its
//	    passages
//
//	Passage: {org}.semsource.web.{system}.chunk.{path-slug}-{ordinal}
//	  - DocType("passage"), DcTitle (qualified by the parent's), DocFilePath
//	  - DocChunkIndex (0-indexed), DocSection when the passage has a heading
//	  - CodeBelongs → the parent, marked as an entity reference
//	  - DocBodyStore / DocBodyKey — the verbatim body handle
//
// Passage identity is (path, ordinal) and nothing else. Deriving it from content
// or from the heading would orphan a passage on every edit or rename, which is
// the failure doc identity itself was already moved away from.
//
// A passage carries its parent's DocFilePath deliberately: the staleness
// lifecycle pass groups by path to decide liveness, and an entity with no path
// predicate is skipped outright. Liveness for a passage is DocChunkIndex <
// DocChunkCount rather than a filesystem check, because a document that shrank
// leaves passages whose file is still very much on disk.
//
// # Web, Repository, Config, Media Sources
//
// Web pages are ingested as single entities carrying URL, content type, ETag,
// and content hash — they are NOT split into passages today. Repository, config,
// and media entities carry identity and provenance facts; media payloads are
// stored by reference, never as triples.
//
// # IRI Mappings
//
// The package registers IRI mappings to standard ontologies:
//   - DocMimeType → dc:format
//   - Passage containment uses BFO part_of/has_part semantics via
//     code.structure.belongs
//
// SemSource-specific predicates use: https://semspec.dev/ontology/source/
package source
