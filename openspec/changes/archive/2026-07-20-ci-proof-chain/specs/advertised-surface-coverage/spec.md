# Delta: advertised-surface-coverage

## ADDED Requirements

### Requirement: The integration proof chain runs in CI

CI SHALL execute the integration-tagged test suites covering MCP answers, the fusion pipeline,
versionDiff, and readiness gating on every PR and main push (with a NATS service available),
within a documented runtime budget.

#### Scenario: Integration regression fails PR CI

- **WHEN** an integration-tagged critical-path test breaks
- **THEN** PR CI fails

### Requirement: MCP tools have answer-content smoke coverage

The compose smoke SHALL perform at least one real `tools/call` per MCP tool and assert answer
content (known symbol → verbatim-body fragment; nonexistent symbol → honest miss; status →
readiness shape) — name-listing alone SHALL NOT count as MCP coverage.

#### Scenario: Wrong answer fails smoke

- **WHEN** code_context stops returning the known fixture symbol's body
- **THEN** the compose smoke fails

### Requirement: The default component set is pinned

A unit-level gate SHALL assert the complete required component spawn map (every component the
product surface depends on: gateways, context servers, manifest, supersession, …), so a dropped
registration fails CI (the bug class that shipped in beta.1).

#### Scenario: Dropped component fails unit CI

- **WHEN** any required component is removed from the default spawn map
- **THEN** a unit test fails

### Requirement: GRAPH stream subjects are pinned

A test SHALL pin the GRAPH stream's explicit subject list against overlap with request/reply
subjects (the documented PubAck silent-empty-results footgun).

#### Scenario: Overlapping subject fails CI

- **WHEN** a stream subject filter is broadened over a request/reply subject
- **THEN** the pinning test fails
