## ADDED Requirements

### Requirement: NATS integration fixture is SemSource-owned and bounded

Integration tests SHALL obtain NATS through `internal/natstest`. It SHALL start one broker per test from an image
pinned by digest and return a client built through `natsclient.NewClient` and `Connect`. JetStream SHALL be enabled
only when the test requests it. Every stream the fixture creates SHALL carry finite age and size bounds. Each start
attempt (image acquisition, start, readiness, host and port lookup) SHALL be bounded. When the client port stays
unpublished for the whole observation window, the fixture SHALL replace the container at most once, and only if the
first container was removed cleanly and the test is still running. Cleanup SHALL close the client before
terminating the broker. A cleanup failure or a cancellation cause SHALL appear in the returned error.

#### Scenario: A test does not request JetStream

- **WHEN** a test starts a broker with no options
- **THEN** the broker reports JetStream not enabled

#### Scenario: A test requests streams

- **WHEN** a test starts a broker with streams
- **THEN** each stream exists with its subjects and with the fixture's age, size and discard bounds

#### Scenario: The client port appears late but within the window

- **WHEN** the client port is unpublished on the first observations and published before the window ends
- **THEN** the same container is used and no replacement is launched

#### Scenario: The client port never appears

- **WHEN** the client port stays unpublished for the whole window, including when the window ends during a lookup
- **THEN** the container is removed and replaced once
- **AND** if the replacement also fails, the error names both attempts

#### Scenario: The first container cannot be removed

- **WHEN** removing a failed container returns an error
- **THEN** no replacement is launched and the error carries both the startup and the removal causes

#### Scenario: The test ends during setup or cleanup

- **WHEN** the test's context is cancelled while the broker starts, while its port is observed, or while a failed
  container is being removed
- **THEN** no replacement is launched and the error carries the cancellation cause

#### Scenario: A non-absence failure occurs

- **WHEN** the container host cannot be resolved, or the port lookup fails for a reason other than absence
- **THEN** the container is removed and no replacement is launched
