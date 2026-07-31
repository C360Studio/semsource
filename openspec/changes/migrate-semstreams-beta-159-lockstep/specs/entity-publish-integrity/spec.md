## ADDED Requirements

### Requirement: The ingest transport stream refuses rather than evicts

The JetStream stream carrying SemSource's `graph.ingest.*` publications SHALL declare a finite age
bound, a finite byte bound, and an explicit discard policy. That policy SHALL refuse writes at the
ceiling rather than evict undelivered messages, so that entity loss under transport pressure is
surfaced to the producer instead of occurring silently between a successful publish and graph
ingestion.

An operator MAY override the policy through configuration, and the override SHALL take effect —
choosing eviction is a deployment's decision to make, but it SHALL NOT be the unstated default.

#### Scenario: Transport pressure is visible to the producer

- **GIVEN** the ingest transport stream has reached its configured byte or age bound
- **WHEN** a source publishes an entity
- **THEN** the publish fails with a terminal error rather than succeeding
- **AND** the failure is counted and surfaced on the owning source's status, not discarded

#### Scenario: Bounds and policy are declared, not inherited

- **WHEN** the ingest transport stream configuration is validated
- **THEN** it declares a positive age bound, a positive byte bound, and a discard policy
- **AND** validation fails if any of the three is absent, rather than falling back to a server default

#### Scenario: An operator overrides the discard policy

- **GIVEN** a deployment that configures a discard policy for the ingest transport stream
- **WHEN** the stream configuration is resolved
- **THEN** the configured policy replaces the default
