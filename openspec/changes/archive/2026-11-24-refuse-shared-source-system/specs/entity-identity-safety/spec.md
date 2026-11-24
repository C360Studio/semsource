## ADDED Requirements

### Requirement: Sources that would share an entity-ID system are refused
SemSource SHALL refuse a source set in which two ast-source roots, or two doc-source roots, would mint entity IDs under the same org and system, unless the collision includes a submodule pin, whose pinned project and version every other root in it then declares. The refusal MUST happen before any colliding component publishes: at `semsource validate` and `semsource run` over the file's sources, at boot over the full set to be spawned, at a runtime `add_source` request, and at ast-source and doc-source component validation. The error SHALL name each colliding source and the shared system. Entity identity SHALL NOT change: the root is not added to the ID.

#### Scenario: Two ast sources under one project are refused
- **GIVEN** a config with ast sources at `./a` and `./b`, both with `project: svc` and no version
- **WHEN** `semsource validate` runs
- **THEN** it fails with an error naming both sources and system `svc`
- **AND** `semsource run` refuses to start with the same error

#### Scenario: Different versions of one project are admitted
- **GIVEN** ast sources at `./v1` and `./v2`, both with `project: lib`, with versions `1.0` and `2.0`
- **WHEN** the config is validated
- **THEN** no collision is reported

#### Scenario: A doc source with several roots under one project is refused
- **GIVEN** a docs source with paths `./a` and `./b` and `project: handbook`
- **WHEN** the config is validated
- **THEN** it fails with an error naming both roots and system `handbook`

#### Scenario: A runtime add that collides is refused and not persisted
- **GIVEN** a running deployment with an ast source under project `svc`
- **WHEN** an `add_source` request adds another ast source at a different path with `project: svc`
- **THEN** the reply carries error code `validation_failed` naming the existing instance
- **AND** the desired manifest is unchanged

#### Scenario: Submodule pins of one SHA are admitted
- **GIVEN** two parent repos that each link the same submodule at the same gitlink SHA
- **WHEN** boot discovery expands both into submodule pins
- **THEN** no collision is reported and both pins start

#### Scenario: A standalone source at a pin's identity shares the pin's IDs
- **GIVEN** a parent repo whose submodule is pinned at gitlink SHA `b1256521ee39`
- **AND** a standalone ast source for the same code with the pin's project and `version: b1256521ee39`
- **WHEN** boot plans both
- **THEN** no collision is reported and both write the same entity IDs

#### Scenario: Two operator sources at one project and version still collide
- **GIVEN** ast sources at `./a` and `./b`, both with `project: lib` and `version: 1.0`, and no submodule pin
- **WHEN** the config is validated
- **THEN** it fails with an error naming both sources
