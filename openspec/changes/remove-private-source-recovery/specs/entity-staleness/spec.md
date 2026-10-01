## MODIFIED Requirements

### Requirement: Stale facts are distinguishable from live facts

SemSource SHALL retain ordinary file-deletion, path-missing and passage-shrink staleness behavior using
its governed graph projection. Retained stale entities SHALL remain distinguishable from live entities.
Automatic whole-source removal marking and positive source reactivation SHALL be unavailable at the
frozen pin after retirement of private recovery authority. A desired removal receipt SHALL NOT imply
new `source_removed` markers or grant freshness to previously marked retained facts.

#### Scenario: Deleted file's entities are marked

- **WHEN** a watched/reindexed source file is deleted and the next index pass completes
- **THEN** its entities remain queryable with the existing staleness marker and ordinary query demotion

#### Scenario: Removed source projection is deferred

- **WHEN** a source is disabled through source management
- **THEN** desired configuration persists and graph history is retained
- **AND** the reply explicitly reports projection unavailable rather than claiming removal provenance was written

#### Scenario: Existing source-removal markers remain sticky

- **GIVEN** retained entities already have `source_removed` markers
- **WHEN** a path reappears, an empty absent-set sweep runs, or the source is re-added
- **THEN** those observations do not clear the markers or claim positive source reactivation
- **AND** ordinary file/passage staleness outside that deferred case remains supported

### Requirement: The staleness pass accepts liveness evidence from sources whose artifacts are not files

The staleness lifecycle pass SHALL accept an explicit set of absent artifact paths from the source
that enumerated them, as an alternative to checking a filesystem root. A source whose artifacts are
not files has no path to stat, and its completed enumeration is authoritative evidence for ordinary
file/path and passage staleness only. It SHALL NOT authorize clearing or replacing `source_removed`.

A request carrying an absent set MUST NOT also carry a filesystem root: two liveness oracles that
disagree SHALL be rejected rather than resolved by precedence.

Every path the request does NOT name MUST be treated as present for ordinary file/path and passage
staleness. Those markers clear on that pass when the existing artifact/passage liveness rules establish
the entity is current, rather than waiting for a re-seed. Existing `source_removed` markers MUST remain
unchanged, including when the absent set is empty.

An empty absent set is a valid assertion that enumeration completed and nothing is gone. It MUST be
distinguishable, over the wire and in memory, from a request making no liveness claim. The former marks
no path absent and preserves passage-liveness checks; a supported scope-wide pass remains subject to
its declared reason and authority. A legacy
request for `source_removed` SHALL be refused rather than treated as ordinary liveness evidence.

#### Scenario: A source states which artifacts are gone

- **WHEN** a source supplies the paths its completed enumeration found absent
- **THEN** in-scope entities carrying those paths receive the ordinary staleness reason
- **AND** entities carrying other paths follow the ordinary artifact/passage liveness rules
- **AND** existing `source_removed` markers remain unchanged

#### Scenario: A document's passages are marked with it

- **WHEN** the absent set names the path of a document that has passage entities
- **THEN** ordinary staleness applies to the document and its passages together
- **AND** existing `source_removed` markers are not replaced

#### Scenario: An artifact reappears

- **WHEN** an entity's path is absent from the set a later completed pass supplies
- **THEN** its ordinary staleness marker clears if artifact/passage liveness rules establish it is current
- **AND** a `source_removed` marker is retained without claiming positive source reactivation

#### Scenario: An empty absent set is not a source removal

- **WHEN** a source supplies an absent set containing no paths
- **THEN** no path is treated as absent, and ordinary artifact/passage liveness checks still apply
- **AND** the pass does not remove the source or clear existing `source_removed` markers

#### Scenario: Two liveness oracles in one request

- **WHEN** a request carries both a filesystem root and an absent set
- **THEN** the request is rejected rather than resolved by precedence
