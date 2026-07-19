# Delta: advertised-surface-coverage

## ADDED Requirements

### Requirement: Tier and durability paths have smoke coverage

The compose smoke SHALL boot the tier0 documented path and assert healthcheck truthfulness and
NATS state survival across container recreation, so the packaging guarantees of this change cannot
regress silently.

#### Scenario: Smoke fails on tier0 regression

- **WHEN** the tier0 config path breaks under compose
- **THEN** the automated compose smoke fails
