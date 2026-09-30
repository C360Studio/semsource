## ADDED Requirements

### Requirement: Fixture artifact identity is trusted and immutable

The MinIO fixture SHALL use the architect-approved exact upstream release through its verified mirror,
pinned by immutable manifest digest. Upstream mirror provenance, manifest/child integrity proof,
resolved image identity, and tested architecture SHALL be recorded. A mutable tag or an existing local
cache SHALL NOT be the sole proof of fixture identity or availability.

#### Scenario: A fixture is prepared on a clean worker

- **GIVEN** a worker without the fixture image cached
- **WHEN** the recorded preparation command runs for a supported architecture
- **THEN** it obtains the selected immutable artifact from the verified mirror
- **AND** records the resolved identity and architecture before running product assertions

#### Scenario: The selected artifact cannot be verified or acquired

- **WHEN** acquisition, integrity verification, or supported-architecture preparation fails
- **THEN** the selected suite reports a setup failure with diagnostics
- **AND** does not silently switch artifacts, skip selected cases, or report success

### Requirement: Fixture startup and cleanup have bounded ownership

Each fixture run SHALL own its resources and use finite acquisition/startup/readiness/cleanup bounds.
Readiness SHALL be established through a health or API signal rather than an arbitrary sleep. Failure
paths SHALL preserve diagnostics, clean up owned resources, and surface cleanup failures.

#### Scenario: Readiness does not arrive

- **GIVEN** an owned fixture container that fails its readiness contract
- **WHEN** the startup bound expires
- **THEN** setup fails, diagnostics identify the owned resource, and bounded cleanup is attempted
- **AND** no unrelated container, network, volume, or developer service is removed

#### Scenario: Startup returns an owned handle and an error

- **WHEN** container startup fails after returning a non-nil owned container handle
- **THEN** bounded cleanup still attempts to terminate that container
- **AND** setup remains failed with startup and cleanup diagnostics preserved

#### Scenario: Cleanup fails after a test

- **WHEN** fixture cleanup cannot complete within its bound
- **THEN** the failure is visible in the test or job outcome and retained evidence
- **AND** an otherwise-green suite fails while an already-failing suite retains a nonzero outcome

### Requirement: Fixture repair retains the original behavioral coverage

The repair SHALL retain all selected S3, Garage, and governance cases and their product assertions.
MinIO and Garage SHALL remain separate compatibility backends. A fixture failure SHALL NOT be masked
by dropping cases, skipping unavailable setup, or changing production storage/graph behavior.

#### Scenario: Combined Garage job executes

- **WHEN** the compatibility job selects `integration,garage` for `storage/s3store`
- **THEN** both the original MinIO and Garage cases execute against their real respective backends
- **AND** all existing assertions and cleanup obligations remain active

#### Scenario: Full governance integration is requested

- **WHEN** the full original integration suite is selected
- **THEN** every existing MinIO-dependent governance case executes
- **AND** prior temporary fixture exclusions do not count as satisfying the gate

### Requirement: Mainline and migration evidence remain independently attributable

The fixture fix SHALL first be validated on its independent mainline base without changing the
SemStreams or embedding-provider pins. A later migration overlay SHALL preserve that migration's
separate pins and record its own commits, artifact identities, architecture, commands, complete case
inventory, and results. Neither evidence set SHALL replace or relabel the other's failures.

#### Scenario: Mainline fixture repair completes

- **GIVEN** the independent beta.161 mainline base
- **WHEN** the fixture repair is validated
- **THEN** full integration and combined Garage results are tied to that exact base and fixture identity
- **AND** independent Go review covers the implementation and recorded evidence

#### Scenario: The fixture patch is overlaid on the migration

- **GIVEN** a separately reviewed migration branch with different pinned upstream contracts
- **WHEN** the fixture patch is applied for validation
- **THEN** full original integration and combined Garage cases run with the migration pins unchanged
- **AND** overlay evidence is labelled separately without claiming unrelated migration blockers resolved
