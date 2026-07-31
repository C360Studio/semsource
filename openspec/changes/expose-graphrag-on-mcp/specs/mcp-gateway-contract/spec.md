## MODIFIED Requirements

### Requirement: Downstream errors are tool errors

The MCP gateway SHALL map downstream handler failures (ADR-060 error envelopes and X-Status
header replies) to MCP error results (`isError: true`) carrying the envelope's message. A
successful fusion-backed tool result SHALL always carry `contract_version`; a response lacking it
SHALL never be returned as success.

The gateway serves two tool families — fusion-backed deterministic tools and capability-gated
GraphRAG tools — and this mapping applies to both. For a tool whose downstream capability may be
absent from the running stack, a transport-level failure that means "this capability is not
present" (no responder, or a timeout with no handler subscribed) SHALL be reported as an error
naming the missing capability and the configuration that provides it — not as a bare
request-failed message, and never as a successful empty result. A capability-gated tool result
SHALL NOT be returned as success unless it carries the substrate's own response payload.

#### Scenario: ADR-060 envelope surfaces as isError

- **WHEN** a code-context/doc-context handler replies with an ADR-060 error envelope
- **THEN** the MCP tool result has `isError: true` and its text contains the envelope message,
  not a fusion-shaped body

#### Scenario: Success responses are attributable

- **WHEN** a fusion-backed tool call succeeds
- **THEN** the returned payload carries `contract_version`

#### Scenario: An absent capability is named

- **WHEN** a capability-gated tool's downstream subject has no responder in the running stack
- **THEN** the tool result is an error naming the missing capability and how to enable it, not a
  generic request-failed message and not an empty success

### Requirement: Signal guarantees are stated truthfully

Tool descriptions and the readiness note SHALL scope their guarantees precisely: the
"miss means genuine absence" claim SHALL be conditioned on `phase == ready` (all sources seeded)
AND `index.ready`, matching the honest gate delivered by this change.

A capability-gated tool's description SHALL additionally name the capability the tool requires and
the signal that reports its availability, so an agent reading the roster can tell that a refused or
empty GraphRAG answer may mean the running configuration does not provide GraphRAG rather than that
the graph is silent. No tool description SHALL claim a guarantee the running configuration cannot
deliver.

#### Scenario: Readiness note matches behavior

- **WHEN** an agent reads the `source_status` note during the seed window
- **THEN** the note does not claim misses are genuine absences for that window

#### Scenario: Capability-gated descriptions state their gate

- **WHEN** an agent lists the gateway's tools
- **THEN** each GraphRAG tool's description names the capability it requires and where that
  capability's availability is reported
