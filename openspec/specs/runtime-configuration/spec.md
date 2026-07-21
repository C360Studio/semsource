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
