## ADDED Requirements

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
