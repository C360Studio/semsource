## ADDED Requirements

### Requirement: Git watch changes carry typed entity state

When the git handler's watch observes a new HEAD, its change event MUST carry the commits' typed
entity state, identified under the configured deployment authority.

#### Scenario: A commit is made after startup

- **GIVEN** a watched git source with a configured authority
- **WHEN** a commit is added to the repository
- **THEN** the next change event carries commit entity states
- **AND** every commit state's ID is under the configured authority
