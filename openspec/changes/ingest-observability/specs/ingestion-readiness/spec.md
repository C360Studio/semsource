# ingestion-readiness delta — progress during seeding

## ADDED Requirements

### Requirement: A seeding source reports progress, not just its endpoints

A source performing its initial seed SHALL report progress on an interval for the
duration of that seed, so that a stalled seed is distinguishable from a slow one.
Reporting SHALL NOT be limited to the transitions into and out of seeding.

Each progress report SHALL carry the source's cumulative confirmed-delivery count
so far, so a consumer or operator can observe whether it is advancing. Progress
reporting SHALL be interval-based and independent of entity volume, so a large
corpus does not turn observability into its own load.

This does not change readiness semantics: `ready` continues to mean every
configured source finished its initial seed. Progress is visible *before* that
point; it is not a new gate.

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
- **THEN** the gate behaves exactly as before, and progress reporting during
  seeding does not cause it to report ready early
