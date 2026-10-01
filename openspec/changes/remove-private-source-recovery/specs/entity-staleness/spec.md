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
