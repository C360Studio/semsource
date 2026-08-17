# ingestion-readiness Delta

## ADDED Requirements

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
