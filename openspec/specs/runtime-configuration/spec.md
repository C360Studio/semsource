# runtime-configuration Specification

## Purpose
SemSource runs as a single external service with no runtime-mode selector: `config.Config`
(`config/config.go`) has no `mode`, `ModeStandalone`, or `SEMSOURCE_MODE` field, and
`config.LoadConfig`/`LoadConfigFromReader` (`config/loader.go`) decode `semsource.json` with
`DisallowUnknownFields`, so a supplied `mode` key fails with the ordinary unknown-field error
instead of being translated by a removed compatibility path — and `semsource validate` and
`semsource run` both load configuration through this same function, so the same guardrails apply
at both surfaces. The package is also the one place a configuration value that becomes an
entity-ID segment is checked against the substrate's charset before any component starts:
`config.ValidateNamespace` validates the configured `Namespace` against `semstreams`'s entity-ID
alphabet and an org-length ceiling (`entityid.MaxOrgLen`), rejecting a bad value by field, value,
and allowed alphabet rather than rewriting it, so a namespace that passes `semsource validate` can
never later be rejected purely for ID shape at publish time.
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

