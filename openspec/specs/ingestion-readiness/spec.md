# ingestion-readiness Specification

## Purpose
The aggregate ingestion phase, per-source phases, readiness sub-signals, and
entity counts tell the truth on every surface (NATS, HTTP, MCP): `ready` means
every configured source finished its initial seed; index/embedding readiness is
present on HTTP and MCP alike (one shared composer) with explicit
{available:false, reason} on failure; counts are distinct-entity cardinality,
invariant under periodic republication (audit 2026-07-19).
## Requirements
### Requirement: Ready means seeded

The aggregate ingestion `phase` SHALL be `ready` only when every configured source has completed
its initial seed AND no source lost an entity it offered during that seed; while any source is
still seeding the phase SHALL be observably `seeding`; any errored source — and any source whose
seed lost entities it had offered — SHALL yield `degraded`.

The documented consumer gate (poll until `ready`) therefore guarantees that no source silently
dropped part of the corpus it ingested. It does not guarantee that every entity has finished
landing at the instant `ready` is first observed: publishing is asynchronous and continues after a
source finishes walking its inputs. The guarantee is against silent loss, which is permanent, not
against in-flight delivery, which resolves on its own.

Completing a seed is necessary but not sufficient for `ready`. A `degraded` phase arising from
delivery loss SHALL be sticky: it SHALL NOT return to `ready` on the strength of continued activity
alone, but only once a subsequent pass completes with no loss.

#### Scenario: Mid-seed window is observable

- **WHEN** at least one configured source has reported but not yet completed its initial seed
- **THEN** `phase` is `seeding` on every status surface

#### Scenario: Ready after the last source completes

- **GIVEN** no configured source has lost an offered entity
- **WHEN** the final configured source reports initial-seed completion
- **THEN** `phase` transitions to `ready`, without waiting for entities still in flight

#### Scenario: Errored source degrades the aggregate

- **WHEN** any source reports phase `errored`
- **THEN** the aggregate phase is `degraded`, not `ready`

#### Scenario: A seed that completed with loss does not report ready

- **GIVEN** a source whose initial seed completed after entities it offered failed to be delivered
- **WHEN** the status surface is polled
- **THEN** the aggregate phase is `degraded`, not `ready`, and that source reports a non-zero loss
  figure

#### Scenario: Loss-degraded phase is sticky

- **GIVEN** the aggregate is `degraded` because a seed completed with loss
- **WHEN** sources continue watching and report no further errors
- **THEN** the phase remains `degraded` until a subsequent pass completes with no loss

### Requirement: Readiness sub-signals are present and honest on every surface

The structural-index and embedding readiness objects SHALL be present on the MCP `source_status`
tool AND the HTTP `/source-manifest/status` endpoint, composed by one shared assembly. When a
readiness sub-query fails, the corresponding object SHALL state `available: false` with a reason —
the key SHALL NOT be silently omitted.

#### Scenario: HTTP parity with the MCP tool

- **WHEN** a consumer fetches HTTP `/source-manifest/status`
- **THEN** the response carries the same `index` and `embedding` readiness objects (shape and
  values) as the MCP `source_status` tool at that instant

#### Scenario: Responder failure is explicit

- **WHEN** the index or embedding status responder is unavailable
- **THEN** the corresponding object reports `available: false` and a reason, and the remainder of
  the status payload is returned normally

### Requirement: Entity counts are distinct-entity truth

`entity_count`, `type_counts`, and `total_entities` SHALL report the cardinality of distinct
confirmed entities per source, invariant under republication of unchanged entities (periodic
reindex, restarts). Throughput counters, if exposed, SHALL be separately named.

#### Scenario: Periodic reindex does not inflate counts

- **WHEN** the periodic reindex republishes unchanged folder/repo/file entities
- **THEN** `entity_count` and `total_entities` are unchanged

### Requirement: Starting a source SHALL NOT prevent the service surfaces from becoming reachable

Bringing a source up SHALL NOT block the service's HTTP status surface or its
metrics endpoint from binding. A source's start SHALL complete promptly, doing
only fast deterministic setup, and SHALL perform its initial seed — and any work
sequenced after it, such as watcher startup — without holding the runtime's
startup path.

This is the availability precondition for every other guarantee in this
capability: a readiness signal that cannot be reached reports nothing, and a
consumer polling it cannot distinguish "still seeding" from "process is gone".

