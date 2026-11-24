## ADDED Requirements

### Requirement: Config-file entities are keyed by their declaring file
The config-file source SHALL include the declaring manifest's path, relative to the source root, in the instance of every entity it mints, and MUST NOT let two manifests under one source root write one entity ID. No instance SHALL depend on the checkout's location or folder name. Roots that share a system and same-coordinate declarations within one manifest are tracked by https://github.com/C360Studio/semsource/issues/375.

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

#### Scenario: A project override hides the checkout folder name
- **WHEN** a source with project `osh-core` is ingested from folders named `osh-core` and `osh-core-2`, each with a root-level `build.gradle`
- **THEN** every config-file entity ID is the same in both
