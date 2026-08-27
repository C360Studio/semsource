# entity-staleness Specification

## Purpose
Keep the retention-first graph honest about time: entities whose source
artifact no longer exists stay retained and queryable but carry a governed,
negatively-weighted staleness marker — a hit is never silently a phantom, and
deletion remains the rare graph-aware exception gated upstream.

## Requirements

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

### Requirement: The staleness pass accepts liveness evidence from sources whose artifacts are not files

The staleness lifecycle pass SHALL accept an explicit set of absent artifact paths from the source
that enumerated them, as an alternative to checking a filesystem root. A source whose artifacts are
not files — an object store answers only when listed — has no path to stat, and its own completed
enumeration is the authoritative liveness evidence.

A request carrying an absent set MUST NOT also carry a filesystem root: two liveness oracles that
disagree resolve silently, and the visible symptom is entities that will not retract or will not
return.

Every path the request does NOT name MUST be treated as present, so an artifact that reappears
clears its marker on the same pass that observes it rather than waiting for a re-seed.

An empty absent set is a valid assertion — the enumeration completed and nothing is gone — and MUST
be distinguishable, over the wire as well as in memory, from a request making no liveness claim at
all. The two lead to opposite outcomes: the first marks nothing, the second marks every entity in
scope.

#### Scenario: A source states which artifacts are gone

- **WHEN** a source triggers the staleness pass with the set of paths its completed enumeration
  found absent
- **THEN** every in-scope entity carrying one of those paths is marked stale with the given reason
- **AND** entities carrying any other path are left alone

#### Scenario: A document's passages are marked with it

- **WHEN** the absent set names the path of a document that has passage entities
- **THEN** the document entity and all of its passage entities are marked stale together

#### Scenario: An artifact reappears

- **WHEN** a previously marked entity's path is absent from the set a later completed pass supplies
- **THEN** its staleness marker is cleared

#### Scenario: An empty absent set is not a source removal

- **WHEN** a source supplies an absent set containing no paths
- **THEN** no entity is marked stale
- **AND** the pass is not treated as the removal of the source itself

#### Scenario: Two liveness oracles in one request

- **WHEN** a request carries both a filesystem root and an absent set
- **THEN** the request is rejected rather than resolved by precedence
