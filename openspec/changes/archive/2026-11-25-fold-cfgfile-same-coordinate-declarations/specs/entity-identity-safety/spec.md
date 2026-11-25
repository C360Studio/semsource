## MODIFIED Requirements

### Requirement: Config-file entities are keyed by their declaring file
The config-file source SHALL include the declaring manifest's path, relative to the source root, in the instance of every entity it mints, and MUST NOT let two manifests under one source root write one entity ID. No instance SHALL depend on the checkout's location or folder name. Two roots that share a system are refused by the shared-system rule; declarations of one artifact within one manifest fold into one entity.

#### Scenario: One coordinate declared by two manifests gets two IDs
- **WHEN** `a/pom.xml` declares `org.slf4j:slf4j-api:2.0.9` with scope `test` and `b/build.gradle` declares it under `implementation`
- **THEN** the source publishes two `config.dependency` entities with distinct IDs
- **AND** one carries `dependency-scope` `test` and the other `dependency-configuration` `implementation`, and neither carries both

#### Scenario: Same-named Gradle directories do not collide
- **WHEN** a source root contains `a/core/build.gradle` and `b/core/build.gradle`
- **THEN** the source publishes two Gradle project entities with distinct IDs

#### Scenario: Two Dockerfiles sharing a base image keep their own ports
- **WHEN** two Dockerfiles both use `FROM golang:1.23` and expose different ports
- **THEN** the source publishes two `config.image` entities, each with its own file's ports and path

#### Scenario: IDs do not depend on the checkout location
- **WHEN** one tree is ingested from two different absolute checkout paths
- **THEN** every config-file entity ID is the same in both

#### Scenario: Two config roots under one project are refused
- **WHEN** one config source lists the paths `/r/a` and `/r/b` with project `shared`, each holding a root-level `package.json` with no name
- **THEN** `semsource validate` refuses the source set, naming both roots and the system `shared`

#### Scenario: A project override hides the checkout folder name
- **WHEN** a source with project `osh-core` is ingested from folders named `osh-core` and `osh-core-2`, each with a root-level `build.gradle`
- **THEN** every config-file entity ID is the same in both

## ADDED Requirements

### Requirement: Declarations of one artifact within a manifest fold into one entity
The config-file source SHALL mint one entity per artifact within a manifest, keeping each usage (scope, configuration, dev or indirect) as a fact on it, and MUST NOT emit two states under one ID or name one ID twice in a manifest's edges. A Maven type other than `jar`, a Maven classifier, and the exact image reference SHALL be part of the instance of every declaration that carries them.

#### Scenario: One Gradle coordinate under two configurations folds into one entity
- **WHEN** one `build.gradle` declares `org.slf4j:slf4j-api:2.0.9` under `implementation` and under `testImplementation`
- **THEN** the source publishes one `config.dependency` entity for it, carrying `dependency-configuration` `implementation` and `testImplementation`
- **AND** the Gradle project's `requires` edges name that entity once

#### Scenario: One Maven coordinate under two scopes folds into one entity
- **WHEN** one `pom.xml` declares `junit:junit:4.13.2` with scope `compile` and again with scope `test`
- **THEN** the source publishes one `config.dependency` entity for it, carrying `dependency-scope` `compile` and `test`

#### Scenario: A Maven test-jar is its own entity
- **WHEN** one `pom.xml` declares `org.acme:core:1.0` and `org.acme:core:1.0` with `<type>test-jar</type>`
- **THEN** the source publishes two `config.dependency` entities with distinct IDs
- **AND** the plain declaration has the ID it had before `<type>` was parsed

#### Scenario: An npm dependency in both sections is a prod dependency
- **WHEN** one `package.json` lists `lodash` `^4.17.21` in `dependencies` and in `devDependencies`
- **THEN** the source publishes one `config.dependency` entity for it with `dependency-kind` `npm-prod`

#### Scenario: Images that slugify alike stay distinct
- **WHEN** one Dockerfile has `FROM foo/bar:1` and `FROM foo-bar:1`
- **THEN** the source publishes two `config.image` entities with distinct IDs
