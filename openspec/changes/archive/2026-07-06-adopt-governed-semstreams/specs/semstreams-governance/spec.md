# SemStreams Governance Adoption

## ADDED Requirements

### Requirement: Current SemStreams target is explicit

SemSource MUST target the latest verified released SemStreams tag for this
migration, not an unreleased local `main` commit.

#### Scenario: Migration target is pinned to a release

**GIVEN** SemStreams remote tags include `v1.0.0-beta.114`
**WHEN** SemSource performs the governed graph migration
**THEN** `go.mod` requires `github.com/c360studio/semstreams v1.0.0-beta.114`

### Requirement: Toolchain gate precedes compatibility work

SemSource MUST run with a Go toolchain that satisfies the SemStreams module
`go` directive before compile compatibility is evaluated.

#### Scenario: Old Go toolchain fails fast

**GIVEN** the local Go toolchain is `go1.25.3`
**WHEN** SemSource tries to compile against SemStreams beta.114
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

Standalone SemSource MUST create the ownership substrate and bind SemSource
projection contracts before graph-ingest starts.

#### Scenario: Graph-ingest sees OWNER_CLAIMS on startup

**GIVEN** SemSource runs in standalone mode
**WHEN** graph-ingest starts
**THEN** OWNER_CLAIMS and OWNER_PRESENCE already exist
**AND** SemSource projection contracts have been registered

### Requirement: Headless mode declares host-owned governance

Headless SemSource MUST make governance ownership explicit because the host app
owns graph infrastructure.

#### Scenario: Headless host has not provided governance

**GIVEN** SemSource runs in headless mode
**WHEN** the host graph substrate has no OWNER_CLAIMS bucket
**THEN** SemSource reports governance-disabled status instead of implying
governed graph enforcement is active

### Requirement: No source relies on triple.add auto-vivify

SemSource MUST NOT rely on `graph.mutation.triple.add` or
`graph.mutation.triple.add_batch` to create entities.

#### Scenario: Derived triple targets an absent entity

**GIVEN** a SemSource source emits a derived triple
**WHEN** the triple subject has not already been born with an envelope
**THEN** the test fails before the source can claim beta.114 compatibility

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
