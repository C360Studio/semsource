# Delta: semstreams-governance (beta.160 migration)

## MODIFIED Requirements

### Requirement: Current SemStreams target is explicit

SemSource MUST target released SemStreams `v1.0.0-beta.160` and MUST NOT use a
local replacement, fork, vendored substitute, or unreleased commit as compatibility evidence.

#### Scenario: Migration target is pinned to a release

**GIVEN** SemStreams has released `v1.0.0-beta.160`
**WHEN** SemSource completes this migration
**THEN** `go.mod` requires `github.com/c360studio/semstreams v1.0.0-beta.160`
**AND** the module has no `replace` directive

### Requirement: Standalone mode declares projection intent before graph-ingest

The SemSource external service MUST declare its local `projection.Contract` intent for every
predicate family it writes before graph-ingest starts, using typed CAS mutations with
`projection.ModeReconcile` for owned current-state predicates and append only for genuinely
append-only evidence. The removed ownership substrate (registries, tokens, heartbeats,
`OWNER_CLAIMS`/`OWNER_PRESENCE`) MUST NOT be recreated in any form, and this behavior is intrinsic
to the sole runtime — never selected through a compatibility mode field or environment variable.

#### Scenario: Graph-ingest starts without an ownership substrate

**GIVEN** SemSource starts as an external service on freshly provisioned NATS storage
**WHEN** graph-ingest starts
**THEN** SemSource's projection contracts are declared locally
**AND** no ownership registry bucket is created or read

#### Scenario: Mutation outcomes are handled distinctly

**GIVEN** SemSource sends a typed mutation through the `semstreams.graph.mutation/v1` port
**WHEN** the mutation fails
**THEN** `entity_not_found`, `revision_mismatch`, and `commit_unknown` are surfaced as distinct
outcomes
**AND** SemSource does not blind-retry the mutation


### Requirement: Incompatible beta graph state is rebuilt from source

An upgrading deployment MUST start beta.160 adoption on newly provisioned NATS storage, per the
upstream adoption contract: no release-time migration, preservation, wipe, or reseed procedure
exists. If retained deployed state is discovered, that adoption MUST stop and come back as a
separate owner-reviewed migration or recovery design. The graph MUST be re-derived from
authoritative source inputs, which are preserved outside NATS by construction.

SemSource MUST NOT preserve or rewrite incompatible graph state, run mixed-version writers, or
provide an in-place converter, alias reader, or dual writer.

#### Scenario: Adoption starts on fresh storage

- **WHEN** operators adopt the beta.160-pinned SemSource on a deployment
- **THEN** NATS storage is newly provisioned (`docker compose down -v` or equivalent)
- **AND** the graph is reseeded from source by the normal continuous ingest path

#### Scenario: Retained state stops the adoption

- **WHEN** retained deployed graph state from an earlier beta is discovered during adoption
- **THEN** that adoption stops rather than migrating, preserving, or wiping in place
