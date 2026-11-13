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

### Requirement: Component configuration changes explicitly require restart

After successful boot, SemSource SHALL treat changes that add, remove, or reconfigure components as
next-boot desired state. A successful configuration write SHALL distinguish persistence from runtime
activation and report `restart_required: true`. SemSource SHALL NOT claim that a newly desired source
is ingesting or that a removed desired source has stopped in the sealed running composition.
Source-file watches owned by already-running components SHALL continue to process their supported
content edits independently of component composition changes.

#### Scenario: A source is added after boot

- **WHEN** a source-management request persists a new source component configuration
- **THEN** the result reports that desired configuration changed and restart is required
- **AND** success does not assert that the source is running or indexed

#### Scenario: A desired source is removed

- **WHEN** a source-management request removes a component from next-boot desired configuration
- **THEN** the result reports restart required
- **AND** the running component's observed status is not silently reclassified as stopped

#### Scenario: Removal overrides an original file-defined source

- **GIVEN** a source was declared by the original JSON and admitted at boot
- **WHEN** its desired removal succeeds and the application restarts with the same JSON and retained store
- **THEN** the source remains excluded from boot admission and default query scopes
- **AND** a later explicit re-add restores next-boot admission without changing deployment authority

#### Scenario: A watched file changes inside an existing source

- **GIVEN** a source component was admitted at boot and supports file watching
- **WHEN** an owned source file changes
- **THEN** the existing watcher processes that edit without requiring composition replacement

### Requirement: Owned entities use the effective deployment authority

SemSource SHALL accept `platform_id` as the declared deployment stem, defaulting to `semsource` when
omitted. It SHALL validate the declared authority through the substrate contract and use the
framework-established effective authority after configuration startup. Different environments sharing
a broker SHALL use distinct stems; an environment label alone is not an isolation boundary. Stems
separate configuration and identity only; independent graph deployments require broker/account isolation.

SemSource SHALL construct owned entity identities as `org.platform.system.domain.type.instance`, using
the effective platform authority established by the substrate configuration manager. Constructors
SHALL receive that authority explicitly. Product names SHALL remain provenance, never substitute for
platform authority. A source declaring another org SHALL fail before ingestion unless a separately
admitted import contract authorizes it. Taxonomy scope SHALL remain correct before result limiting.

#### Scenario: First boot mints an authority

- **WHEN** the framework establishes an effective authority from the declared platform stem
- **THEN** every owned entity and owned relationship subject uses that effective authority
- **AND** constructors do not predict the minted suffix or rewrite a previously created ID

#### Scenario: Restart uses retained storage

- **WHEN** the same deployment restarts against its retained configuration store
- **THEN** it adopts the recorded authority and recreates the same intended owned entity IDs

#### Scenario: A taxonomy query crosses source systems

- **WHEN** a code or document lens searches its taxonomy across configured source systems
- **THEN** its scope selects the correct fourth-segment taxonomy before applying result limits
- **AND** it does not broaden to all deployment entities and filter only after truncation

### Requirement: The composition version orders the binary and the operator

SemSource SHALL derive the version of its generated composition as `<generation>.<config_revision>.0`,
where the generation is a constant of the binary and `config_revision` is an optional non-negative
integer in `semsource.json` that defaults to `0`. A negative `config_revision` SHALL fail validation.

#### Scenario: A configuration without a revision keeps the current version

- **GIVEN** a `semsource.json` with no `config_revision`
- **WHEN** the composition is generated
- **THEN** its version is the generation followed by `.0.0`

#### Scenario: A bumped revision makes the file newest

- **GIVEN** a retained store written from a file at revision `0`
- **WHEN** the operator changes a source in the file, sets `config_revision` to `1`, and restarts
- **THEN** the changed source is in the running composition

#### Scenario: A negative revision is refused

- **WHEN** `semsource.json` sets `config_revision` to `-1`
- **THEN** loading the configuration fails and names the field

### Requirement: Source commands that write the configuration file advance its revision

`semsource add` and `semsource remove` SHALL increment `config_revision` by one whenever they write a
changed `semsource.json`. `semsource init` SHALL write a revision one higher than that of an existing
file it overwrites.

#### Scenario: A source added with the CLI takes effect on a retained store

