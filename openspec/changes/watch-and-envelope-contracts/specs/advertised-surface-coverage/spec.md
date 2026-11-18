## ADDED Requirements

### Requirement: Published envelopes decode through the bootstrap registry

Every payload type SemSource publishes as a typed message envelope MUST be registered in the
payload registry `buildPayloadRegistry()` returns, so the envelope decodes to its concrete type.

#### Scenario: A broadcast payload's registration is removed

- **WHEN** the registration for the manifest, predicate-schema or status payload is removed
- **THEN** a test decoding that broadcast through the bootstrap registry fails
