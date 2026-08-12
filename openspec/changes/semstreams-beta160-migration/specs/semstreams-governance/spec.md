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
