# Delta: advertised-surface-coverage

## ADDED Requirements

### Requirement: The readiness gate and error mapping have behavior tests

Automated tests SHALL pin the seed-window phase semantics (a status assertion during a mid-seed
state) and the ADR-060 → `isError` mapping (a forced downstream error observed through a real MCP
`tools/call`), so the two honesty guarantees of this change cannot regress silently.

#### Scenario: Seed-window test exists and fails on regression

- **WHEN** the aggregate phase computation is altered to report ready mid-seed
- **THEN** an automated test in the standard gates fails

#### Scenario: Error-mapping test exists and fails on regression

- **WHEN** the gateway returns a downstream error envelope as a successful tool result
- **THEN** an automated test in the standard gates fails
