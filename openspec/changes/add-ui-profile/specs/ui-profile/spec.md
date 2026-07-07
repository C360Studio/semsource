# ui-profile

## ADDED Requirements

### Requirement: SemTeams UI profile

SemSource SHALL provide an optional Docker Compose `ui` profile that layers a
SemTeams UI checkout and a same-origin Caddy proxy on top of the core SemSource
stack without changing the default backend-only profile.

When the `ui` profile is enabled, the default UI build context SHALL be the sibling
SemTeams UI checkout at `../semteams/ui`, while allowing operators to override it
with `UI_CONTEXT`. The core profile (`docker compose up` without `--profile ui`)
SHALL NOT require any sibling UI checkout and SHALL continue to publish the
SemSource HTTP/MCP API on the configured SemSource HTTP port.

#### Scenario: UI profile uses the SemTeams UI checkout by default

- **GIVEN** `UI_CONTEXT` is unset
- **WHEN** the `ui` profile compose config is rendered
- **THEN** the UI service build context resolves to `../semteams/ui`

#### Scenario: Core profile does not require the UI checkout

- **GIVEN** the operator runs `docker compose up` without `--profile ui`
- **WHEN** Compose renders and starts the core stack
- **THEN** the SemSource, semembed, and NATS services start without reading the UI
  checkout path

#### Scenario: UI context remains overrideable

- **GIVEN** `UI_CONTEXT` points at an alternate local UI checkout
- **WHEN** the `ui` profile compose config is rendered
- **THEN** the UI service uses that override for its build context and development
  mounts

### Requirement: Same-origin SemSource routes for the UI profile

The UI profile SHALL expose a Caddy entry point on the configured C360 port that
serves the UI shell and routes SemSource backend APIs on the same origin. The route
map SHALL include `/health`, `/graphql`, `/source-manifest/*`, `/code-context/*`,
`/doc-context/*`, `/mcp-gateway/*`, `/metrics`, and the legacy `/graph` WebSocket.

The `/health` route SHALL proxy a SemSource-owned health envelope and return HTTP
200 JSON compatible with SemTeams UI's system status reader, including at least
`component: semsource`, `healthy`, `status`, `message`, `namespace`, `phase`, and
`total_entities`. It SHALL NOT be a static proxy-only response, fall through to
the UI server, or return non-JSON text.

#### Scenario: Health is machine-readable for the UI

- **WHEN** a browser or Playwright test requests `GET /health` through the Caddy
  entry point
- **THEN** it receives HTTP 200 JSON from SemSource with `component: semsource`,
  `healthy: true`, source-manifest status fields, and a human-readable message

#### Scenario: Source status is proxied through Caddy

- **WHEN** a browser requests `GET /source-manifest/status` through the Caddy entry
  point
- **THEN** it receives the SemSource status payload with `namespace` and aggregate
  `phase`

#### Scenario: GraphQL is routed to the SemSource graph gateway

- **WHEN** a browser posts a GraphQL request to `/graphql` through the Caddy entry
  point
- **THEN** the request reaches the SemSource graph gateway and returns a
  GraphQL-shaped JSON response rather than UI HTML or a proxy 404

### Requirement: Light Playwright UI-profile smoke

SemSource SHALL provide a lightweight Playwright e2e smoke for the `ui` profile that
actively polls the Caddy entry point and asserts the UI shell and SemSource backend
routes are reachable. The smoke SHALL have bounded deadlines and actionable failure
messages that include the last observed HTTP response for failed endpoint polls.
SemSource SHALL also provide a one-command smoke target that starts the UI profile,
runs the Playwright assertions, and tears the UI profile down by default. Before
starting Docker, the one-command target SHALL preflight the configured UI checkout
and fail fast when `UI_CONTEXT`, `Dockerfile.dev`, or the local Playwright
installation is missing.

#### Scenario: UI profile smoke proves the operator path

- **GIVEN** the `ui` profile stack is running
- **WHEN** the Playwright smoke is executed against the configured Caddy URL
- **THEN** it verifies `/health`, `/source-manifest/status`, `/graphql`, and `/`
  through the same origin an operator uses

#### Scenario: Smoke failures include concrete state

- **WHEN** an endpoint never reaches the expected state before the deadline
- **THEN** the failing assertion reports the endpoint, final HTTP status, and final
  response body snippet

#### Scenario: One-command smoke owns profile lifecycle

- **WHEN** an operator runs the UI-profile smoke target
- **THEN** it validates the UI checkout prerequisites, starts
  `docker compose --profile ui`, runs the Playwright smoke against the Caddy
  origin, and tears the profile down when the smoke exits

#### Scenario: One-command smoke fails before Docker when UI checkout is absent

- **GIVEN** `UI_CONTEXT` points at a missing or unprepared checkout
- **WHEN** an operator runs the UI-profile smoke target
- **THEN** it reports the missing prerequisite and does not start the compose stack

### Requirement: SemTeams UI integration feedback is recorded

SemSource SHALL record actionable SemTeams UI integration findings discovered while
validating the profile. Each finding SHALL identify whether the next action belongs
to SemSource, SemTeams UI, or SemStreams, and SHALL avoid committing sibling-repo UI
changes from the SemSource checkout.

#### Scenario: A UI integration mismatch is found during smoke work

- **WHEN** the SemSource profile exposes an endpoint shape that SemTeams UI does not
  consume correctly, or SemTeams UI expects a route SemSource does not own
- **THEN** the finding is recorded with owner and evidence before or alongside any
  SemSource fix
