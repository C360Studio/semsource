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
