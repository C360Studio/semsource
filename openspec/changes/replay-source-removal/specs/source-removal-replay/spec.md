## ADDED Requirements

### Requirement: Removal intent is durable before desired acknowledgment

SemSource SHALL persist a revision-fenced removal record before acknowledging desired removal.
It SHALL preserve the disabled component envelope and distinguish desired change from effective
runtime and graph projection. Partial desired writes SHALL be repairable and observable.

#### Scenario: Original-file source removal

- **GIVEN** an original JSON source was admitted in the sealed boot composition
- **WHEN** its removal is acknowledged
- **THEN** the durable journal and disabled desired envelope exist
- **AND** the reply reports restart required and pending graph projection
- **AND** the running producer remains admitted until process retirement

#### Scenario: Crash between desired writes

- **WHEN** the application crashes after journal preparation or disabled-envelope persistence
- **THEN** restart rereads the journal and exact desired config to repair only committed removal
- **AND** an enabled replacement or ambiguous state never authorizes an old graph removal

### Requirement: Projection waits for retirement and settled graph ingestion

Within the supported single-process-per-authority replacement boundary, removal projection SHALL
require the removed producer absent from the immutable boot snapshot and matching current desired
intent. It SHALL require authoritative current graph-consumer settlement for a retained-state pass,
not cached readiness or transport publication acknowledgments. Terminal removal completion SHALL
additionally require a supported proof of the full applied/unresolved-input set. On the frozen pin
that proof is unavailable: a successful current retained pass SHALL remain pending with
applied_tail_unproven and periodic repair. Unknown ownership, backlog, or stream generation SHALL defer.

#### Scenario: Desired removal in the requesting boot

- **WHEN** a source is disabled in desired state while its old boot producer still runs
- **THEN** no removal marker is projected by that intent

#### Scenario: Retained delivery after process death

- **GIVEN** the previous process has exited and retained entity delivery is pending
- **WHEN** the replacement boots without that source
- **THEN** removal remains pending until relevant delivery settles and authoritative query succeeds
- **AND** failed observations or unresolved source ingestion errors cannot produce complete status

#### Scenario: Parked work is invisible to current counters

- **GIVEN** the old process retired and qualified current pending counters are zero
- **WHEN** the pin provides no complete applied/unresolved-input proof
- **THEN** a retained-state pass may mark its exact currently retained set
- **AND** the intent remains pending with applied_tail_unproven even when that set is empty
- **AND** AckFloor, missing advisories and private guard decoding cannot manufacture completion

#### Scenario: An entity arrives after retained-state convergence

- **GIVEN** removal remains pending with applied_tail_unproven and its producer stays absent
- **WHEN** an entity later arrives in its proven source scope
- **THEN** periodic repair includes that entity in a later retained-state pass
- **AND** the prior partial progress was never reported as terminal removal completion

### Requirement: Exact complete scope is the completion boundary

Replay SHALL use exact retained authority, source identity, taxonomy and proven artifact ownership.
It SHALL traverse all cursor pages, mark parents and passages with source_removed through governed
mutation, preserve retained history, and propagate incomplete enumeration or mutation. Ambiguous
same-taxonomy ownership SHALL fail visibly without marking live siblings.

#### Scenario: Expanded-repo sibling isolation

- **WHEN** one source child is removed while another taxonomy for the same repository remains active
- **THEN** only the removed child's proven entities carry source_removed
- **AND** sibling source facts, content references and lifecycle remain unchanged
- **AND** known AST `dc.terms.created` indexing timestamps are recorded separately across reseed;
  they are not source-content changes or freshness proof

#### Scenario: More than one page or partial mutation failure

- **WHEN** the affected set spans pages or a later query/mutation fails
- **THEN** opaque cursors are followed until exhausted and partial effects remain pending on failure
- **AND** retry converges without duplicating effects or declaring a truncated result complete

#### Scenario: Legacy liveness cannot revive removed history

