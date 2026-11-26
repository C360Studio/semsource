## ADDED Requirements

### Requirement: Config-file dependency and image identity excludes the version
A config-file dependency's entity ID SHALL be derived from its coordinate and declaring file only, and an image's from its name without tag or digest and its declaring file. A version or tag MUST be a fact on the entity, never part of its ID. One manifest declaring one coordinate at several versions, or one image at several tags, SHALL produce one entity carrying every declared value.

#### Scenario: A version bump updates the dependency in place
- **GIVEN** `go.mod` requires `github.com/a/b v1.2.0` and has been ingested
- **WHEN** it is changed to require `github.com/a/b v1.3.0` and re-ingested
- **THEN** exactly one `config.dependency` entity exists for `github.com/a/b` in that manifest
- **AND** it carries version `v1.3.0` and not `v1.2.0`

#### Scenario: Image tags do not split an image
- **GIVEN** a Dockerfile with `FROM golang:1.22 AS build` and `FROM golang:1.23`
- **WHEN** it is ingested
- **THEN** one `config.image` entity exists for `golang` in that file, carrying both references

#### Scenario: A registry port is not a tag
- **WHEN** a Dockerfile uses `FROM registry:5000/team/app:1.0`
- **THEN** the image name used for identity is `registry:5000/team/app`

#### Scenario: A Gradle classifier is its own artifact
- **GIVEN** a `build.gradle` declaring `org.lwjgl:lwjgl:3.3.1` and `org.lwjgl:lwjgl:3.3.1:natives-linux`
- **WHEN** it is ingested
- **THEN** two `config.dependency` entities exist for `org.lwjgl:lwjgl`, each with version `3.3.1`
- **AND** only one carries classifier `natives-linux`
