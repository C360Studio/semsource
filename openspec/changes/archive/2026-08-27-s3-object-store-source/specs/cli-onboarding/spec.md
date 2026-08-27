# cli-onboarding Specification

## ADDED Requirements

### Requirement: Object-store sources register non-interactively with explicit identity

`semsource add` SHALL register an object-store source non-interactively from a bucket, an optional
key prefix, and an optional endpoint, and SHALL accept the same `--project` and `--version` identity
flags the code and repo registrations accept. Omitting `--project` SHALL fall back to a
bucket-derived slug.

Explicit identity matters more for this source type than any other: a bucket carries no repository,
module path, or remote URL to derive a project from, so the automatic fallback is the weakest one
SemSource offers. The flags are the difference between a corpus that is identifiable and one that is
merely addressable.

#### Scenario: Registering a bucket prefix with explicit identity

- **WHEN** `semsource add s3` runs with a bucket, a prefix, an endpoint, and `--project`
- **THEN** the config gains one object-store entry carrying the bucket, prefix, endpoint, and project
- **AND** no credential value is written to the config

#### Scenario: Omitting explicit identity

- **WHEN** an object-store source is registered without `--project`
- **THEN** the entry is written without a project key
- **AND** identity derives from the bucket slug

#### Scenario: Two prefixes of one bucket as distinct projects

- **WHEN** two object-store entries are registered against the same bucket with different prefixes
  and different `--project` values
- **THEN** the config gains two entries whose ingested entities carry distinct system segments

#### Scenario: Registration failure is actionable

- **WHEN** registration is attempted with a bucket that cannot be reached or authenticated
- **THEN** the command fails with a message naming the endpoint, the bucket, and the cause
- **AND** the config file is left unchanged
