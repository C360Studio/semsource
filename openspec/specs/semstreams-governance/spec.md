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

SemSource MUST target released SemStreams `v1.0.0-beta.159` for this migration and MUST NOT use a
local replacement, fork, vendored substitute, or unreleased commit as compatibility evidence.

#### Scenario: Migration target is pinned to a release

**GIVEN** SemStreams has released `v1.0.0-beta.159`
**WHEN** SemSource completes this migration
**THEN** `go.mod` requires `github.com/c360studio/semstreams v1.0.0-beta.159`
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

An upgrading deployment MUST stop all writers, capture and review a literal NATS account/resource
inventory, and delete only observed incompatible graph-derived resources before canonical reseed. It
MUST derive the framework-owned portion of that deletion set from the SemStreams KV bucket catalog at
the pinned target under each bucket's resolved name, rather than from a copied or remembered literal
list. It MUST delete `semstreams_config`; observed `GRAPH`; every enabled, observed catalog bucket;
and `PREDICATE_CATALOG` only when observed. It MUST preserve authoritative source inputs,
source/content/media/object stores, component status, and unrelated state. It MUST NOT apply a
wildcard deletion or a copied default list to a shared account.

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

#### Scenario: Replay parity is proven after reseed

- **WHEN** the reseeded deployment is restarted once with no intervening write
- **THEN** the canonical known-answer query returns the same result as before the restart

#### Scenario: A legacy catalog was not observed

- **WHEN** the captured account inventory does not contain `PREDICATE_CATALOG`
- **THEN** the cutover does not issue a speculative deletion for it

#### Scenario: Entity-state history depth is reconciled destructively

- **GIVEN** a live `ENTITY_STATES` bucket carrying a History depth greater than the catalog
  declaration
- **WHEN** the migrated deployment boots for the first time
- **THEN** the framework reconciles the bucket down to the declared History, discarding stored
  revisions beyond that depth
- **AND** any out-of-tree tooling that replays entity-state history has captured what it needs before
  the upgrade

### Requirement: KV port declarations conform to the framework bucket catalog

Every KV port subject SemSource declares in its component composition MUST resolve to a bucket in the
SemStreams KV catalog at the pinned target. SemSource MUST NOT declare an output port on a component
that the framework declares as having none, and MUST NOT create or expect creation of a framework
bucket it does not own — readers bind to existing buckets and MUST surface the framework's
not-ready error rather than conjuring an empty bucket.

#### Scenario: Embedding component declares no output ports

**GIVEN** SemSource composes the graph-embedding component
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
SemStreams catalog at the pinned target, and that assertion MUST fail when the catalog gains or loses
a bucket. The inventory MUST NOT be widened silently to accommodate a catalog change.

#### Scenario: The framework catalog changes

**GIVEN** the SemStreams catalog adds or removes a framework-owned bucket
**WHEN** the cutover rehearsal runs
**THEN** the parity assertion fails and stops the rehearsal for review
**AND** the deletion boundary is not widened until the inventory is updated deliberately

### Requirement: A request subject has exactly one handler

SemSource MUST NOT subscribe a component to a request/reply subject that the pinned SemStreams
target also serves. Where both subscribe, the requester receives whichever handler replies first and
discards the other; when the two return different payload shapes, the result is a nondeterministic
contract that no caller can rely on and no shape assertion can pin.

SemSource MAY continue to serve SemSource-specific concepts on subjects inside the
`graph.query.*` namespace where the substrate defines no such subject, but the burden is on
SemSource to keep them disjoint from the substrate's set at the pinned target.

#### Scenario: An overlapping subscription fails the guard

- **GIVEN** a SemSource component subscribes to a request subject the pinned SemStreams target also
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

