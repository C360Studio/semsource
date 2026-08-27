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

Configuration correctness also covers the **model registry**: `config.validateModelRegistry`
rejects a capability that resolves to an endpoint which cannot serve it — an LLM capability landing
on an embeddings endpoint — before any component starts, and requires a capability a selected tier
depends on to be declared explicitly rather than reached through a catch-all default. Leaving an
LLM capability unbound is a supported state that the consuming components degrade for (keyword-only
classification, template synthesis), not an omission; the shipped configs therefore set no
`defaults.model`, and every one of them is checked by discovery so a config added later is covered
without being registered by hand.

Configuration additionally reaches settings SemSource passes *through* to composed substrate
components, where the shape of the value matters as much as its correctness: `graph-clustering`'s
`entity_id_edges` block is tri-state, so an unset toggle is omitted from the composed config rather
than sent as `false`, and "not configured" never becomes "disabled" behind the operator's back.

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

### Requirement: A model capability resolves to an endpoint that can serve it

Configuration load SHALL reject a model registry in which any capability resolves to an endpoint that cannot serve that capability's protocol — in particular an LLM capability resolving to an embeddings endpoint. The error MUST name the capability, the endpoint it resolved to, and both remedies: bind the capability to an endpoint that can serve it, or leave it unbound so the consuming component degrades.

A misroute MUST be rejected rather than silently treated as unbound. Quietly degrading would correct the runtime behavior while leaving the configuration asserting something untrue, which is the same defect one layer up.

This extends the existing rule that a capability a selected tier needs must be explicitly declared. The complement is that a capability which resolves *at all* must resolve to something real: both express that a capability's binding is never allowed to be fictional.

#### Scenario: An LLM capability resolves to the embeddings endpoint

- **WHEN** a configuration would resolve `query_classification` or `answer_synthesis` to an
  embeddings endpoint, whether bound explicitly or reached through a catch-all default
- **THEN** `semsource validate` and `semsource run` both fail before any component starts
- **AND** the error names the capability, the endpoint, and both remedies

#### Scenario: An unbound LLM capability is accepted

- **WHEN** a configuration declares no binding for an LLM capability and no catch-all routes it
  anywhere
- **THEN** configuration load succeeds
- **AND** the consuming component uses its documented non-LLM path rather than calling an
  endpoint that cannot serve it

#### Scenario: An embedding capability bound to an embeddings endpoint is accepted

- **WHEN** `embedding` is bound to an endpoint that serves embeddings
- **THEN** configuration load succeeds, because the capability and the endpoint agree

### Requirement: Every shipped configuration is checked for capability-role agreement

The repository SHALL verify capability-role agreement across **every** configuration it ships, discovered rather than enumerated, so a configuration added later is covered without anyone remembering to register it.

Enumerating configurations by hand would keep passing while a new configuration ships unchecked — which is how a misroute reached every shipped configuration in the first place.

#### Scenario: A new shipped configuration is added

- **WHEN** a configuration file is added under the shipped configuration directories
- **THEN** it is checked for capability-role agreement without any change to the test
- **AND** a misroute in it fails the check

#### Scenario: Existing shipped configurations agree

- **WHEN** the shipped configurations are checked
- **THEN** every capability in each resolves either to an endpoint that can serve it or to
  nothing at all

### Requirement: Clustering edge synthesis is configurable

SemSource SHALL expose `graph-clustering`'s virtual-edge synthesis settings through
`semsource.json` rather than hardcoding them, so an operator can match edge synthesis to the shape
of their graph without editing Go.

The configuration SHALL be tri-state per boolean: unset means "use the substrate default", and only
an explicit value overrides it. An omitted block SHALL be passed through as omitted, so SemSource
never silently converts "not configured" into "disabled". Numeric settings SHALL follow the same
rule — a zero value means unset, not zero.

The settings SHALL apply only where clustering runs; they SHALL have no effect on tiers that do not
enable it.

#### Scenario: An operator overrides one setting

- **GIVEN** a config that sets only the system-peer toggle
- **WHEN** SemSource composes the clustering component
- **THEN** that toggle is passed through and every other synthesis setting is left to the substrate
  default

