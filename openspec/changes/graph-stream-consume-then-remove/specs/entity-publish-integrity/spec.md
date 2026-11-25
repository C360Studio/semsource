## MODIFIED Requirements

### Requirement: The ingest transport stream refuses rather than evicts

The JetStream stream carrying SemSource's `graph.ingest.*` publications SHALL declare a finite age
bound, a finite byte bound, an explicit discard policy that refuses writes at the ceiling rather
than evicting undelivered messages, and a fixed retention policy that removes a message once
graph-ingest has acknowledged it and retains one published before graph-ingest's consumer exists.
An operator MAY override the discard policy; the retention policy SHALL NOT be configurable.

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

#### Scenario: An operator overrides the discard policy

- **GIVEN** a deployment that configures a discard policy for the ingest transport stream
- **WHEN** the stream configuration is resolved
- **THEN** the configured policy replaces the default

#### Scenario: Retention cannot be overridden

- **GIVEN** a configuration whose `streams.GRAPH` names a retention policy
- **WHEN** the configuration loads
- **THEN** loading fails and the error names the field
