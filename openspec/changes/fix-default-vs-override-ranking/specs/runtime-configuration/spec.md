## MODIFIED Requirements

### Requirement: ID-shaped configuration is validated at load

The system SHALL validate the configured namespace/org — the value that becomes the entity-ID org segment — at config load against the substrate's segment contract, in `semsource validate` AND at `semsource run` startup, with an error naming the field, value, and allowed alphabet. An invalid org is rejected, never silently rewritten, because org is the sovereignty boundary and is never normalized.

Other identity-shaped configuration is NORMALIZED rather than rejected. `SourceEntry.Project` and `WatchPathConfig.Project` are checked only for non-emptiness at load, then flow through `entityid.SystemSlug`, which maps characters outside the allowed alphabet to `-` and truncates past 80 characters with a content hash. That normalization is deliberate: `SystemSlug` exists to slugify arbitrary module paths and filesystem paths, and rejecting a value it would cleanly slugify would cost usability for no safety gain.

#### Scenario: Dotted org fails validate and run

- **WHEN** `semsource.json` carries `"namespace": "acme.io"`
- **THEN** `semsource validate` and `semsource run` both fail with an error naming `namespace`,
  the value, and the allowed alphabet, before any component starts

#### Scenario: A project value outside the alphabet is normalized, not rejected

- **WHEN** a source entry carries a `project` containing characters outside the ID alphabet
- **THEN** configuration load succeeds
- **AND** the value is slugified by `entityid.SystemSlug` when it becomes an ID segment

#### Scenario: Validate-pass implies publishable identity

- **WHEN** `semsource validate` succeeds for a configuration
- **THEN** no entity produced from that configuration is later rejected purely for ID-segment
  shape at the publish gate
