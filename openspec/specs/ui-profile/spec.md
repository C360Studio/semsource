# ui-profile Specification

## Purpose

Define SemSource's optional browser-profile packaging, same-origin proxy contract, and integration
evidence while preserving the UI-independent backend as the default deployment.
## Requirements
### Requirement: Same-origin SemSource routes for the UI profile

The UI profile SHALL expose a Caddy entry point on the configured C360 port that serves the SemSource
workbench and routes SemSource backend APIs on the same origin. The route map SHALL include `/health`,
`/graphql`, `/source-manifest/*`, `/code-context/*`, `/doc-context/*`, `/mcp-gateway/*`, `/metrics`, and
the raw `/graph` WebSocket. It SHALL NOT imply or proxy SemTeams-only routes.

The `/health` route SHALL proxy a SemSource-owned health envelope and return HTTP 200 JSON, including
at least `component: semsource`, `healthy`, `status`, `message`, `namespace`, `phase`, and
`total_entities`. It SHALL NOT be a static proxy-only response, fall through to the UI server, or
return non-JSON text.

#### Scenario: Health is machine-readable for the workbench

- **WHEN** a browser or Playwright test requests `GET /health` through the Caddy entry point
- **THEN** it receives HTTP 200 JSON from SemSource with `component: semsource`, `healthy: true`,
  source-manifest status fields, and a human-readable message

#### Scenario: Source status is proxied through Caddy

- **WHEN** a browser requests `GET /source-manifest/status` through the Caddy entry point
- **THEN** it receives the SemSource status payload with `namespace` and aggregate `phase`

#### Scenario: GraphQL is routed to the SemSource graph gateway

- **WHEN** a browser posts a GraphQL request to `/graphql` through the Caddy entry point
- **THEN** the request reaches the SemSource graph gateway and returns a GraphQL-shaped JSON response
  rather than UI HTML or a proxy 404

### Requirement: Light Playwright UI-profile smoke

SemSource SHALL provide a lightweight Playwright e2e smoke for the `ui` profile that uses the
SemSource-owned Playwright dependency, actively polls the Caddy entry point, and asserts the shell and
SemSource backend routes are reachable. The smoke SHALL have bounded deadlines and actionable failure
messages that include the last observed HTTP response for failed endpoint polls.

SemSource SHALL also provide a one-command smoke target that starts the `ui` profile, runs the
Playwright assertions, and tears the profile down by default. The released smoke path SHALL NOT require
a sibling checkout or local Node toolchain. An explicitly selected development source override MAY
preflight only the prerequisites for that override.

#### Scenario: UI profile smoke proves the operator path

- **GIVEN** the `ui` profile stack is running with its SemSource-owned artifact
- **WHEN** the Playwright smoke is executed against the configured Caddy URL
- **THEN** it verifies `/health`, `/source-manifest/status`, `/graphql`, and `/` through the same origin
  an operator uses

#### Scenario: Smoke failures include concrete state

- **WHEN** an endpoint never reaches the expected state before the deadline
- **THEN** the failing assertion reports the endpoint, final HTTP status, and final response body
  snippet

#### Scenario: One-command smoke owns profile lifecycle

- **WHEN** an operator runs the UI-profile smoke target
- **THEN** it starts `docker compose --profile ui`, runs the Playwright smoke against the Caddy origin,
  and tears the profile down when the smoke exits

#### Scenario: Released smoke needs no local UI source

- **GIVEN** no sibling UI checkout or local Node toolchain exists
- **WHEN** an operator runs the released UI-profile smoke target
- **THEN** it starts and validates the pinned artifact without local UI preflight failure

#### Scenario: Mutable convenience tag is rejected

- **GIVEN** the operator supplies `latest` with or without a digest
- **WHEN** released-image verification or profile preflight runs
- **THEN** it rejects the reference as release evidence

#### Scenario: Smoke owns its Playwright dependency

- **WHEN** the SemSource UI smoke resolves Playwright
- **THEN** it uses the dependency declared and locked under `ui/`
- **AND** it does not resolve a SemTeams or donor checkout

### Requirement: Optional SemSource UI profile

SemSource SHALL provide an opt-in Compose `ui` profile using a SemSource-owned workbench artifact built
from `ui/`. The default core and embedded modes SHALL remain fully functional without resolving,
pulling, building, validating, or starting any UI dependency.

The released profile SHALL use an immutable SemSource UI image and SHALL NOT require a sibling source
checkout or local JavaScript toolchain. An explicit development path MAY build `./ui`, but it SHALL NOT
be a production prerequisite.

#### Scenario: Default Compose path remains headless

- **WHEN** an operator renders or starts the default Compose configuration without optional profiles
- **THEN** no UI service, proxy, image, sibling checkout, or Node prerequisite is required

#### Scenario: Operator explicitly starts the SemSource workbench

- **WHEN** an operator enables the `ui` profile
- **THEN** the SemSource-owned workbench artifact and same-origin proxy start alongside the
  core stack

#### Scenario: Released workbench requires no sibling checkout

