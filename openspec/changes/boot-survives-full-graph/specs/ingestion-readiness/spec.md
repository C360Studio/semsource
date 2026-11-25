## ADDED Requirements

### Requirement: A full ingest transport stream SHALL NOT stop SemSource from starting

SemSource SHALL start and serve its status, source and predicate surfaces when the ingest transport stream refuses
a start-time manifest or predicate-schema broadcast, so graph-ingest can drain the stream. Each refused broadcast
SHALL be retried in the background until it is delivered or SemSource stops. The refusal SHALL be logged once on
entry and once on delivery. The aggregate phase SHALL NOT depend on broadcast delivery.

#### Scenario: Boot into a full stream stays up and drains

- **GIVEN** the ingest transport stream holds unconsumed entity messages up to its byte bound
- **WHEN** SemSource starts
- **THEN** the process keeps running and the status surface answers
- **AND** graph-ingest drains the stream and the manifest broadcast is delivered once there is room

#### Scenario: Source and predicate queries answer before the broadcast lands

- **GIVEN** the manifest and predicate-schema broadcasts have been refused and not yet delivered
- **WHEN** a consumer queries the configured sources or the predicate schema over NATS or HTTP
- **THEN** it receives the current manifest and schema

#### Scenario: Readiness does not wait for the broadcast

- **GIVEN** every configured source has completed its initial seed with no loss
- **WHEN** the manifest broadcast is still undelivered
- **THEN** the aggregate phase is `ready`

#### Scenario: A refusal is reported once, not once per attempt

- **GIVEN** the stream refuses the manifest broadcast for several retry attempts
- **WHEN** it is finally delivered
- **THEN** the log carries one warning for the refusal and one line for the delivery

#### Scenario: Stopping ends the retry

- **GIVEN** a broadcast is still being retried
- **WHEN** SemSource stops
- **THEN** the retry ends and the stop completes within its bound