#### Scenario: Status is reachable while a long seed is still running

- **GIVEN** a source seeding a corpus large enough to take many minutes
- **WHEN** the status surface is polled shortly after process start
- **THEN** it answers, reporting the source in its seeding phase — rather than
  refusing the connection

#### Scenario: Metrics are reachable while a long seed is still running

- **WHEN** the metrics endpoint is scraped during a long initial seed
- **THEN** it answers and serves the publish counters for that source

#### Scenario: Unreachable source paths do not black out the surfaces

- **GIVEN** a source whose configured paths do not exist yet, so it is retrying
- **THEN** the status and metrics surfaces are still reachable throughout the
  retry window, and the source reports the condition rather than the process
  appearing dead

#### Scenario: Readiness gating is unchanged

- **WHEN** a consumer gates on the aggregate phase being `ready`
- **THEN** the gate behaves exactly as before — `ready` still means every
  configured source finished its initial seed, and a source that has started but
  not finished seeding does NOT report ready

### Requirement: A failed initial seed is reported, not swallowed

Because the initial seed no longer completes within the source's start, a seed
failure SHALL be surfaced through the source's own error reporting — its error
count and last-error detail, and a log at `Warn` or above — rather than being
silently lost when the start path returns successfully.

Failures that are immediate and deterministic — invalid configuration, or an
inability to construct the publisher — SHALL continue to fail the start itself,
so genuine misconfiguration still fails fast rather than becoming a runtime
surprise.

#### Scenario: Seed failure is visible on the status surface

- **GIVEN** a source whose initial seed fails after start has returned
- **WHEN** the status surface is polled
- **THEN** the source's error count is greater than zero, `last_error` describes
  the failure, and the source does not report itself as ready

#### Scenario: Invalid configuration still fails the start

- **GIVEN** a source configured with values that fail validation
- **THEN** the failure occurs during start, not asynchronously

### Requirement: Shutdown during a seed is safe

Stopping a source while its initial seed is still running SHALL cancel that seed
and wait for it to finish before tearing down the delivery path, so that no seed
work publishes into a stopped publisher and shutdown does not race the seed.

#### Scenario: Stop during an in-flight seed

- **GIVEN** a source whose initial seed is in progress
- **WHEN** the component is stopped
- **THEN** the stop returns only after the seed has stopped, and no entity is
  published after the publisher has been shut down

### Requirement: A seeding source reports progress, not just its endpoints

A source performing its initial seed SHALL report progress on an interval for the
duration of that seed, so that a stalled seed is distinguishable from a slow one.
Reporting SHALL NOT be limited to the transitions into and out of seeding.

Each progress report SHALL carry the source's cumulative confirmed-delivery count
so far, so a consumer or operator can observe whether it is advancing. "Confirmed
delivery" carries the meaning fixed by *Source status reflects delivery truth*:
acceptance for publication is not confirmation. Progress reporting SHALL be
interval-based and independent of entity volume, so a large corpus does not turn
observability into its own load.

Progress reporting is not itself a gate. It is visible *before* readiness and does not alter it;
readiness semantics are defined solely by *Ready means seeded*.

#### Scenario: A long seed shows movement

- **GIVEN** a source seeding a corpus large enough to take many minutes
- **WHEN** the status surface is polled repeatedly during the seed
- **THEN** the source's phase remains the seeding phase and its reported count
  increases between polls

#### Scenario: A stalled seed is distinguishable from a slow one

- **GIVEN** a source that is seeding but making no forward progress
- **WHEN** the status surface is polled repeatedly
- **THEN** the reported count does not advance between polls, while a slow-but-
  progressing source's count does

#### Scenario: Readiness gating is unchanged

- **WHEN** a consumer gates on the aggregate phase being `ready`
- **THEN** progress reporting during seeding does not cause the gate to report ready early, and
  does not alter the gate's conditions, which are governed entirely by *Ready means seeded*

### Requirement: Pre-publish seed work is visibly alive

A source whose seed performs substantial work before publishing — parsing a
watch path, resolving references, offloading verbatim bodies — SHALL advance at
least one externally visible progress counter during that work, so a plateau in
the delivery count is distinguishable from a hang. The counters SHALL be
visible on the same status surface as the delivery count and mirrored on the
metrics endpoint, per source instance.

