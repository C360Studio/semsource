## ADDED Requirements

### Requirement: Registered vocabulary is emitted or removed, never reserved indefinitely

Registered source vocabulary SHALL either be emitted by a live producer or be removed — this
applies to every predicate, class, and ID constructor. Vocabulary MUST NOT remain registered
while no code path emits it, and vocabulary documentation MUST NOT describe an entity model that no
producer implements.

#### Scenario: Chunk vocabulary becomes live

- **WHEN** the doc source emits passage entities
- **THEN** `DocChunkIndex`, `DocChunkCount`, and `DocSection` are emitted as triples by that producer
- **AND** each remains registered in the canonical vocabulary

#### Scenario: Unused vocabulary is removed

- **WHEN** a registered predicate, class, or ID constructor has no emitting producer after this
  change
- **THEN** it is deleted rather than left registered

#### Scenario: Vocabulary documentation matches the implementation

- **WHEN** the source vocabulary documents an entity model
- **THEN** a live producer emits entities matching that model, including the described identity shape

### Requirement: Passage entities are name-reachable

Passage entities SHALL carry the canonical title predicate `dc.terms.title`, stamped at ingest and
registered through the canonical vocabulary, so a passage is resolvable through the name index like
any other queryable entity.

#### Scenario: A passage is resolved by name

- **WHEN** a passage entity is published
- **THEN** it carries `dc.terms.title` and is resolvable through the name index

#### Scenario: Passage containment uses entity-reference datatype

- **WHEN** a passage emits its containment triple naming its parent document
- **THEN** the object is a canonical six-part entity ID marked with `EntityReferenceDatatype`
