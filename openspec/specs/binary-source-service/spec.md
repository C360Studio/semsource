# binary-source-service Specification

## Purpose
SemSource can serve as a service boundary for binary/protocol source ingestion
without putting raw binary in the graph: it stores payloads by reference and
projects only metadata, hashes, offsets, provenance, findings, and storage
references into triples (verified against `internal/binaryproof/` and the
metadata-only media handlers). Binary handling is memory-bounded, its projections
are governed like any other source, and the SemOps handoff is evidence-bounded —
claiming only what the fixtures demonstrate, with no protocol-conformance overreach.

## Requirements
### Requirement: Binary bytes stay out of graph triples

SemSource MUST store raw binary payloads by reference and project only metadata,
hashes, provenance, offsets, findings, and storage references into graph triples.

#### Scenario: Opaque synthetic binary fixture is ingested

**GIVEN** an opaque synthetic binary fixture is ingested by SemSource
**WHEN** the resulting graph entity is read from ENTITY_STATES
**THEN** no triple object contains the raw binary payload
**AND** the entity includes a storage reference or equivalent by-reference pointer
**AND** the fixture docs do not claim KLV, MISB ST 0601, STANAG 4609, SAPIENT,
SKG, streaming-binary, parser, translation, or protocol conformance

### Requirement: Binary handling is memory bounded

SemSource MUST prove bounded file handling for the synthetic fixture path before
it is described as a binary-source substrate.

#### Scenario: Binary storage path is reviewed

**GIVEN** a synthetic binary source uses SemSource-owned local filestore storage
**WHEN** the source stores or hashes payload bytes
**THEN** tests prove the SemSource-owned path can hash and store via streaming
file reads
**AND** docs state that generic SemStreams `Store.Put` backends remain
byte-slice based until an upstream streaming-write contract exists

### Requirement: Binary source projections are governed

Binary-source metadata entities MUST use the same governed graph migration rules
as ordinary source entities.

#### Scenario: Binary metadata entity is published

**GIVEN** SemSource extracts format-neutral metadata from an opaque synthetic
binary fixture
**WHEN** the metadata is published to graph-ingest
**THEN** the entity is born with a semantic envelope
**AND** the owning projection contract covers the predicates it writes
**AND** the entity uses indexing profile `trace`

### Requirement: SemOps handoff is evidence bounded

SemSource MUST tell SemOps exactly what binary-source evidence exists and what
remains unclaimed.

#### Scenario: SemOps requests binary protocol SKG support

**GIVEN** SemOps asks SemSource to support a binary protocol source
**WHEN** SemSource hands off the proof result
**THEN** the handoff names the synthetic fixture, storage boundary, memory bound,
graph predicates, owner/indexing behavior, and open conformance gaps
**AND** it states that KLV/MISB/STANAG/SAPIENT/SKG parsing and conformance are
SemOps product concerns, not SemSource substrate claims

