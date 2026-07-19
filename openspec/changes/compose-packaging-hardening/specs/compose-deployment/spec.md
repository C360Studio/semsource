# Delta: compose-deployment

## ADDED Requirements

### Requirement: Every documented config path boots

Every configuration file shipped in `configs/` (including `configs/tiers/`) SHALL be bootable
under the shipped compose stack exactly as its documentation states (mount + `SEMSOURCE_CONFIG`
resolution), and the tier documentation SHALL match the actual mounts.

#### Scenario: tier0 boots as documented

- **WHEN** the stack is started with `SEMSOURCE_CONFIG=tier0-statistical.json` per the README
- **THEN** semsource boots healthy in BM25 mode (no crash loop)

### Requirement: Healthchecks prove liveness

The semsource compose healthcheck SHALL exercise a real serving surface (HTTP status endpoint),
so `healthy` implies the service answers requests — dependent services (Caddy) gate on truth.

#### Scenario: Wedged service goes unhealthy

- **WHEN** the semsource process stops serving HTTP while the binary still exists
- **THEN** the container transitions to unhealthy

### Requirement: Graph state survives container recreation

The NATS service SHALL persist JetStream state in a named volume and carry a restart policy, so
recreating containers does not silently wipe the graph.

#### Scenario: Recreate keeps entities

- **WHEN** the nats container is recreated after an ingest
- **THEN** previously ingested entities remain queryable

### Requirement: The default install indexes the whole workspace honestly

The default compose configuration SHALL index all languages the AST source supports for the
mounted workspace (or state its restriction prominently at startup), so polyglot repos do not get
a silently partial graph from the one-command install.

#### Scenario: Polyglot repo coverage

- **WHEN** `docker compose up` runs against a repo containing Go and TypeScript/Svelte code
- **THEN** symbols from all of those languages are queryable (or the startup log and docs state
  the configured restriction explicitly)

### Requirement: Shipped images are pinned and identifiable

Compose service images SHALL be pinned immutably (digest or exact version, no bare `latest`), and
the built semsource image SHALL report its version and commit via `semsource version`.

#### Scenario: Version identifies the build

- **WHEN** `semsource version` runs in the compose-built container
- **THEN** it reports the release version/commit, not `dev`
