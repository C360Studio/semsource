## ADDED Requirements

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
changed `semsource.json`.

#### Scenario: A source added with the CLI takes effect on a retained store

- **GIVEN** a deployment that has booted once from a file with one source
- **WHEN** `semsource add` appends a second source and the application restarts on the same store
- **THEN** the manifest lists both sources

#### Scenario: A source removed with the CLI advances the revision

- **GIVEN** a `semsource.json` at revision `2` with two sources
- **WHEN** `semsource remove` removes one
- **THEN** the written file has revision `3` and one source

### Requirement: A retained configuration that differs from the file is reported

When the retained store's configuration is in effect after boot arbitration and its services or
components differ from those the file generates, SemSource SHALL log a warning that names the differing
entries and states that the file's values were not applied. It SHALL NOT warn when they are equal.

#### Scenario: An edit without a revision bump is reported

- **GIVEN** a deployment that has booted once
- **WHEN** the operator changes a source in the file without changing `config_revision` and restarts
- **THEN** the retained configuration stays in effect
- **AND** a warning names the differing component entry

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
