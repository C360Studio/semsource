## RENAMED Requirements

- FROM: `### Requirement: Current SemStreams target is explicit`
- TO: `### Requirement: Current SemEngine target is explicit`

## MODIFIED Requirements

### Requirement: Current SemEngine target is explicit

SemSource MUST target a released SemEngine tag (currently `v0.1.0-alpha.6`) and MUST NOT use a
local replacement, fork, vendored substitute, or unreleased commit as compatibility evidence.

#### Scenario: Substrate target is pinned to a release

**GIVEN** SemEngine has released the tag SemSource qualifies against
**WHEN** SemSource pins it
**THEN** `go.mod` requires `github.com/c360studio/semengine` at that tag
**AND** the module has no `replace` directive and requires no `github.com/c360studio/semstreams` module

### Requirement: Toolchain gate precedes compatibility work

SemSource MUST run with a Go toolchain that satisfies the `go` directive in its `go.mod`, which
tracks the SemEngine module's directive, before compile compatibility is evaluated.

#### Scenario: Old Go toolchain fails fast

**GIVEN** the local Go toolchain is older than the `go` directive in `go.mod` (`1.26.11`)
**WHEN** SemSource tries to compile against the pinned SemEngine tag
**THEN** the build is blocked until a satisfying toolchain is available

### Requirement: Entity birth carries a semantic envelope

Every SemSource graph entity MUST be born through a SemEngine graph-ingest lane that carries
a valid payload MessageType or explicit entity envelope.

#### Scenario: Source payload enters graph-ingest

**GIVEN** a SemSource source processor publishes an `EntityPayload`
**WHEN** graph-ingest stores the entity in ENTITY_STATES
**THEN** the stored entity has MessageType `semsource.entity.v1`

### Requirement: Source entities declare indexing intent

Every SemSource graph entity MUST declare the SemEngine indexing profile that
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

**GIVEN** SemSource sends a typed mutation through the `semengine.graph.mutation` `v1` port
**WHEN** the mutation fails
**THEN** `entity_not_found`, `revision_mismatch`, and `commit_unknown` are surfaced as distinct
outcomes
**AND** SemSource does not blind-retry the mutation

### Requirement: No source relies on triple.add auto-vivify

SemSource MUST NOT rely on `graph.mutation.triple.add` or
`graph.mutation.triple.add_batch` to create entities.

#### Scenario: Derived triple targets an absent entity

**GIVEN** a SemSource source emits a derived triple
**WHEN** the triple subject has not already been born with an envelope
**THEN** the test fails before the source can claim current SemEngine compatibility

### Requirement: Current query surfaces remain available

SemSource standalone graph mode MUST expose current SemEngine graph query
surfaces needed by SemOps and other consumers. A subject the substrate owns MUST return the
substrate's contract; SemSource's own summary is a distinct surface on a distinct subject, and the
two MUST NOT be conflated.

#### Scenario: Consumer discovers source entities by prefix

**GIVEN** SemSource has ingested source entities
**WHEN** a consumer requests `graph.query.prefix` with a SemSource prefix
**THEN** the response uses the typed paginated prefix contract

#### Scenario: Consumer asks for graph summary

**GIVEN** SemSource has ingested source entities
**WHEN** a consumer requests `graph.query.summary`
**THEN** the response includes entity type examples and predicate summary data
**AND** it is the substrate's handler that answered, deterministically, with no SemSource handler
competing

### Requirement: Incompatible beta graph state is rebuilt from source

An upgrading deployment that crosses a storage break (the SemEngine cutover, ADR-0013) MUST start
on newly provisioned NATS storage: no release-time migration, preservation, wipe, or reseed procedure
exists. If retained deployed state is discovered, that adoption MUST stop and come back as a
separate owner-reviewed migration or recovery design. The graph MUST be re-derived from
authoritative source inputs, which are preserved outside NATS by construction.

SemSource MUST NOT preserve or rewrite incompatible graph state, run mixed-version writers, or
provide an in-place converter, alias reader, or dual writer.

#### Scenario: Adoption starts on fresh storage

- **WHEN** operators adopt a SemSource release across a storage break
- **THEN** NATS storage is newly provisioned (`docker compose down -v` or equivalent)
- **AND** the graph is reseeded from source by the normal continuous ingest path