- **GIVEN** a deployment that has booted once from a file with one source
- **WHEN** `semsource add` appends a second source and the application restarts on the same store
- **THEN** the manifest lists both sources

#### Scenario: A source removed with the CLI advances the revision

- **GIVEN** a `semsource.json` at revision `2` with two sources
- **WHEN** `semsource remove` removes one
- **THEN** the written file has revision `3` and one source

#### Scenario: Init over an existing file keeps the file newest

- **GIVEN** an existing `semsource.json` at revision `4`
- **WHEN** `semsource init` overwrites it
- **THEN** the written file has revision `5`

### Requirement: A newer file replaces the retained components

When the file's version is newer than the retained store's, SemSource SHALL delete from the retained
store, once the configuration manager has started, every component the file's composition lacks, so that
a component the file dropped does not return on a later boot. A failure of that deletion SHALL be logged
and SHALL NOT fail the boot; a component it leaves behind is reported as retained-only on later boots.

#### Scenario: A source removed with the CLI stays removed

- **GIVEN** a deployment that has booted once from a file with two sources
- **WHEN** `semsource remove` removes one and the application restarts twice on the same store
- **THEN** the removed source's component is absent on both boots
- **AND** no configuration-difference warning is logged on either

#### Scenario: A corrected source path does not bring the old source back

- **GIVEN** a deployment that has booted once with a source at one path
- **WHEN** the operator changes that source's path, bumps `config_revision`, and restarts twice
- **THEN** only the component for the new path is present on both boots

#### Scenario: The deletion is interrupted

- **GIVEN** a boot that pushed a newer file and stopped before deleting a component the file dropped
- **WHEN** the application boots again with the same file
- **THEN** the leftover component is in effect
- **AND** the configuration-difference warning names it as retained-only

### Requirement: A retained configuration that differs from the file is reported

When the retained store's configuration is in effect after boot arbitration and its services, components,
model registry, or NATS settings differ from those the file generates, SemSource SHALL log a warning
that names the differing entries, never their values, and states that the file's values were not applied.
Components that exist only in the retained configuration SHALL be named separately, and the warning SHALL
then say that making the file newest would drop them. Components the file enables and the retained
configuration holds disabled SHALL be named separately, and the warning SHALL then say that making the
file newest would re-enable them. The warning SHALL NOT state that a newer file removes a model registry
or service that only the retained configuration holds. It SHALL NOT warn when nothing differs.

#### Scenario: An edit without a revision bump is reported

- **GIVEN** a deployment that has booted once
- **WHEN** the operator changes a source in the file without changing `config_revision` and restarts
- **THEN** the retained configuration stays in effect
- **AND** a warning names the differing component entry

#### Scenario: A model registry edit without a revision bump is reported

- **GIVEN** a deployment that has booted once with a model registry
- **WHEN** the operator changes an endpoint in the file's model registry without changing `config_revision` and restarts
- **THEN** the retained endpoint stays in effect
- **AND** a warning names the model registry

#### Scenario: A component that exists only in the retained configuration is named separately

- **GIVEN** a retained configuration holding a source component that the file does not define
- **WHEN** the application boots at an equal version
- **THEN** the warning names that component as retained-only
- **AND** it does not advise bumping `config_revision` without saying the component would be dropped

#### Scenario: A source removed at runtime is named as retained-disabled

- **GIVEN** a file-defined source whose removal was accepted at runtime
- **WHEN** the application boots with the same file at an equal version
- **THEN** the warning names that component as retained-disabled
- **AND** it says a higher `config_revision` would re-enable it

#### Scenario: An unchanged file is silent

- **GIVEN** a deployment that has booted once
- **WHEN** it restarts with the same file
- **THEN** no configuration-difference warning is logged

### Requirement: A change to the generated composition requires a new generation

The composition SemSource generates from a fixed configuration SHALL match the fingerprint recorded for
the binary's generation, so that a change to the generated composition cannot ship without a generation
bump.

#### Scenario: The generator changes without a bump

- **WHEN** the composition generated from the fixed configuration no longer matches the recorded fingerprint
- **THEN** the fingerprint test fails and instructs the author to bump the generation and record the new fingerprint

