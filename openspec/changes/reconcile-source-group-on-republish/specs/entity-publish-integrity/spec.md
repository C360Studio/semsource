# entity-publish-integrity

## ADDED Requirements

### Requirement: A republish leaves exactly the statements the source last emitted
When a reconcile-enabled publisher republishes an entity ID whose producer knows the source content changed, the entity's `semsource` statements in contract group `source` SHALL converge to exactly the group statements that publish carried, so a predicate the source stopped emitting is removed. The publisher MUST follow each successful stream publish of a payload not marked `SkipReconcile` with one reconcile of group `source` for that entity, and MUST NOT touch statements outside the group or from any other source. A producer SHALL mark `SkipReconcile` every publish whose content it does not know changed, including the first pass after a start.

#### Scenario: A dropped call and doc comment are removed
- **GIVEN** ast-source has published function `Run` with `code.relationship.calls` and `code.doc.comment`
- **WHEN** the file is edited to drop both and `Run` is republished
- **THEN** the stored entity in `ENTITY_STATES` carries neither predicate
- **AND** it carries the republished `code.metric.start-line`

#### Scenario: A restart over an unchanged corpus makes no reconcile calls
- **GIVEN** ast-source starts over a retained store and an unchanged corpus
- **WHEN** its first pass and any unchanged reindex republish every entity
- **THEN** the publisher sends no reconcile request

#### Scenario: An edit made while semsource was down is the known gap
- **GIVEN** a file edited to drop a predicate while semsource was not running
- **WHEN** semsource starts and its first pass republishes the file
- **THEN** the dropped predicate stays until the file's next edit, which reconciles it

#### Scenario: Other sources and non-group predicates survive
- **GIVEN** an entity carrying a `supersession` lineage edge, a `lifecycle` stale marker and `entity.ontology.class`
- **WHEN** it is republished with fewer `code.*` predicates
- **THEN** the lineage edge, the stale marker and `entity.ontology.class` are unchanged

#### Scenario: A birth is reconciled once its write applies
- **GIVEN** an entity ID that is absent from `ENTITY_STATES` when its reconcile first reads it
- **WHEN** the read reports `entity_not_found`
- **THEN** the publisher counts no error and reads again until the stream write applies, within the reconcile budget
- **AND** the reconcile then commits without removing any statement the publish carried

#### Scenario: A reconcile failure is visible but does not fail delivery
- **GIVEN** a stream publish that succeeded
- **WHEN** its reconcile fails, or its stream write is not applied within the reconcile budget
- **THEN** a reconcile-failure counter moves and the entity still counts as published
- **AND** every re-read is bounded by that budget, and no replace is sent over state the reconcile has not read

#### Scenario: An older queued write cannot restore a dropped predicate
- **GIVEN** two publishes of one entity acknowledged but not yet applied, the newer dropping predicate P
- **WHEN** both apply and the newer publish's reconcile runs
- **THEN** the reconcile waits until the stored state shows the newer write before replacing the group
- **AND** the stored entity does not carry P

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