- **GIVEN** no C360 UI source repository or Node toolchain exists on the host
- **WHEN** the operator starts the released `ui` profile
- **THEN** it uses the immutable released SemSource artifact without attempting a local UI build

#### Scenario: Embedded consumer remains UI-independent

- **WHEN** another sem* product starts or connects to SemSource as a service
- **THEN** SemSource ingestion, readiness, HTTP, MCP, NATS, GraphQL, and graph behavior work without the
  UI artifact

#### Scenario: Former SemTeams behavior is absent

- **WHEN** an operator enables the repurposed `ui` profile
- **THEN** it serves the SemSource workbench and does not build, mount, or expose the former SemTeams UI

#### Scenario: Development build is explicit

- **GIVEN** a developer explicitly selects the documented local UI build path
- **WHEN** the `ui` profile is rendered
- **THEN** it builds `./ui` without changing the immutable released default

#### Scenario: Donor paths are absent

- **WHEN** the released and development UI profiles are rendered
- **THEN** neither configuration references `../semteams`, `../semstreams-ui`, or another donor UI

### Requirement: Workbench and headless release evidence

SemSource SHALL maintain independent release evidence for the headless core and the SemSource-owned
`ui` profile. The UI smoke SHALL actively verify the shell, SemSource health/source status, query
reachability, accessible result/detail navigation, and the advertised graph state against a real
SemSource backend. The headless smoke SHALL prove the same backend starts and serves its non-UI
contracts without any UI artifact.

Pull requests SHALL run locked UI quality/browser gates, clean production-image verification, and
release-verifier contract tests without publishing. Only trusted pushes to `main` or a valid
`v`-prefixed SemVer tag SHALL receive package-write permission and publish the SemSource-owned image
to `ghcr.io/c360studio/semsource-ui` for `linux/amd64` and `linux/arm64`.

A `main` publication SHALL emit `latest` and `sha-<full-revision>` tags, with the SHA tag as the
verification tag and OCI version. A release publication SHALL emit the exact `v<semver>` tag and the
plain `<semver>` tag, with the `v` tag as the verification tag and the plain SemVer as the OCI version.
Every platform image SHALL carry the derived `org.opencontainers.image.version` and the full Git
revision in `org.opencontainers.image.revision`. `latest` SHALL NOT qualify as release evidence even
when digest-qualified.

The publish job SHALL expose repository, manifest digest, verification tag, version, and revision to
a separate release-smoke job. That job SHALL verify that the tag currently resolves to the supplied
digest, the exact digest is a multi-platform OCI index containing both required platforms, each
platform image carries the expected OCI labels, and the locally pulled exact reference reports the
same repository digest. It SHALL run the released profile with that exact tag-plus-digest and verify
the Compose-rendered image and running container pin. Success evidence SHALL record the authoritative
GitHub Actions run URL and attempt; failure SHALL preserve actionable profile diagnostics.

#### Scenario: Pinned workbench compatibility is tested

- **GIVEN** the released `ui` profile is started with its immutable SemSource artifact
- **WHEN** the SemSource workbench smoke runs
- **THEN** it verifies the workbench shell, authoritative readiness, source status, search or query
  reachability, accessible search result/detail selection, and the advertised graph state

#### Scenario: Pull request validates without publishing

- **WHEN** a pull request runs CI
- **THEN** UI quality, browser, clean-image, and release-verifier contract gates run
- **AND** no UI image is published and the job has no package-write permission

#### Scenario: Trusted main publication is derived deterministically

- **GIVEN** a trusted push to `main` at a full Git revision
- **WHEN** the UI image is published
- **THEN** it receives `latest` and `sha-<full-revision>` tags
- **AND** the SHA tag, not `latest`, is passed to release verification
- **AND** OCI version is `sha-<full-revision>` and OCI revision is the full revision

#### Scenario: Trusted release publication is derived deterministically

- **GIVEN** a trusted push of a valid `v`-prefixed SemVer tag
- **WHEN** the UI image is published
- **THEN** it receives both the exact `v<semver>` and plain `<semver>` tags
- **AND** the exact `v` tag is passed to release verification
- **AND** OCI version is the plain SemVer and OCI revision is the full revision

#### Scenario: Published manifest becomes release evidence

- **GIVEN** the publish job reports an image repository, verification tag, manifest digest, version,
  and full revision
- **WHEN** the separate release-smoke job verifies the publication
- **THEN** the tag resolves to that exact multi-platform manifest digest
- **AND** required platform labels and the locally observed repository digest match
- **AND** Compose rendering, the running container, and `task ui:smoke` use the same immutable pin
- **AND** success evidence records the run URL and attempt

#### Scenario: Published-image verification fails

- **WHEN** registry provenance, manifest platforms, OCI labels, local repository digest, Compose pin,
  running-container pin, or released-profile smoke does not match
- **THEN** release evidence is not published as successful
- **AND** the CI run retains actionable failure diagnostics

#### Scenario: Headless release path is tested independently

- **WHEN** the SemSource core smoke runs without optional UI profiles
- **THEN** it proves backend readiness and HTTP/MCP/query availability without pulling or starting a UI
  artifact
