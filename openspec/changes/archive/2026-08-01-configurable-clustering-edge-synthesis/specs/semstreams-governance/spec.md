## ADDED Requirements

### Requirement: Composed substrate defaults are a deliberate choice

SemSource composes substrate components by supplying their configuration. For a composed component,
SemSource SHALL treat the substrate's defaults as a choice it is making, not as an absence of one:
where a default's suitability depends on the shape of SemSource's own data, that dependence SHALL be
examined rather than inherited silently.

This is the composition analogue of the existing rule that SemSource's KV port declarations must
conform to the framework catalog: the framework supplies the mechanism, and SemSource remains
answerable for the values it hands over.

#### Scenario: A default whose fitness depends on SemSource's ID shape

- **GIVEN** a substrate default whose behavior is driven by a segment of the entity ID that
  SemSource controls
- **WHEN** SemSource composes that component
- **THEN** the default's fitness for SemSource's ID shape is evaluated, and the outcome recorded —
  whether the default is kept or overridden

#### Scenario: Inherited defaults are visible

- **WHEN** SemSource composes a substrate component
- **THEN** the settings it does not supply are discoverable, so an operator can tell which behavior
  is SemSource's and which is the substrate's

### Requirement: Substrate behavior is fixed upstream, not worked around

Where a substrate default produces an unwanted result, SemSource SHALL correct it by supplying
different configuration through the supported seam. SemSource SHALL NOT reimplement the substrate
behavior locally, and where no configuration seam exists the gap SHALL be recorded in
`docs/upstream/semstreams-asks.md` and raised as a semstreams issue.

#### Scenario: A configurable default is corrected by configuration

- **WHEN** an unwanted clustering result traces to a substrate default that is configurable
- **THEN** SemSource supplies a different value and computes no clustering of its own

#### Scenario: A non-configurable defect becomes an upstream issue

- **WHEN** an unwanted result traces to substrate behavior with no configuration seam
- **THEN** it is filed upstream with evidence, and SemSource states the resulting limitation rather
  than substituting a local implementation
