# Delta: runtime-configuration

## ADDED Requirements

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
