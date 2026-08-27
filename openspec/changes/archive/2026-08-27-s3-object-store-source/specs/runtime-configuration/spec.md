# runtime-configuration Specification

## ADDED Requirements

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
