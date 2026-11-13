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

When the file's version is newer than the retained store's, SemSource SHALL leave no component in the
retained store that the file's composition lacks, so that a component the file dropped does not return on
a later boot.

#### Scenario: A source removed with the CLI stays removed

- **GIVEN** a deployment that has booted once from a file with two sources
- **WHEN** `semsource remove` removes one and the application restarts twice on the same store
- **THEN** the removed source's component is absent on both boots
- **AND** no configuration-difference warning is logged on either

#### Scenario: A corrected source path does not bring the old source back

- **GIVEN** a deployment that has booted once with a source at one path
- **WHEN** the operator changes that source's path, bumps `config_revision`, and restarts twice
- **THEN** only the component for the new path is present on both boots

### Requirement: A retained configuration that differs from the file is reported

When the retained store's configuration is in effect after boot arbitration and its services, components,
model registry, or NATS settings differ from those the file generates, SemSource SHALL log a warning
that names the differing entries, never their values, and states that the file's values were not applied.
Entries that exist only in the retained configuration SHALL be named separately, and the warning SHALL
then say that making the file newest would drop them. It SHALL NOT warn when nothing differs.

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
