# advertised-surface-coverage Specification

## Purpose
Keep SemSource's user-facing README and integration guides honest by requiring
named test, smoke, or blocker evidence for every advertised command, route, task,
MCP tool, and direct consumer contract.
## Requirements
### Requirement: README advertised surfaces have explicit test evidence

SemSource SHALL maintain explicit test evidence for every executable command,
task, MCP tool, HTTP route, and GraphQL route advertised in `README.md`.
Low-level NATS subjects and predicate/schema routes SHALL be documented in the
consumer integration guide unless they are required for first-run usage. Evidence
MAY be a unit test, integration test, e2e test, smoke task, or a temporary
upstream blocker entry with an issue/tag reference.

#### Scenario: README adds an executable command

- **GIVEN** `README.md` advertises a shell command such as `semsource add` or
  `docker compose up`
- **WHEN** the README change is reviewed
- **THEN** the advertised-surface matrix names the test or smoke command that
  proves the command

#### Scenario: A surface is blocked upstream

- **GIVEN** an advertised surface cannot be reliably tested until an upstream
  SemStreams fix is released
- **WHEN** the surface remains in the README
- **THEN** the matrix names the upstream issue or fixed tag and the SemSource
  validation command that will close the blocker

#### Scenario: Low-level query contracts stay out of the README

- **GIVEN** a route or subject is only needed by direct integration consumers,
  such as `graph.query.*` or predicate/schema inspection
- **WHEN** the README is updated
- **THEN** the README links to the M5 consumer integration guide instead of
  enumerating the low-level contract inline

### Requirement: README CLI examples are command-tested

SemSource SHALL test the CLI commands shown in the README at the user-facing
command boundary. Config-mutating commands SHALL prove the resulting
`semsource.json` shape and user-visible output where applicable.

#### Scenario: Non-interactive add commands update config

- **GIVEN** a valid existing `semsource.json`
- **WHEN** the README examples for `semsource add ast`, `semsource add repo`,
  `semsource add docs`, and `semsource add url` are executed in tests
- **THEN** the config contains the expected source entries and option values

#### Scenario: Sources command lists configured entries

- **GIVEN** a config with multiple source entries
- **WHEN** `semsource sources` is executed in tests
- **THEN** the output includes the configured source types and locations

#### Scenario: Remove commands mutate config

- **GIVEN** a config with multiple source entries
- **WHEN** `semsource remove --index N` or interactive `semsource remove` is
  executed in tests
- **THEN** only the selected source is removed and the remaining config is valid

### Requirement: Native quick-start is black-box tested

SemSource SHALL provide a black-box test for the native README quick-start path:
`semsource init --quick`, `semsource validate`, and `semsource run`.

#### Scenario: Quick-start generated config can run

- **GIVEN** a temporary project fixture and a disposable NATS server
- **WHEN** a compiled SemSource binary runs `init --quick`, then `validate`, then
  `run` against the generated config
- **THEN** SemSource starts, publishes source entities, and reports a non-empty
  source manifest/status response

### Requirement: Default Docker Compose profile has a core smoke

SemSource SHALL provide a smoke test for the default Docker Compose profile
advertised by `docker compose up`. The smoke SHALL assert concrete state on the
published `:8080` SemSource HTTP/MCP surface and tear the stack down by default.

#### Scenario: Core profile serves status and MCP

- **GIVEN** SemSource targets a SemStreams release with restart-safe runtime
  components
- **WHEN** the core smoke starts the default compose profile
- **THEN** `/source-manifest/status`, `/source-manifest/sources`, and the MCP
  gateway are reachable on the configured SemSource HTTP port

### Requirement: Agent-facing MCP tools have happy-path coverage

SemSource SHALL test successful MCP calls for the agent tools advertised in the
README: `code_context`, `code_search`, `code_impact`, `doc_context`, and
`code_changes`.

#### Scenario: MCP query tools translate to graph requests

- **GIVEN** the MCP gateway is connected to NATS responders for the relevant graph
  or fusion subjects
- **WHEN** an MCP client calls each advertised query tool with valid input
- **THEN** the tool returns a non-error response with the expected envelope shape

### Requirement: Documented status and query routes have behavior tests

SemSource SHALL test the product-owned HTTP and NATS status/query routes listed in
the README or consumer integration guide, including `/source-manifest/predicates`,
`graph.query.predicates`, and `graph.query.versionDiff`.

#### Scenario: Source-manifest predicates are queryable

- **GIVEN** the source-manifest component is running
- **WHEN** a client requests `/source-manifest/predicates` or
  `graph.query.predicates`
- **THEN** the response includes predicate schema grouped by source type

#### Scenario: Version diff query is served

- **GIVEN** versioned source entities exist in the graph
- **WHEN** a client requests `graph.query.versionDiff` or
  `POST /supersession/versionDiff`
- **THEN** the response reports added, removed, and changed symbols using the
  advertised response shape

### Requirement: The readiness gate and error mapping have behavior tests

Automated tests SHALL pin the seed-window phase semantics (a status assertion during a mid-seed
state) and the ADR-060 → `isError` mapping (a forced downstream error observed through a real MCP
`tools/call`), so the two honesty guarantees of this change cannot regress silently.

#### Scenario: Seed-window test exists and fails on regression

- **WHEN** the aggregate phase computation is altered to report ready mid-seed
- **THEN** an automated test in the standard gates fails

#### Scenario: Error-mapping test exists and fails on regression

- **WHEN** the gateway returns a downstream error envelope as a successful tool result
- **THEN** an automated test in the standard gates fails
