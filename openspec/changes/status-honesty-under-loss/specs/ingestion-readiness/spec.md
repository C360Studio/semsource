## MODIFIED Requirements

### Requirement: Ready means seeded

The aggregate ingestion `phase` SHALL be `ready` only when every configured source has completed
its initial seed AND every entity those sources accepted for publication was confirmed delivered;
while any source is still seeding the phase SHALL be observably `seeding`; any errored source — and
any source whose seed completed with entities accepted but never delivered — SHALL yield
`degraded`. The documented consumer gate (poll until `ready`) therefore guarantees the initial
corpus is fully published.

Completing a seed is necessary but not sufficient for `ready`. A `degraded` phase arising from
delivery loss SHALL be sticky: it SHALL NOT return to `ready` on the strength of continued activity
alone, but only once a subsequent pass completes with no loss.

#### Scenario: Mid-seed window is observable

- **WHEN** at least one configured source has reported but not yet completed its initial seed
- **THEN** `phase` is `seeding` on every status surface

#### Scenario: Ready after the last source completes

- **GIVEN** no configured source has lost an accepted entity
- **WHEN** the final configured source reports initial-seed completion
- **THEN** `phase` transitions to `ready`

#### Scenario: Errored source degrades the aggregate

- **WHEN** any source reports phase `errored`
- **THEN** the aggregate phase is `degraded`, not `ready`

#### Scenario: A seed that completed with loss does not report ready

- **GIVEN** a source whose initial seed completed after entities it accepted failed to be delivered
- **WHEN** the status surface is polled
- **THEN** the aggregate phase is `degraded`, not `ready`, and that source reports a non-zero loss
  figure

#### Scenario: Loss-degraded phase is sticky

- **GIVEN** the aggregate is `degraded` because a seed completed with loss
- **WHEN** sources continue watching and report no further errors
- **THEN** the phase remains `degraded` until a subsequent pass completes with no loss

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

## ADDED Requirements

### Requirement: Acceptance, delivery, and loss are separately named on every status surface

Per-source status SHALL carry the number of entities accepted for publication, the number confirmed
delivered, and the number lost as three separately named figures, on every status surface that
carries per-source counts.

No status surface SHALL publish a single figure that conflates acceptance with delivery. Such a
figure cannot answer the only question a consumer asks of it — whether the corpus actually arrived —
and reads as throughput while overstating delivery.

The three figures SHALL reconcile: entities accepted equals entities delivered, plus entities lost,
plus any still in flight. The delivered figure SHALL never exceed the number of entities confirmed
onto the graph stream.

#### Scenario: A lossy seed reports all three figures

- **GIVEN** a seed in which some accepted entities were never delivered
- **WHEN** the status surface is polled after that seed completes
- **THEN** the source reports a non-zero loss figure, a delivered figure equal to the number
  confirmed onto the stream, and an accepted figure equal to delivered plus lost

#### Scenario: A clean seed reports zero loss

- **GIVEN** a seed in which every accepted entity was delivered
- **WHEN** the status surface is polled after that seed completes
- **THEN** the loss figure is zero and the delivered figure equals the accepted figure

#### Scenario: No conflated figure is published

- **WHEN** any status surface carrying per-source counts is polled
- **THEN** no field in the response reports acceptance and delivery as a single number
