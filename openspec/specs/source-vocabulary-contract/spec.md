# source-vocabulary-contract Specification

## Purpose
The canonical source vocabulary (`source/vocabulary/`) is the single registry of the predicates,
classes, and ID constructors SemSource emits. Every registered identity is canonical under the
SemStreams beta.148 predicate grammar, unique, and semantically unambiguous, and each one is backed
by a live producer — vocabulary is emitted or deleted, never reserved indefinitely, and the
vocabulary's own documentation describes only entity models a producer actually implements. Each
migrated fact uses exactly one canonical identity across registrations, producers, exact queries,
and fixtures, with no alias, rename map, dual read, or dual write. Guaranteed relationship objects
are canonical six-part entity IDs marked with `EntityReferenceDatatype` while ordinary facts keep
their declared literal datatypes, and every entity intended to be reachable by name carries
`dc.terms.title` stamped at ingest.

## Requirements
### Requirement: The beta.148 migration vocabulary is canonical and complete

SemSource MUST migrate the reviewed set of 90 invalid registered predicates and MUST register the two
emitted body-reference predicates. The resulting 92 target identities MUST be canonical under the
SemStreams beta.148 grammar, unique, registered, and semantically unambiguous.

#### Scenario: Migration contract gate runs

- **WHEN** the reviewed migration ledger and production registrations are compared
- **THEN** exactly 90 retired registrations map to canonical replacements
- **AND** the two body-reference predicates have canonical registrations
- **AND** all 92 target identities are unique and accepted by SemStreams

#### Scenario: Two meanings collide

- **WHEN** two retired facts map to one canonical target
- **THEN** the migration is blocked until distinct identities are reviewed
- **AND** no runtime disambiguation is introduced

### Requirement: Predicate surfaces cut over atomically

Each migrated fact MUST use its one canonical identity in registrations, producers, SemSource exact
queries, and positive fixtures. Production code MUST NOT contain a legacy alias, runtime rename map,
normalizer, dual read, or dual write for the retired set.

#### Scenario: A migrated fact is produced and queried

- **WHEN** a SemSource handler or processor emits one of the migrated facts
- **THEN** the producer uses the registered canonical identity
- **AND** every SemSource exact query and positive fixture for that fact uses the same identity

#### Scenario: Retired identities are scanned

- **WHEN** production and SemSource exact-query code are checked against the reviewed retired set
- **THEN** there are zero behavior-bearing hits
- **AND** any retained occurrence is an explicitly classified negative fixture or historical evidence

### Requirement: Guaranteed relationships use entity-reference datatype

Guaranteed relationship objects MUST be canonical six-part entity IDs marked with SemStreams
`EntityReferenceDatatype`. Ordinary string facts MUST retain their declared literal datatype.
This applies to relationship emission in git entity construction, video entity construction, and
supersession edge construction.

#### Scenario: A guaranteed relationship is emitted

- **WHEN** git, video, or supersession code creates a relationship triple
- **THEN** its object is a canonical six-part entity ID
- **AND** its datatype is `EntityReferenceDatatype`

#### Scenario: A literal resembles an entity ID

- **WHEN** an ordinary path, body, hash, or unresolved value happens to resemble an entity ID
- **THEN** SemSource does not infer reference semantics from string shape

### Requirement: Queryable entities carry a registered title predicate

Every entity type intended to be reachable by name SHALL carry the canonical title predicate
(`dc.terms.title`) stamped at ingest, registered through the canonical vocabulary — config and git
entity vocabularies gain title (and any demotion-marker) predicates as registered vocabulary, not
ad-hoc strings.

#### Scenario: Config dependency entity has a title

- **WHEN** the cfgfile source ingests a go.mod dependency
- **THEN** the resulting entity carries `dc.terms.title` with the dependency's name and is
  resolvable through the name index

#### Scenario: Markers are registered vocabulary

- **WHEN** a demotion or authority marker predicate is emitted by any source
- **THEN** that predicate is registered in the canonical vocabulary with its salience weight

### Requirement: Registered vocabulary is emitted or removed, never reserved indefinitely

Registered source vocabulary SHALL either be emitted by a live producer or be removed — this
applies to every predicate, class, and ID constructor. Vocabulary MUST NOT remain registered
while no code path emits it, and vocabulary documentation MUST NOT describe an entity model that no
producer implements.

#### Scenario: Chunk vocabulary is emitted

- **WHEN** the doc source emits passage entities
- **THEN** `DocChunkIndex`, `DocChunkCount`, and `DocSection` are emitted as triples by that producer
- **AND** each remains registered in the canonical vocabulary

#### Scenario: Unused vocabulary is removed

- **WHEN** a registered predicate, class, or ID constructor has no emitting producer
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
