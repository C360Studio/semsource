## MODIFIED Requirements

### Requirement: Shipped images are pinned and identifiable

Compose service images SHALL be pinned immutably (digest or exact version, no bare `latest` and no
floating major or minor tag), and the built semsource image SHALL report its version and commit via
`semsource version`. The NATS server image SHALL additionally name the same server line SemStreams
tests its own substrate against, so SemSource never runs its governed graph on a server version the
framework has not exercised.

#### Scenario: Version identifies the build

- **WHEN** `semsource version` runs in the compose-built container
- **THEN** it reports the release version/commit, not `dev`

#### Scenario: NATS server line matches the framework

- **WHEN** the Compose NATS service image is reviewed against the pinned SemStreams target
- **THEN** it names the same server line SemStreams uses in its own e2e, tiered, and integration
  harnesses
- **AND** the tag is an exact version, so a server upgrade is an explicit, reviewable edit rather
  than an implicit pull