#### Scenario: An omitted block preserves substrate behavior

- **GIVEN** a config with no edge-synthesis block
- **WHEN** SemSource composes the clustering component
- **THEN** no synthesis keys are sent, and behavior matches the substrate's own defaults

#### Scenario: The settings are inert without clustering

- **GIVEN** a tier that does not enable clustering
- **WHEN** the config carries edge-synthesis settings
- **THEN** validation accepts them and no clustering component is composed

### Requirement: A substrate default SemSource overrides is justified by measurement

Where SemSource ships a default that differs from the substrate's, the divergence SHALL be recorded
with the measurement that motivated it — the corpus, the observable, and the before/after — so a
future reader can tell a deliberate override from an accident, and can re-measure when the corpus
shape changes.

A default SHALL NOT be changed on reasoning alone. The graph shape that motivates an override
SHALL be stated, because a default that suits one shape may be wrong for another.

#### Scenario: A shipped override names its evidence

- **WHEN** a shipped configuration overrides a substrate clustering default
- **THEN** the documentation states the corpus it was measured on, the effect measured, and the
  graph shape the override suits

#### Scenario: An unmeasured graph shape is not silently assumed

- **GIVEN** a deployment shape whose effect on clustering has not been measured
- **WHEN** a default is chosen
- **THEN** the unmeasured shape is called out rather than assumed to behave like the measured one

### Requirement: Object-store credentials are resolved from the environment, never persisted into configuration

Object-store access credentials SHALL be read from the process environment at component construction
and MUST NOT be accepted in, written to, or echoed from the runtime configuration document, the
configuration KV, validation output, status surfaces, or logs. A source entry references the store by
endpoint, bucket, and prefix only.

This follows the posture the MCP gateway already takes for its API token, and matters more here:
runtime configuration is watched and replicated through KV, so a credential placed in it would be
distributed well beyond the process that needs it.

#### Scenario: Credentials supplied by environment

- **WHEN** an object-store source is configured and its credentials are present in the environment
- **THEN** the source authenticates against the store
- **AND** the configuration document contains no credential field

#### Scenario: A credential is placed in the configuration document

- **WHEN** a configuration document carries an access key or secret on a source entry
- **THEN** strict decoding rejects it with the ordinary unknown-field classification

#### Scenario: Credentials never appear in output

- **WHEN** `semsource validate` runs, or a status surface renders an object-store source
- **THEN** no credential value appears in the output
- **AND** the source is identified by endpoint, bucket, and prefix

### Requirement: An object-store source entry is validated at load

A configuration entry for an object-store source SHALL be rejected at load — in `semsource validate`
and at `semsource run` startup — when it omits a bucket, or when it declares a non-default endpoint
that is not a parseable URL. The error MUST name the field and the offending value.

#### Scenario: A source entry without a bucket

- **WHEN** an object-store source entry omits its bucket
- **THEN** `semsource validate` and `semsource run` both fail with an error naming the field
- **AND** no component starts

#### Scenario: A malformed endpoint

- **WHEN** an object-store source entry carries an endpoint that is not a parseable URL
- **THEN** loading fails with an error naming the field and the value

#### Scenario: A valid entry with no endpoint

- **WHEN** an object-store source entry declares a bucket and no endpoint
- **THEN** loading succeeds and the store's default endpoint applies

### Requirement: Every source type configuration accepts is spawnable

A source type accepted by configuration validation SHALL be constructible into running components.
Configuration validation MUST NOT accept a source type the component-spawning path cannot build, so
that a configuration passing `semsource validate` cannot then fail at `semsource run` for the sole
reason that its source type is unsupported.

The set of accepted types MUST be enumerable by the packages that need to check this, so the
agreement is verifiable rather than a convention maintained by hand.

#### Scenario: Every accepted type builds

- **WHEN** the full set of source types configuration accepts is enumerated
- **THEN** each one produces component specifications
- **AND** none returns an unsupported-type error

#### Scenario: Validate-pass implies a startable configuration

- **WHEN** a configuration declaring any accepted source type passes `semsource validate`
- **THEN** starting the service does not fail on the grounds that the type is unsupported