- **GIVEN** retained entities carry source_removed, including an old symbol whose file still exists
- **WHEN** a legacy filesystem, empty-Absent, or passage-count lifecycle pass observes a present path
- **THEN** source_removed remains unchanged
- **AND** only admitted current-publication reactivation can grant freshness to those entities

#### Scenario: Indistinguishable source ownership

- **GIVEN** two admitted source configs can emit indistinguishable entities into the proposed scope
- **WHEN** removal replay cannot establish exclusive ownership
- **THEN** it retains pending work and reports ambiguous_source_scope
- **AND** it does not broaden the scope or silently mark shared live entities

### Requirement: Re-add supersedes old intent and refreshes only current entities

A same-handle re-add SHALL fence every older removal generation before it can mutate the replacement.
After applied removal, freshness SHALL be granted only to exact current entities proven by the new
producer's CURRENT boot seed epoch, successfully sealed current-ID manifest, acknowledged publication
receipts, continuing publication eligibility, and authoritative source-fact match. A clear SHALL be
conditioned on the SAME nonzero exact revision whose source facts and marker were checked. The frozen
public client cannot express this condition; SemSource SHALL issue no production clear and SHALL report
conditional_reconcile_unavailable for a matching source_removed entity (SemStreams #1445).
Prior-process receipts are ineligible. Lifecycle-only UpdatedAt changes, path existence, or source
admission SHALL NOT prove current entity publication.

#### Scenario: Re-add before first restart

- **WHEN** a source is removed and re-added while the original boot remains running
- **THEN** the old removal operation is superseded
- **AND** its redelivery cannot mark the replacement's entities

#### Scenario: Re-add after completed removal

- **GIVEN** an admitted public conditional-reconcile capability and proven current publication eligibility
- **WHEN** the re-added source publishes current parents and passages under their stable IDs
- **THEN** their source_removed markers are cleared after verified publication effects
- **AND** vanished documents, symbols and tail passages stay retained stale
- **AND** a newer different stale reason is not cleared by an older publication receipt
- **AND** this positive acceptance remains blocked at the frozen pin, not qualified by refusal to clear

#### Scenario: A checked source fact changes before reconcile

- **GIVEN** a current receipt matches source facts at exact revision R
- **WHEN** another graph-ingest write changes the entity before the public client rereads it internally
- **THEN** SemSource does not use that client's newer revision to authorize a clear
- **AND** the marker remains unchanged with conditional_reconcile_unavailable at the frozen pin
- **AND** no raw mutation-subject shim, copied internal client, or direct KV write bypasses that gate

#### Scenario: Re-add crashes before selective freshness completes

- **GIVEN** an earlier re-add boot published A and B but died before clearing their removal markers
- **WHEN** B disappears offline and the replacement seed publishes only A
- **THEN** the replacement invalidates earlier receipt eligibility before producer Start
- **AND** only the new successful seed manifest can authorize freshness, so B remains stale

#### Scenario: Re-add follows partially applied removal

- **WHEN** a removal marked only part of its scope before failing and the source is re-added
- **THEN** the new generation still performs selective current-entity reactivation
- **AND** rapid remove/add toggles cannot erase the need to clear earlier applied removal markers

### Requirement: Current-epoch batch inventory preserves independent initial proof

SemSource SHALL renew the publication epoch before each boot admission for every matching enabled
Reactivate history, including records previously Complete. It SHALL reset initial proof and pending
state without making prior-epoch receipts eligible. ListSeeds SHALL return only independently validated
terminal manifests for the exact binding and epoch; malformed or partial data SHALL remain an error.
Periodic repair SHALL consider subsequent batches even after prior batch completion.

A separately sealed live target SHALL NOT amend or complete its initial seed. Its request SHALL carry
an independently successful same-epoch InitialManifest and exact InitialReceipts in addition to the
selected target Manifest/Receipts. The projector SHALL validate both proof sets rather than trusting a
boolean or removing the Initial check. At the frozen pin, these proofs admit inventory and exact reads
only; they SHALL NOT authorize production freshness or hide conditional_reconcile_unavailable.

#### Scenario: A completed history restarts with fewer current entities

- **GIVEN** an earlier boot completed a reactivation batch containing A and B
- **WHEN** the same enabled source restarts and its new seed contains only A
- **THEN** a new epoch and pending initial proof are persisted before producer Start
- **AND** earlier B receipts cannot authorize any freshness in the new epoch

#### Scenario: Live publication follows an unsuccessful initial seed

- **GIVEN** the current epoch's initial enumeration failed or never sealed
- **WHEN** a separate live batch successfully publishes B
- **THEN** its seal remains separate and cannot satisfy the initial prerequisite
- **AND** reactivation remains pending without any lifecycle clear

#### Scenario: Later recreation is inventoried without claiming freshness

- **GIVEN** the current epoch has a successful A-only initial seed
- **WHEN** B later publishes in a separately sealed acknowledged live batch
- **THEN** periodic inventory considers the exact B batch and independently validates the initial proof
- **AND** the frozen adapter remains pending for source_removed B and issues no clear
- **AND** positive live-B freshness remains intended acceptance behind the mutation and eligibility gates

#### Scenario: A historic receipt outlives publication eligibility

- **GIVEN** a sealed B receipt is followed by deletion, withdrawal, or failed current enumeration
- **WHEN** B's retained source facts still match that receipt and source_removed remains sticky
- **THEN** historic matching and unchanged revision alone do not authorize a clear
- **AND** publication/withdrawal eligibility remains an explicit unqualified gate, not inferred from
  BatchID spelling or KV listing order

### Requirement: Failed work repairs continuously and remains observable

The journal owner SHALL reconcile current state on startup and periodically with bounded contexts,
explicit generation CAS, visible pending/error/progress status, and Start-owned shutdown/join.
KV watch delivery SHALL NOT substitute for failed-work repair. No uncompleted correctness-critical
intent or receipt SHALL expire or be evicted automatically.

#### Scenario: Mutation service recovers without another request

- **WHEN** projection fails and its dependency later recovers while the application remains running
- **THEN** periodic repair retries the retained intent and records current retained progress
- **AND** terminal completion additionally requires supported applied-tail proof
- **AND** the user need not resend removal or restart solely to recover the projection

#### Scenario: Shutdown interrupts replay

- **WHEN** bounded shutdown cannot finish accepted replay work
- **THEN** it remains durable and incomplete for the replacement process
- **AND** the stopping process reports failure honestly and joins its owned work

### Requirement: Generation authorization survives caller loss

Removal effects SHALL use a synchronous, single-bound local projector while holding the source desired
state gate. The former product lifecycle RPC SHALL refuse mutation requests. Every remote mutation
SHALL be preceded by a durable exact binding/entity/attempt fence; current journal and disabled desired
state SHALL be checked before that fence authorizes an effect.

#### Scenario: A queued old request outlives its caller

- **GIVEN** an old product lifecycle request is delayed until after its caller loses the reply
- **WHEN** a newer source generation is admitted and the delayed request is released
- **THEN** the old RPC refuses the request and performs zero graph mutations.

#### Scenario: A mutation outcome is unknown

- **GIVEN** an exact effect fence was persisted before a possible graph mutation
- **WHEN** the outcome is unknown, internally failed, malformed, or interrupted by a crash
- **THEN** the durable fence remains and generation-changing source configuration writes are refused
- **AND** restart, readback and elapsed time do not imply resolution.

#### Scenario: A terminal outcome resolves only its own attempt

- **GIVEN** the public mutation result proves commit or an explicit before-effect rejection
- **WHEN** the coordinator records terminal resolution for the exact current attempt by CAS
- **THEN** generation changes may proceed only from authoritative journal terminal evidence
- **AND** the exact attempt and resolution are retained if the caller misses the write acknowledgement
- **AND** stale attempts, conflicting CAS, or absent terminal evidence leave admission blocked.
