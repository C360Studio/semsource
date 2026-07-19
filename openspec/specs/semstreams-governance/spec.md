# semstreams-governance Specification

## Purpose
SemSource runs against the governed SemStreams substrate and binds its own source
projection authority. Every graph write carries a semantic envelope (no reliance on
`triple.add` auto-vivify, post-ADR-055), source entities declare an indexing
profile, and in standalone mode SemSource bootstraps ownership (OWNER_CLAIMS /
OWNER_PRESENCE) before graph-ingest starts. It pins an explicit SemStreams target
with a toolchain gate, and keeps the current query surfaces available.
## Requirements
### Requirement: Current SemStreams target is explicit

SemSource MUST target released SemStreams `v1.0.0-beta.153` for this migration and MUST NOT use a
local replacement, fork, vendored substitute, or unreleased commit as compatibility evidence.

#### Scenario: Migration target is pinned to a release

**GIVEN** SemStreams has released `v1.0.0-beta.153`
**WHEN** SemSource completes this migration
**THEN** `go.mod` requires `github.com/c360studio/semstreams v1.0.0-beta.153`
**AND** the module has no `replace` directive

### Requirement: Toolchain gate precedes compatibility work

SemSource MUST run with a Go toolchain that satisfies the SemStreams module
`go` directive before compile compatibility is evaluated.

#### Scenario: Old Go toolchain fails fast

**GIVEN** the local Go toolchain is `go1.25.3`
**WHEN** SemSource tries to compile against the current SemStreams beta
**THEN** the migration is blocked until Go `1.26.3` is available

### Requirement: Entity birth carries a semantic envelope

Every SemSource graph entity MUST be born through a SemStreams lane that carries
a valid payload MessageType or explicit entity envelope.

#### Scenario: Source payload enters graph-ingest

**GIVEN** a SemSource source processor publishes an `EntityPayload`
**WHEN** graph-ingest stores the entity in ENTITY_STATES
**THEN** the stored entity has MessageType `semsource.entity.v1`

### Requirement: Source entities declare indexing intent

Every SemSource graph entity MUST declare the SemStreams indexing profile that
matches its retrieval role.

#### Scenario: Human-authored source text enters graph-ingest

**GIVEN** a SemSource document, URL text, transcript, OCR output, or source
comment entity is emitted
**WHEN** graph-ingest stores the entity
**THEN** the entity has indexing profile `content`

#### Scenario: Operational source metadata enters graph-ingest

**GIVEN** a SemSource manifest, config, commit identity, decode trace, or parser
diagnostic entity is emitted
**WHEN** graph-ingest stores the entity
**THEN** the entity has indexing profile `control` or `trace`

### Requirement: Standalone mode boots ownership before graph-ingest

The SemSource external service MUST create the ownership substrate and bind SemSource projection
contracts before graph-ingest starts. This behavior is intrinsic to the sole runtime and MUST NOT be
selected through a compatibility mode field or environment variable.

#### Scenario: Graph-ingest sees OWNER_CLAIMS on startup

**GIVEN** SemSource starts as an external service
**WHEN** graph-ingest starts
**THEN** OWNER_CLAIMS and OWNER_PRESENCE already exist
**AND** SemSource projection contracts have been registered

### Requirement: No source relies on triple.add auto-vivify

SemSource MUST NOT rely on `graph.mutation.triple.add` or
`graph.mutation.triple.add_batch` to create entities.

#### Scenario: Derived triple targets an absent entity

**GIVEN** a SemSource source emits a derived triple
**WHEN** the triple subject has not already been born with an envelope
**THEN** the test fails before the source can claim current SemStreams compatibility

### Requirement: Current query surfaces remain available

SemSource standalone graph mode MUST expose current SemStreams graph query
surfaces needed by SemOps and other consumers.

#### Scenario: Consumer discovers source entities by prefix

**GIVEN** SemSource has ingested source entities
**WHEN** a consumer requests `graph.query.prefix` with a SemSource prefix
**THEN** the response uses the typed paginated prefix contract

#### Scenario: Consumer asks for graph summary

**GIVEN** SemSource has ingested source entities
**WHEN** a consumer requests `graph.query.summary`
**THEN** the response includes entity type examples and predicate summary data

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
