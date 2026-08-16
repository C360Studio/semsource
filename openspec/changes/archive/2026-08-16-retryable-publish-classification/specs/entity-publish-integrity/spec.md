# entity-publish-integrity delta — transient trouble retries, terminal trouble fails loudly

## ADDED Requirements

### Requirement: Publish errors are classified by kind, and only terminal kinds fail an entity

The publisher SHALL classify every publish error as retryable or terminal by
error class, never by string comparison. Retryable classes are the transient
transport conditions: circuit-open, not-connected, timeout/deadline expiry,
no-responders, and stream-capacity refusal (the ingest stream's documented
refuse-rather-than-evict posture). Terminal classes are conditions retrying
cannot cure (marshal failure, invalid subject) and shutdown cancellation.

A retryable error SHALL be retried within a bounded per-entity delivery
budget. An entity SHALL be marked failed only on budget exhaustion or a
terminal class. Failure messages SHALL state what actually happened: an
entity failed on first attempt is never described as having failed "after
retries", and budget exhaustion states the attempt count.

#### Scenario: A transient broker outage loses nothing within the budget

- **GIVEN** a seed publishing entities and a broker unreachable for a period
  shorter than the delivery budget
- **WHEN** the broker returns
- **THEN** every entity published during the outage window is delivered,
  the failed count is zero, and the retries counter reflects the outage

#### Scenario: A full ingest stream is retried, not abandoned

- **GIVEN** the ingest stream at its capacity ceiling refusing writes
- **WHEN** capacity frees within the delivery budget
- **THEN** refused entities deliver on retry rather than having been
  terminally failed on first refusal

#### Scenario: A dead transport still fails loudly, after the budget

- **GIVEN** a transport that never recovers
- **WHEN** an entity's delivery budget is exhausted
- **THEN** the entity is marked failed, and the failure names the attempt
  count and final error class

### Requirement: Retried publishes are idempotent

Because the ingest consumer applies entity merges by APPENDING triples, and a
timed-out publish may have succeeded server-side, every publish that can be
retried SHALL carry a deterministic per-payload message ID (derived from the
entity identity and content) so the stream's duplicate-detection window
collapses ambiguous retries to a single stored message. The retry budget's
total wall-clock SHALL fit within the stream's duplicate window, so a retry
can never land outside the window that makes it safe.

#### Scenario: An ambiguous timeout does not double-apply

- **GIVEN** a publish that times out after the server stored the message
- **WHEN** the publisher retries it
- **THEN** the stream stores one copy and downstream merge applies once

### Requirement: Failure logging aggregates; floods never spray

Terminal publish failures SHALL be reported edge-triggered: one entry at the
default level on entering a failing state (naming the first entity and the
error class) and one on recovery, with counts carried by the failed counter
and the seed-end summary. A flood of N failing entities SHALL NOT produce N
log lines.

#### Scenario: A failure flood is one entry, not thousands

- **GIVEN** thousands of entities failing terminally in one window
- **WHEN** the window elapses
- **THEN** the default-level log carries one entering-failure entry and one
  recovery entry, and the exact count is available on the metrics and status
  surfaces
