## ADDED Requirements

### Requirement: Consumer implementation waits for admitted engine contracts

The consumer qualification SHALL record approved SemEngine 03B contracts, port scope, repair decisions
and an exact usable Tier 0 source commit before code migration. A planning branch SHALL NOT claim
engine availability or qualification merely because the reference SemStreams binary builds.

#### Scenario: Only the engine harness is available

- **GIVEN** 03B contract admission remains open and engine main contains only foundation/harness packages
- **WHEN** preparing the SemSource 04A consumer claim
- **THEN** planning and inventory may proceed, but runtime/API migration remains blocked
- **AND** no missing engine API is guessed or implemented in the consumer

### Requirement: Tier 0 qualifies an exclusive no-embedder composition

The admitted Tier 0 consumer binary SHALL use SemEngine exclusively with no SemStreams runtime and no
embedding/model-service requirement. Its declared public interfaces and readiness SHALL match the
admitted graph-foundation contract; lexical/neural APIs are separate qualification slices.

#### Scenario: The reference structural checks run under BM25

- **WHEN** comparing the reference to the future Tier 0 composition
- **THEN** structural behavior under the reference BM25 config is comparison evidence only
- **AND** the actual engine binary SHALL independently prove a no-embedder composition

### Requirement: Known answers retain their positive acceptance obligations

The admitted graph-foundation slice of `test/setup03a` SHALL preserve exact identity, provenance,
relationship and content expectations through updates, deletion/recreation and both restart modes.
The disabled-component envelope SHALL remain until the admitted config contract proves its replacement.

#### Scenario: Broker-recovery cases are known failures at the reference pin

- **WHEN** running Tier 0 qualification on SemEngine
- **THEN** `broker_restart_reingested_exact_relationship` and
  `broker_restart_reingested_exact_content` SHALL both pass
- **AND** the reference known-at-pin disposition SHALL NOT skip or weaken either assertion

### Requirement: Mainline cutover qualifies Tier 0 and BM25 while deferring neural 04C

SemSource mainline SHALL cut over to SemEngine once the Tier 0 graph-foundation corpus
(`configs/tiers/tier0-foundation.json`, `embedder_type: "none"`), the BM25 statistical smoke suites
(`semsource.json`, `embedder_type: "bm25"`), and independent review pass. Neural HTTP embedding
qualification (`semengine#489`, 04C), Tier 2 LLM community summaries (`semengine#485`), and
SemEngine's separate SemConnect/dogfood obligations SHALL remain tracked in their respective slices
without being claimed by the Tier 0 cutover.

#### Scenario: Tier 0 and BM25 pass before neural 04C admission

- **WHEN** the SemEngine consumer branch passes the admitted Tier 0 graph-foundation workload and BM25 smoke suites
- **THEN** mainline cuts over to SemEngine for Tier 0 (`none`) and statistical (`bm25`) compositions
- **AND** neural (`http`) and LLM community-summary qualification remain deferred to 04C (`semengine#489`, `semengine#485`)
