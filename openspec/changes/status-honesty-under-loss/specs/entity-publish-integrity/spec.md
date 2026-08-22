## MODIFIED Requirements

### Requirement: Source status reflects delivery truth

Per-source status SHALL count an entity as ingested only after confirmed hand-off to delivery,
and SHALL surface parse failures and publish rejections in `error_count` (with `last_error`
detail), so a healthy-looking status implies entities actually reached the graph substrate.

"Confirmed hand-off to delivery" means the entity was confirmed onto the graph stream. Acceptance
into the publisher's send buffer SHALL NOT satisfy this requirement: acceptance succeeds before any
delivery outcome exists, so a count taken at that point reports intent rather than arrival.

An entity SHALL NOT be provisionally counted as ingested and later corrected. An entity that is
accepted and subsequently fails terminally after retries, or is dropped on buffer overflow, SHALL
be absent from the confirmed count for the whole of its lifetime, and SHALL be reflected in the
loss figures instead.

#### Scenario: Parse failure is visible

- **WHEN** a source file fails to parse during seed or reindex
- **THEN** the source's `error_count` is greater than zero and `last_error` describes the failure

#### Scenario: Rejected entities are not counted as ingested

- **WHEN** entities are rejected by the publish gate or dropped by the publisher
- **THEN** they are excluded from the ingested/confirmed count and reflected in error/drop counts

#### Scenario: Acceptance is not confirmation

- **GIVEN** an entity accepted into the publisher's send buffer whose delivery is not yet confirmed
- **WHEN** the status surface is polled
- **THEN** the entity is absent from the confirmed count, and present in the accepted count

#### Scenario: A terminal failure after acceptance is never counted

- **GIVEN** an entity that was accepted for publication and then failed terminally after retries
- **WHEN** the status surface is polled
- **THEN** the entity is absent from the confirmed count at every point in its lifetime, and is
  reflected in the loss figures
