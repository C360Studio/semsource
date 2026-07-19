# Delta: advertised-surface-coverage

## ADDED Requirements

### Requirement: The removal round-trip has automated coverage

An automated gate SHALL exercise a real add → status → remove → status round-trip through MCP
`tools/call`, asserting the source appears, then disappears, and that removing an unknown handle
returns the NOT_FOUND error (the audit found no automated gate executes any MCP `tools/call`).

#### Scenario: Round-trip gate fails on phantom sources

- **WHEN** removal stops deregistering sources from status
- **THEN** the automated round-trip gate fails
