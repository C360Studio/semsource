## ADDED Requirements

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
