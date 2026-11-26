## MODIFIED Requirements

### Requirement: The ingest transport stream refuses rather than evicts

The JetStream stream carrying SemSource's `graph.ingest.*` publications SHALL declare a finite age
bound, a finite byte bound, an explicit discard policy that refuses writes at the ceiling rather
than evicting undelivered messages, and a fixed retention policy that removes a message once
graph-ingest has acknowledged it and retains one published before graph-ingest's consumer exists.
The stream SHALL use file storage unless the operator selects the `ephemeral` posture, which uses
memory storage. Only the posture and the byte bound SHALL be configurable; the subjects, age bound,
replicas, discard policy and retention policy SHALL NOT be.

#### Scenario: Transport pressure is visible to the producer

- **GIVEN** the ingest transport stream holds unconsumed messages up to its byte bound
- **WHEN** a source publishes an entity
- **THEN** the publish is retried within the publisher's bounded delivery budget and then fails rather than succeeding
- **AND** the failure is counted and surfaced on the owning source's status, not discarded

#### Scenario: Bounds and policy are declared, not inherited

- **WHEN** the ingest transport stream configuration is validated
- **THEN** it declares a positive age bound, a positive byte bound, a discard policy, and a retention policy
- **AND** validation fails if any of the bounds or the discard policy is absent, rather than falling back to a server
  default

#### Scenario: An applied entity frees its space

- **GIVEN** graph-ingest has applied and acknowledged every published entity
- **WHEN** the stream's state is read
- **THEN** it holds no `graph.ingest.entity` message

#### Scenario: A restart within the age bound republishes without refusal

- **GIVEN** a completed seed whose stream bytes exceed half the byte bound
- **WHEN** SemSource alone restarts within the age bound and republishes the corpus
- **THEN** no publish is refused and status reaches `ready` with no dropped or failed entities

#### Scenario: A publish before the consumer binds is not lost

- **GIVEN** a freshly created ingest transport stream with no consumer bound
- **WHEN** a source publishes an entity and graph-ingest binds afterwards
- **THEN** graph-ingest applies that entity

#### Scenario: The durable posture survives a NATS restart

- **GIVEN** a deployment with the default posture and messages published but not yet applied
- **WHEN** NATS restarts before graph-ingest applies them
- **THEN** the messages are still in the stream and graph-ingest applies them after it reconnects

#### Scenario: The ephemeral posture uses memory storage

- **GIVEN** a configuration whose `graph_stream.posture` is `ephemeral`
- **WHEN** the stream configuration is resolved
- **THEN** the stream uses memory storage and every other fixed property is unchanged

#### Scenario: An operator sizes the byte bound

- **GIVEN** a configuration that sets `graph_stream.max_bytes`
- **WHEN** the stream configuration is resolved
- **THEN** the configured bound replaces the default and nothing else changes

#### Scenario: The raw stream overrides are gone

- **GIVEN** a configuration with a top-level `streams` key
- **WHEN** the configuration loads
- **THEN** loading fails and the error names `streams`

#### Scenario: An unknown posture is refused

- **GIVEN** a configuration whose `graph_stream.posture` is neither `durable` nor `ephemeral`
- **WHEN** the configuration loads
- **THEN** loading fails and the error names the posture
