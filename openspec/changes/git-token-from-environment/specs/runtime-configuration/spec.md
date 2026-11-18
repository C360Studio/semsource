## ADDED Requirements

### Requirement: The git token is resolved from the environment, never persisted into configuration

SemSource SHALL read the git access token only from the `SEMSOURCE_GIT_TOKEN` environment variable, at
the point of use, and MUST NOT accept it in `semsource.json` or write it to any component
configuration or the configuration KV bucket.

#### Scenario: Token supplied by environment

- **GIVEN** `SEMSOURCE_GIT_TOKEN` is set and a remote repository source is configured
- **WHEN** SemSource boots
- **THEN** the git source authenticates its clone with the token
- **AND** no generated component configuration contains the token value

#### Scenario: A token placed in the configuration file

- **WHEN** `semsource.json` carries a `git_token` field
- **THEN** loading fails with an error that names `SEMSOURCE_GIT_TOKEN`

#### Scenario: A retained component configuration loses the token

- **GIVEN** a retained configuration bucket whose git-source components carry a `git_token` value
- **WHEN** SemSource boots a build at the next composition generation
- **THEN** the latest revision of every retained git-source component key contains no token

#### Scenario: Rotating the token takes effect on restart

- **GIVEN** a deployment that has booted with one token
- **WHEN** the operator sets a different `SEMSOURCE_GIT_TOKEN` and restarts without changing `config_revision`
- **THEN** the git sources use the new token

### Requirement: Source commands write back only the file's own content

`semsource add` and `semsource remove` SHALL write `semsource.json` with only the keys the file
already held, plus the changed `sources` and `config_revision`. Defaults and environment values MUST
NOT be written.

#### Scenario: Environment and defaults stay out of the file

- **GIVEN** `SEMSOURCE_GIT_TOKEN` is set and `semsource.json` has no `workspace_dir`
- **WHEN** `semsource add` appends a source
- **THEN** the written file contains neither the token value nor a `workspace_dir` key

#### Scenario: A broken file is not rewritten

- **GIVEN** a `semsource.json` that fails validation
- **WHEN** `semsource remove` runs
- **THEN** it fails and the file is unchanged