This closes the residual gap recorded as `async-source-seed` task 5.7: the
delivery count alone proved a seed was not failing (no retries, no drops, no
errors) but could not separate "parsing, not yet publishing" from "hung".

#### Scenario: A parse or offload window reads as work, not a hang

- **GIVEN** a seed in a window where files are being parsed or bodies offloaded
  but no entities are being published
- **WHEN** the status surface is polled repeatedly
- **THEN** a pre-publish counter (files parsed, bodies offloaded) increases
  between polls while the delivery count stays flat

#### Scenario: A genuinely wedged seed shows no movement anywhere

- **GIVEN** a seed making no forward progress of any kind
- **WHEN** the status surface is polled repeatedly
- **THEN** neither the delivery count nor any pre-publish counter advances
  between polls

### Requirement: Publisher distress is visible per source

A source whose entity publisher is in sustained backpressure — retrying
against a refusing or saturated transport without yet dropping or failing —
SHALL report that condition as a per-source boolean on every status surface
(HTTP status, MCP `source_status`). A publisher in this state reports no
drops, no failures, and no errors while being functionally stalled; the flag
is what separates "slow" from "stalled" without reading logs.

#### Scenario: A retrying-but-stalled publisher is visible

- **GIVEN** a source whose publisher is in sustained backpressure
- **WHEN** the status surface is polled
- **THEN** that source's entry reports `backpressure: true` while its error
  count remains unchanged

#### Scenario: Recovery clears the flag

- **WHEN** the transport recovers and the publisher drains
- **THEN** subsequent status reports show `backpressure: false` (or omit the
  field) without operator intervention

### Requirement: The internal status report is a single shared contract

Every source component SHALL construct the one shared status-report type when
reporting to the aggregator, and the aggregator SHALL decode reports into
that same type strictly: a report carrying fields unknown to the shared type
is a defect and SHALL be rejected loudly (error log), never leniently decoded
with fields silently dropped. Re-declaring the report's wire shape (inline
structs, mirrored types) SHALL NOT appear in source components.

#### Scenario: A producer-side field addition reaches the surfaces

- **WHEN** a field is added to the shared report type and populated by a
  producer
- **THEN** the aggregator decodes it without consumer-side re-declaration,
  and dropping it silently is impossible by construction

#### Scenario: A bypassing producer is loud, not lossy

- **GIVEN** a report published with fields the shared type does not declare
- **WHEN** the aggregator decodes it
- **THEN** the report is rejected with an error log naming the decode
  failure, rather than accepted with the unknown fields discarded

### Requirement: Offering, delivery, and loss are separately named on every status surface

Per-source status SHALL carry the number of entities offered for publication, the number confirmed
delivered, and the number lost as three separately named figures, on every status surface that
carries per-source counts.

The offered figure SHALL count every entity the source handed to its publisher, including those the
publisher refused rather than buffered. A refused entity is counted as lost, so excluding it from
the offered figure would place it on one side of the arithmetic only.

No status surface SHALL publish a single figure that conflates offering with delivery. Such a figure
cannot answer the only question a consumer asks of it — whether the corpus actually arrived — and
reads as throughput while overstating delivery.

Each figure SHALL be independently meaningful: the delivered figure SHALL never exceed the number
of entities confirmed onto the graph stream, and the loss figure SHALL count every entity that was
offered and did not arrive.

No arithmetic relationship between the figures is required to hold at any instant. Status is a
sample of a live asynchronous system, and entities legitimately sit between offered and delivered
while publishing continues. Requiring the figures to reconcile exactly would demand a moving system
hold still, and would make a routine in-flight window indistinguishable from loss.

#### Scenario: A lossy seed reports all three figures

- **GIVEN** a seed in which some offered entities were never delivered
- **WHEN** the status surface is polled after that seed completes
- **THEN** the source reports a non-zero loss figure and a delivered figure equal to the number
  confirmed onto the stream

#### Scenario: A clean seed reports zero loss

- **GIVEN** a seed in which every offered entity was delivered
- **WHEN** the status surface is polled after that seed completes
- **THEN** the loss figure is zero

#### Scenario: No conflated figure is published

- **WHEN** any status surface carrying per-source counts is polled
- **THEN** no field in the response reports offering and delivery as a single number

