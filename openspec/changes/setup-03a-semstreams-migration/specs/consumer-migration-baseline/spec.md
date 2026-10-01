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
test-inclusive closures. Independent Go review SHALL approve baseline evidence before accepting SETUP 03A.
Baseline acceptance SHALL remain distinct from qualification of every intended behavior.

#### Scenario: The tier numbers differ

- **WHEN** comparing the lexical SemEngine slice 1
- **THEN** the baseline explicitly names SemSource `tier0-statistical.json`

#### Scenario: A mandatory case is unrun

- **WHEN** required workload, capacity, RPC collision, or provider evidence is unavailable
- **THEN** the result retains the affected evidence gap and does not claim that behavior qualified
- **AND** a separately approved baseline scope SHALL record the gap and its owner rather than count it as a pass

#### Scenario: A reproduced failure is accepted as known at the pin

- **GIVEN** both comparison points retain the failed expectation and reproduced mechanism
- **AND** the owner and independent reviewers approve the measured baseline with that limitation
- **WHEN** preparing the SETUP 03A baseline PR for merge
- **THEN** `broker_restart_reingested_exact_relationship` and `broker_restart_reingested_exact_content`
  remain failed and classified known-at-pin under SemStreams #1442 / SemEngine #15
- **AND** the harness does not skip, weaken or convert those assertions to passing outcomes
- **AND** baseline merge readiness does not require a SemStreams repair or move the frozen pin

#### Scenario: SemEngine Tier 0 consumes the baseline corpus

- **WHEN** SemSource begins 04A after its separate contract and boundary admission
- **THEN** the exclusive SemEngine branch SHALL pass the graph-foundation slice of `test/setup03a`,
  including both named broker-recovery expectations, with a true no-embedder composition
- **AND** known-at-pin baseline classification SHALL NOT excuse a failed engine qualification
- **AND** lexical and neural workloads remain independently qualified at 04B and 04C

### Requirement: The disabled-envelope workaround remains retained

The baseline SHALL retain the tested `Enabled:false` desired-component envelope for file-defined
source removal. SemEngine #17 owns the framework repair for SemStreams #1443; an absent KV key
SHALL NOT be treated as a qualified substitute for the consumer workaround.

#### Scenario: Baseline readiness is recorded without an upstream fix

- **WHEN** the owner accepts baseline merge readiness and ends the SemStreams continuation
- **THEN** the disabled-envelope behavior, pin and existing test expectations remain unchanged
- **AND** later removal of the workaround requires the SemEngine config contract and proving tests

### Requirement: SemStreams remains the shipped substrate

SETUP 03A SHALL retain SemStreams. Later SemEngine qualification SHALL use a separate exclusive
SemEngine branch. Mainline cutover SHALL wait for full semembed-backed and lower-profile workloads.

#### Scenario: Lower engine profiles pass first

- **WHEN** graph/lexical engine profiles pass but neural qualification remains incomplete
- **THEN** SemSource mainline stays on SemStreams
