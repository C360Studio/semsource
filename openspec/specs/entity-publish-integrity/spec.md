# entity-publish-integrity Specification

## Purpose
The publish boundary (`internal/entitypub`) never loses entities silently: the
producer enforces the substrate ID contract (semstreams ValidateEntityID), the
buffered publisher applies bounded backpressure and drops LOUDLY (real counters,
WARN logs), and per-source status reflects delivery truth — parse failures,
rejections, drops, and terminal publish failures are all visible in error counts.
## Requirements
### Requirement: Publish gate enforces the downstream contract

The publish boundary (`internal/entitypub`) SHALL validate entity IDs with the substrate's own
validator (semstreams `ValidateEntityID`) before publishing, so an entity that graph-ingest would
reject is rejected at the producer with an error attributing the source and the offending segment.

#### Scenario: Invalid segment rejected at the producer

- **WHEN** a payload carries an entity ID with a segment violating the graph-ingest alphabet
- **THEN** publish fails with an error naming the entity ID and source instance, the entity is not
  sent, and the source's error count increments

### Requirement: The publisher never drops silently

The buffered entity publisher SHALL NOT discard entities silently. On buffer overflow it SHALL
apply bounded backpressure; if the bound is exceeded it SHALL drop loudly: increment a real drop
counter, log the entity ID and source at WARN, and surface the counter in source status.

#### Scenario: Sustained overflow is visible

- **WHEN** the publisher's buffer remains full beyond the bounded backpressure window
- **THEN** the drop counter surfaced in source status increments by exactly the number of dropped
  entities and a WARN log names each dropped entity ID

#### Scenario: Transient overflow loses nothing

- **WHEN** the buffer fills transiently and drains within the backpressure bound
- **THEN** every entity is delivered and the drop counter does not increment

### Requirement: Source status reflects delivery truth

Per-source status SHALL count an entity as ingested only after confirmed hand-off to delivery,
and SHALL surface parse failures and publish rejections in `error_count` (with `last_error`
detail), so a healthy-looking status implies entities actually reached the graph substrate.

#### Scenario: Parse failure is visible

- **WHEN** a source file fails to parse during seed or reindex
- **THEN** the source's `error_count` is greater than zero and `last_error` describes the failure

#### Scenario: Rejected entities are not counted as ingested

- **WHEN** entities are rejected by the publish gate or dropped by the publisher
- **THEN** they are excluded from the ingested/confirmed count and reflected in error/drop counts

### Requirement: The ingest transport stream refuses rather than evicts

The JetStream stream carrying SemSource's `graph.ingest.*` publications SHALL declare a finite age
bound, a finite byte bound, and an explicit discard policy. That policy SHALL refuse writes at the
ceiling rather than evict undelivered messages, so that entity loss under transport pressure is
surfaced to the producer instead of occurring silently between a successful publish and graph
ingestion.

An operator MAY override the policy through configuration, and the override SHALL take effect —
choosing eviction is a deployment's decision to make, but it SHALL NOT be the unstated default.

#### Scenario: Transport pressure is visible to the producer

- **GIVEN** the ingest transport stream has reached its configured byte or age bound
- **WHEN** a source publishes an entity
- **THEN** the publish fails with a terminal error rather than succeeding
- **AND** the failure is counted and surfaced on the owning source's status, not discarded

#### Scenario: Bounds and policy are declared, not inherited

- **WHEN** the ingest transport stream configuration is validated
- **THEN** it declares a positive age bound, a positive byte bound, and a discard policy
- **AND** validation fails if any of the three is absent, rather than falling back to a server default

#### Scenario: An operator overrides the discard policy

- **GIVEN** a deployment that configures a discard policy for the ingest transport stream
- **WHEN** the stream configuration is resolved
- **THEN** the configured policy replaces the default

### Requirement: Backpressure is visible even when nothing is lost

The publisher already drops loudly. Backpressure that does NOT drop — the bounded
retry path taken when the transport refuses a publish — SHALL also be observable,
because a publisher retrying every entity is functionally stalled while reporting
no drops, no failures, and no errors.

Sustained retrying SHALL raise a `Warn` naming the condition, on entering that
state rather than per attempt, and SHALL clear when publishing recovers. The
cumulative retry count SHALL be exported as a metric and surfaced in source
status alongside the existing published, failed, and dropped counts.

#### Scenario: A retrying publisher is not silent

- **GIVEN** a transport applying sustained backpressure so that publishes are
  retried rather than refused outright
- **WHEN** the instance runs at the default log level
- **THEN** a `Warn` names the backpressure condition, and the retry count is
  visible in metrics and source status

#### Scenario: Retrying does not log per attempt

- **WHEN** backpressure persists across many entities and many retry attempts
- **THEN** the condition is logged on entry and on recovery, not once per attempt

#### Scenario: Recovery is reported

- **WHEN** the transport accepts publishes again after a period of backpressure
- **THEN** the recovery is recorded and the condition is no longer reported as active