#### Scenario: Retained state stops the adoption

- **WHEN** retained deployed graph state from before the break is discovered during adoption
- **THEN** that adoption stops rather than migrating, preserving, or wiping in place

### Requirement: KV port declarations conform to the framework bucket catalog

Every KV port subject SemSource declares in its component composition MUST resolve to a bucket in the
SemEngine KV catalog at the pinned target. SemSource MUST NOT declare an output port on a component
that the framework declares as having none, and MUST NOT create or expect creation of a framework
bucket it does not own — readers bind to existing buckets and MUST surface the framework's
not-ready error rather than conjuring an empty bucket.

#### Scenario: An output-less component declares no output ports

**GIVEN** SemSource composes a component the framework declares with no output ports
**WHEN** the component configuration is validated at startup
**THEN** the configuration carries no `ports.outputs` entry
**AND** component creation succeeds

#### Scenario: An off-catalog KV subject fails boot

**GIVEN** a declared KV output port whose subject does not resolve to a catalog bucket
**WHEN** the owning component starts
**THEN** the component fails its start naming the offending subject
**AND** no stray bucket is created

#### Scenario: Querying before the owner has provisioned a bucket

**GIVEN** a bucket whose owning component has not provisioned it in this deployment
**WHEN** a SemSource read path queries through it
**THEN** the caller receives the framework's classified not-ready error naming the owner
**AND** the read path retries with backoff instead of treating the condition as an empty result

### Requirement: The cutover bucket inventory tracks the framework catalog

SemSource's cutover rehearsal MUST assert its literal framework-owned bucket inventory against the
SemEngine catalog at the pinned target, and that assertion MUST fail when the catalog gains or loses
a bucket. The inventory MUST NOT be widened silently to accommodate a catalog change.

#### Scenario: The framework catalog changes

**GIVEN** the SemEngine catalog adds or removes a framework-owned bucket
**WHEN** the cutover rehearsal runs
**THEN** the parity assertion fails and stops the rehearsal for review
**AND** the deletion boundary is not widened until the inventory is updated deliberately

### Requirement: A request subject has exactly one handler

SemSource MUST NOT subscribe a component to a request/reply subject that the pinned SemEngine
target also serves. Where both subscribe, the requester receives whichever handler replies first and
discards the other; when the two return different payload shapes, the result is a nondeterministic
contract that no caller can rely on and no shape assertion can pin.

SemSource MAY continue to serve SemSource-specific concepts on subjects inside the
`graph.query.*` namespace where the substrate defines no such subject, but the burden is on
SemSource to keep them disjoint from the substrate's set at the pinned target.

#### Scenario: An overlapping subscription fails the guard

- **GIVEN** a SemSource component subscribes to a request subject the pinned SemEngine target also
  serves
- **WHEN** the subject-ownership guard runs
- **THEN** it fails, naming the contested subject and both claimants

#### Scenario: A SemSource-specific subject is permitted

- **GIVEN** a subject inside `graph.query.*` that names a SemSource concept the substrate does not
  define
- **WHEN** the guard runs
- **THEN** it passes, because no substrate handler answers that subject

#### Scenario: The source summary is served on a SemSource subject

- **GIVEN** SemSource has ingested source entities
- **WHEN** a consumer requests `graph.query.sourceSummary`
- **THEN** the response is the source-manifest summary — namespace, phase, entity-ID format, domain
  counts, and predicate schema — and no substrate handler competes to answer it

### Requirement: Substrate behavior is fixed upstream, not worked around

Where a substrate default produces an unwanted result, SemSource SHALL correct it by supplying
different configuration through the supported seam. SemSource SHALL NOT reimplement the substrate
behavior locally, and where no configuration seam exists the gap SHALL be raised as a
SemEngine issue.

#### Scenario: A configurable default is corrected by configuration

- **WHEN** an unwanted clustering result traces to a substrate default that is configurable
- **THEN** SemSource supplies a different value and computes no clustering of its own

#### Scenario: A non-configurable defect becomes an upstream issue

- **WHEN** an unwanted result traces to substrate behavior with no configuration seam
- **THEN** it is filed upstream with evidence, and SemSource states the resulting limitation rather
  than substituting a local implementation
