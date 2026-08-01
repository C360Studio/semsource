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

The gateway serves two tool families — fusion-backed deterministic tools and substrate graph-query
tools — and this mapping applies to both. A graph-query tool result SHALL NOT be returned as success
unless it carries the substrate's own response payload; the absence of `contract_version` on a
graph-query result SHALL NOT be treated as a failure, because that field belongs to the fusion
contract and the substrate does not emit it.

#### Scenario: ADR-060 envelope surfaces as isError

- **WHEN** a code-context/doc-context handler replies with an ADR-060 error envelope
- **THEN** the MCP tool result has `isError: true` and its text contains the envelope message,
  not a fusion-shaped body

#### Scenario: Success responses are attributable

- **WHEN** a fusion-backed tool call succeeds
- **THEN** the returned payload carries `contract_version`

#### Scenario: A graph-query success carries the substrate payload

- **WHEN** a graph-query tool call succeeds
- **THEN** the result carries the substrate's response payload, and its lack of `contract_version`
  is not treated as a failed response

### Requirement: Signal guarantees are stated truthfully

Tool descriptions and the readiness note SHALL scope their guarantees precisely: the
"miss means genuine absence" claim SHALL be conditioned on `phase == ready` (all sources seeded)
AND `index.ready`, matching the honest gate delivered by this change.

A tool that can answer through more than one retrieval path SHALL state in its description that its
result discloses which path answered, so a thinner answer is not read as a richer one. A tool
description SHALL NOT claim community or LLM-backed reasoning as unconditional when the running
configuration may not provide it, and SHALL NOT claim a guarantee the running configuration cannot
deliver.

#### Scenario: Readiness note matches behavior

- **WHEN** an agent reads the `source_status` note during the seed window
- **THEN** the note does not claim misses are genuine absences for that window

#### Scenario: Multi-path tools advertise their disclosure

- **WHEN** an agent lists the gateway's tools
- **THEN** a tool that can answer through more than one retrieval path states that its result
  discloses which path answered

