## ADDED Requirements

### Requirement: Migration comparisons use immutable inputs

A substrate migration SHALL record exact consumer/substrate snapshots, module version, corpus version,
configuration, and broker storage policy for both runs. The baseline SHALL run before the dependency
changes. Provider-backed comparisons SHALL hold service build/image, actual model artifact/version,
vector dimensions, and query preprocessing constant.

#### Scenario: The target tag is absent when migration begins

- **GIVEN** beta.163 is absent at SETUP 03A start
- **WHEN** selecting the dependency
- **THEN** newest main at that time is recorded by exact SHA as the shared migration/extraction baseline
- **AND** later upstream changes do not silently move the pin

#### Scenario: Actual provider identity is unknown

- **WHEN** build, model artifact, dimensions, or preprocessing cannot be established
- **THEN** the provider comparison is blocked and not counted as passing neural evidence

### Requirement: Known answers establish consumer behavior

The versioned corpus SHALL cover expected entities/relationships, exact/scoped queries, provenance,
content, updates, deletion/recreation, application restart, and persistent broker restart. Assertions
SHALL use retained public interfaces, bounded completion, and explicit absence checks. Deletion SHALL
follow the admitted retention/stale/visibility contract.

#### Scenario: Startup succeeds but content is absent

- **GIVEN** the process reports ready
- **WHEN** an expected entity, relationship, provenance record, or body is missing
- **THEN** the workload fails rather than treating readiness as semantic correctness

#### Scenario: A source is deleted and recreated

- **WHEN** an ingested source is deleted and then recreated
- **THEN** visibility and history are checked against the recorded deletion contract
- **AND** recreation restores intended identity/bytes without duplicate current entities

#### Scenario: Broker restarts before indexing finishes

- **GIVEN** accepted graph work and actual transport storage policy are recorded
- **WHEN** the broker restarts from the same persistent store before indexing finishes
- **THEN** the workload proves the declared durable-effect or replay/re-ingestion recovery path
- **AND** memory-transport acknowledgement is not asserted to prove durable completion

### Requirement: Profiles, differences, and acceptance remain explicit

The migration SHALL map SemSource configs to SemEngine slices, distinguish structural/BM25/provider
proof, attribute every difference to a pinned contract or reproduced defect, and record production and
test-inclusive closures. Independent Go review SHALL approve evidence before SETUP 03A completion.

#### Scenario: The tier numbers differ

- **WHEN** comparing the lexical SemEngine slice 1
- **THEN** the baseline explicitly names SemSource `tier0-statistical.json`

#### Scenario: A mandatory case is unrun

- **WHEN** required workload, capacity, RPC collision, or provider evidence is unavailable
- **THEN** the result retains a blocker and does not claim full SETUP 03A acceptance

### Requirement: SemStreams remains the shipped substrate

SETUP 03A SHALL retain SemStreams. Later SemEngine qualification SHALL use a separate exclusive
SemEngine branch. Mainline cutover SHALL wait for full semembed-backed and lower-profile workloads.

#### Scenario: Lower engine profiles pass first

- **WHEN** graph/lexical engine profiles pass but neural qualification remains incomplete
- **THEN** SemSource mainline stays on SemStreams
