# Delta: entity-staleness

## ADDED Requirements

### Requirement: Stale facts are distinguishable from live facts

The graph SHALL mark entities whose source artifact no longer exists (file deleted or renamed,
source removed, doc superseded by edit) with a governed staleness marker with negative signed salience —
retained and queryable, but demoted and distinguishable on every query surface. A hit SHALL never
present retained-stale knowledge as indistinguishable from live knowledge.

#### Scenario: Deleted file's entities are marked

- **WHEN** a watched/reindexed source file is deleted and the next index pass completes
- **THEN** its entities remain queryable but carry the staleness marker and rank below live
  entities

#### Scenario: Removed source's entities are marked

- **WHEN** a source is deregistered via remove_source
- **THEN** its retained entities carry removal provenance distinguishable by consumers

### Requirement: Document edits supersede instead of orphaning

Editing a document SHALL NOT mint an unrelated new entity while silently retaining the old one;
the document's identity SHALL be stable across edits (path-anchored), with content changes
expressed as supersession/staleness — the collision-prone content-hash-prefix instance is retired.

#### Scenario: Doc edit produces one live entity

- **WHEN** an indexed markdown file is edited and re-ingested
- **THEN** queries surface exactly one live entity for that document, with prior content
  demoted/superseded, not a second co-equal entity
