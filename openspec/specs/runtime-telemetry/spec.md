# runtime-telemetry Specification

## Purpose

What SemSource exposes for an operator to answer "is this healthy, and if not,
what is wrong" without attaching a debugger or raising the log level. Owns the
Prometheus metrics surface and the severity contract for logs: continuous state
(counters, depths, progress) belongs in metrics and status, while logs carry
state transitions and unrecoverable events. Exists because a 30-minute ingest
stall produced no metrics, no status, and no logs at the default level.

## Requirements

### Requirement: SemSource publishes its own metrics

SemSource SHALL register metrics on the Prometheus registry it already
constructs and passes to every component, and those metrics SHALL be served on
the configured metrics endpoint. At minimum the entity publish boundary SHALL
export, per source instance, the count of entities published, failed, dropped,
and retried.

Retry count SHALL be exported as its own series rather than folded into failures,
because a retrying publisher is healthy-but-degraded while a failing one is not,
and the two demand different operator responses.

#### Scenario: Metrics endpoint serves SemSource series

- **WHEN** an operator scrapes the configured metrics endpoint of a running instance
- **THEN** the response contains SemSource's own publish counters, not only the
  platform's default process and runtime metrics

#### Scenario: Retry pressure is distinguishable from failure

- **GIVEN** a publisher whose transport is applying backpressure but eventually
  accepting every entity
- **THEN** the retry series increases while the failed and dropped series stay flat

### Requirement: Log severity is chosen by consequence, not by volume

A log call's level SHALL be determined by what an operator must do about it, not
by how often it fires. Specifically:

- A condition that silently breaks a user-visible guarantee — readiness reporting,
  liveness, seed completion, or delivery — SHALL NOT be logged below `Warn`.
- A condition that is recoverable and self-correcting SHALL be logged on the
  transition into and out of it, NOT on every occurrence.
- Per-item failures SHALL NOT be logged individually at `Warn` when the item count
  scales with corpus size; they SHALL be counted, with detail available at `Debug`
  and an aggregate reported.

Volume SHALL be controlled by aggregating or by logging transitions, never by
lowering the level of a consequential event until it disappears.

#### Scenario: Degradation is visible at the default level

- **GIVEN** an instance running at the default log level
- **WHEN** publishing enters sustained backpressure, status reporting starts
  failing, or a source cannot reach its configured paths
- **THEN** each condition produces at least one `Warn` naming the condition, and
  the operator does not have to raise the log level to discover it

#### Scenario: A sustained condition does not flood

- **WHEN** a degraded condition persists across many events
- **THEN** the log records the transition into it once rather than one line per
  event, and records the return to healthy when it clears

#### Scenario: Per-item failures are counted, not enumerated at Warn

- **WHEN** a seed encounters many individually-failing files
- **THEN** the failures are reflected in a count and an aggregate report, and
  individual entries are available at `Debug` rather than one `Warn` each
