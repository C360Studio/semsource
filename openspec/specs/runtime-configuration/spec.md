# runtime-configuration Specification

## Purpose
TBD - created by archiving change remove-legacy-ingest-adapters. Update Purpose after archive.
## Requirements
### Requirement: SemSource has one runtime configuration

SemSource MUST run as an external service without a runtime mode selector. Configuration, environment
processing, schema, examples, logs, and current guidance MUST NOT expose `mode`, `ModeStandalone`,
`SEMSOURCE_MODE`, or another compatibility selector.

#### Scenario: Canonical configuration is loaded

- **WHEN** an operator loads valid SemSource configuration without a mode field
- **THEN** the service initializes its one external-service runtime
- **AND** no mode default or branch is evaluated

#### Scenario: Removed mode field is supplied

- **WHEN** strict top-level JSON decoding encounters `mode`
- **THEN** loading fails with the ordinary unknown-field classification
- **AND** no legacy translation or special removed-mode handler runs

#### Scenario: Removed environment variable is present

- **WHEN** the environment contains `SEMSOURCE_MODE`
- **THEN** SemSource does not read it or change behavior

### Requirement: ID-shaped configuration is validated at load

The system SHALL validate every configuration value that becomes an entity-ID segment
(namespace/org, explicit source identity overrides) at config load against the substrate's segment contract, in
`semsource validate` AND at `semsource run` startup, with errors naming the field, value, and
allowed alphabet. Values are rejected, never silently rewritten.

#### Scenario: Dotted org fails validate and run

- **WHEN** `semsource.json` carries `"namespace": "acme.io"`
- **THEN** `semsource validate` and `semsource run` both fail with an error naming `namespace`,
  the value, and the allowed alphabet, before any component starts

#### Scenario: Validate-pass implies publishable identity

- **WHEN** `semsource validate` succeeds for a configuration
- **THEN** no entity produced from that configuration is later rejected purely for ID-segment
  shape at the publish gate
