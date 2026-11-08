## MODIFIED Requirements

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

#### Scenario: Legacy status fields are not re-emitted

- **GIVEN** a graph index or embedding status reply that still carries the legacy `phase`,
  `revision`, or `last_synced` fields
- **WHEN** SemSource assembles the readiness object on any status surface
- **THEN** the object carries only `available`, `ready`, `state`, `indexed_revision`,
  `target_revision`, `lag`, and `reason`
- **AND** read-your-write freshness is compared against the numeric `indexed_revision` alone
