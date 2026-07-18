## ADDED Requirements

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
