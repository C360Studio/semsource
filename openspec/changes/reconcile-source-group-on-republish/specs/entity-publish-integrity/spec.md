# entity-publish-integrity

## ADDED Requirements

### Requirement: A republish leaves exactly the statements the source last emitted
When a reconcile-enabled publisher republishes an entity ID, the entity's `semsource` statements in contract group `source` SHALL converge to exactly the group statements that publish carried, so a predicate the source stopped emitting is removed. The publisher MUST follow each successful stream publish with one reconcile of group `source` for that entity, and MUST NOT touch statements outside the group or from any other source.

#### Scenario: A dropped call and doc comment are removed
- **GIVEN** ast-source has published function `Run` with `code.relationship.calls` and `code.doc.comment`
- **WHEN** the file is edited to drop both and `Run` is republished
- **THEN** the stored entity in `ENTITY_STATES` carries neither predicate
- **AND** it carries the republished `code.metric.start-line`

#### Scenario: Other sources and non-group predicates survive
- **GIVEN** an entity carrying a `supersession` lineage edge, a `lifecycle` stale marker and `entity.ontology.class`
- **WHEN** it is republished with fewer `code.*` predicates
- **THEN** the lineage edge, the stale marker and `entity.ontology.class` are unchanged

#### Scenario: A birth needs no reconcile write
- **GIVEN** an entity ID that is absent from `ENTITY_STATES`
- **WHEN** the reconcile reports `entity_not_found`
- **THEN** the publisher treats it as nothing to clear and counts no error

#### Scenario: A reconcile failure is visible but does not fail delivery
- **GIVEN** a stream publish that succeeded
- **WHEN** its reconcile fails or loses a revision race
- **THEN** a reconcile-failure counter moves and the entity still counts as published
- **AND** the reconcile is not blind-retried

### Requirement: Reconcile is enabled only for handlers proven to publish complete sets
A publisher SHALL reconcile only when its handler opts in, and a handler MUST opt in only after a test proves that each of its publishes carries the entity's complete `semsource` statement set for group `source` (one writer per entity ID). The supersession publisher SHALL NOT reconcile. AST `folder` and `repo` entities SHALL NOT be reconciled while the watch path republishes them with a partial `code.contains` set (#347).

#### Scenario: A handler without the completeness test does not reconcile
- **GIVEN** a handler whose publisher was built without the reconcile option
- **WHEN** it republishes an entity with fewer predicates
- **THEN** no reconcile request is sent for that entity

#### Scenario: Watch-path folder republish is not reconciled
- **GIVEN** a reconcile-enabled ast-source
- **WHEN** a file edit republishes its ancestor `folder` entities
- **THEN** no reconcile request is sent for any `folder` or `repo` entity
