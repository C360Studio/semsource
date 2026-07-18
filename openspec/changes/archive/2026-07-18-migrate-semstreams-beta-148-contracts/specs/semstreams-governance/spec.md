## MODIFIED Requirements

### Requirement: Current SemStreams target is explicit

SemSource MUST target released SemStreams `v1.0.0-beta.148` for this migration and MUST NOT use a
local replacement, fork, vendored substitute, or unreleased commit as compatibility evidence.

#### Scenario: Migration target is pinned to a release

**GIVEN** SemStreams has released `v1.0.0-beta.148`
**WHEN** SemSource completes this migration
**THEN** `go.mod` requires `github.com/c360studio/semstreams v1.0.0-beta.148`
**AND** the module has no `replace` directive

## ADDED Requirements

### Requirement: Incompatible beta graph state is rebuilt from source

An upgrading deployment MUST stop all writers, capture and review a literal NATS account/resource
inventory, and delete only observed incompatible graph-derived resources before canonical reseed. It
MUST delete `semstreams_config`; observed `GRAPH`; enabled, observed SemStreams
`FrameworkOwnedBuckets`; observed `ENTITY_SUFFIX_INDEX`; observed `GRAPH_INGEST_APPLIED_SEQ`; and
`PREDICATE_CATALOG` only when observed. It MUST preserve authoritative source inputs,
source/content/media/object stores, component status, and unrelated state.

SemSource MUST NOT preserve or rewrite incompatible graph state, run mixed-version writers, or provide
an in-place converter, alias reader, or dual writer.

#### Scenario: Cutover is rehearsed

- **WHEN** operators rehearse the migration in a disposable real-NATS account
- **THEN** all writers are stopped before a literal deletion sheet is executed
- **AND** every removed resource is both observed and in the allowed incompatible set
- **AND** every authoritative or unrelated resource in the preservation inventory remains

#### Scenario: Configuration and graph state are recreated

- **WHEN** deletion is complete
- **THEN** configuration is recreated from the reviewed `semsource.json` through the normal startup
  path
- **AND** only migrated writers start and reseed from authoritative source inputs
- **AND** public status reaches ready and a canonical known-answer query succeeds

#### Scenario: A legacy catalog was not observed

- **WHEN** the captured account inventory does not contain `PREDICATE_CATALOG`
- **THEN** the cutover does not issue a speculative deletion for it
