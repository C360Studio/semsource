# mcp-gateway-contract Specification

## Purpose
MCP tool results are honest: downstream ADR-060 handler errors surface as
`isError` results (RequestClassified, never the plain-Request footgun), argument
validation stays strict, fusion-backed successes always carry `contract_version`,
and tool descriptions state exactly what each readiness signal guarantees.

## Requirements

### Requirement: Downstream errors are tool errors

The MCP gateway SHALL map downstream handler failures (ADR-060 error envelopes and X-Status
header replies) to MCP error results (`isError: true`) carrying the envelope's message. A
successful fusion-backed tool result SHALL always carry `contract_version`; a response lacking it
SHALL never be returned as success.

#### Scenario: ADR-060 envelope surfaces as isError

- **WHEN** a code-context/doc-context handler replies with an ADR-060 error envelope
- **THEN** the MCP tool result has `isError: true` and its text contains the envelope message,
  not a fusion-shaped body

#### Scenario: Success responses are attributable

- **WHEN** a fusion-backed tool call succeeds
- **THEN** the returned payload carries `contract_version`

### Requirement: Signal guarantees are stated truthfully

Tool descriptions and the readiness note SHALL scope their guarantees precisely: the
"miss means genuine absence" claim SHALL be conditioned on `phase == ready` (all sources seeded)
AND `index.ready`, matching the honest gate delivered by this change.

#### Scenario: Readiness note matches behavior

- **WHEN** an agent reads the `source_status` note during the seed window
- **THEN** the note does not claim misses are genuine absences for that window
