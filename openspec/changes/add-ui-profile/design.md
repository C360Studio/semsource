# Design: add-ui-profile

## Context

SemSource's product boundary is source ingestion and source-graph publishing. A UI
profile should therefore be packaging and integration glue, not a new product layer:
SemSource provides the running backend stack and a stable same-origin route map;
SemTeams UI renders the operator experience and receives feedback when integration
shape mismatches appear.

The current repo already has enough pieces to avoid a large design:

- core compose profile: NATS + semembed + SemSource on `:8080`;
- optional `ui` profile: Caddy on `:3000`, Vite UI on `:5173`;
- source manifest HTTP routes: `/source-manifest/{sources,status,predicates,summary}`;
- query routes: `/graphql`, `/code-context/*`, `/doc-context/*`;
- SemTeams UI: SvelteKit dev server, `Dockerfile.dev`, and Playwright dependency.

The main gaps are the target checkout (`../semstreams-ui` vs `../semteams/ui`),
the `/health` route expected by SemTeams UI, and a SemSource-owned smoke that proves
the composed path.

## Decisions

### D1 - SemSource owns the profile, SemTeams UI owns the app

The `ui` compose profile is a SemSource-owned backend integration profile. It sets
up the SemSource stack, builds/runs the selected UI checkout, and gives the browser
a same-origin Caddy entry point. It does not fork SemTeams UI code or add SemSource
specific UI components here.

### D2 - Default UI checkout is `../semteams/ui`

The profile should default `UI_CONTEXT` to `../semteams/ui`, because that is the
local sibling path for SemTeams UI. `UI_CONTEXT` remains overrideable so a developer
can point at `../semstreams-ui` or an experimental UI checkout without editing the
compose file.

The README must name the default path precisely and avoid the stale
`../semteams-ui` spelling.

### D3 - Caddy is the stable same-origin contract

Caddy remains the public entry point for the profile. Its route map is the contract
the Playwright smoke verifies:

| Route | Target |
| --- | --- |
| `/` | SemTeams UI Vite server |
| `/health` | SemSource health JSON compatible with SemTeams UI's status store |
| `/graphql` | SemSource graph gateway |
| `/source-manifest/*` | SemSource source manifest HTTP API |
| `/code-context/*` | SemSource code fusion gateway |
| `/doc-context/*` | SemSource docs fusion gateway |
| `/mcp-gateway/*` | SemSource MCP gateway |
| `/metrics` | SemSource metrics |
| `/graph` | legacy graph WebSocket |

The `/health` response must be JSON with at least `healthy`, `status`, and
`message`, matching the shape SemTeams UI reads. Caddy proxies this to
SemSource's source-manifest health envelope so the route proves backend reachability
instead of only proving that the proxy process is alive.

### D4 - Playwright smoke is active polling, not log watching

The e2e should assert concrete state:

1. `docker compose --profile ui up` starts the profile.
2. `GET /health` returns `200` and JSON with `healthy: true`.
3. `GET /source-manifest/status` returns SemSource status JSON with a namespace and
   aggregate `phase`.
4. `GET /` renders the SemTeams UI shell rather than an upstream error page.
5. `POST /graphql` reaches the graph gateway and returns a GraphQL-shaped response,
   even if the specific query reports a schema error.

The smoke should poll these endpoints with bounded deadlines and include the last
HTTP response in failure output. It should not infer readiness from a quiet log.

### D5 - Feedback is captured as an integration artifact

SemSource should record findings for SemTeams UI in a dedicated integration note
or issue list. The first expected entries are:

- SemSource's current profile points at `../semstreams-ui`, not SemTeams UI.
- SemTeams UI expects `/health` JSON; SemSource's current Caddyfile only exposes
  `/semsource/health`.
- The graph page tolerates missing `/flowbuilder/flows`, but that request is still
  SemTeams-shaped and should stay best-effort when the backend is SemSource.

## Risks & mitigations

- **SemTeams UI depends on SemTeams-only backend routes.** Mitigation: make the
  SemSource smoke cover only routes SemSource owns; record UI findings separately.
- **Node/Playwright dependencies add weight to a Go repo.** Mitigation: isolate the
  harness under a small test folder or Task target and keep the core Go/e2e gates
  unchanged.
- **`/health` could lie if implemented as a static proxy-only response.** Mitigation:
  proxy `/health` to the source-manifest component's health envelope and keep the
  smoke's independent `/source-manifest/status` assertion.
- **UI profile can disturb developer ports.** Mitigation: keep `C360_PORT`,
  `SEMSOURCE_HTTP_PORT`, `NATS_HOST_PORT`, `NATS_MONITOR_HOST_PORT`, and
  `UI_CONTEXT` overrideable, and document the defaults.

## Alternatives considered

- **Keep targeting semstreams-ui.** Rejected for this slice: the user's target
  consumer is SemTeams UI alpha.1, and the local sibling checkout is `../semteams/ui`.
- **Move all e2e into SemTeams UI.** Rejected: SemTeams UI should test its app, but
  SemSource owns whether its compose/Caddy profile actually serves the backend
  contract.
- **Build a SemSource-native UI.** Rejected: product ownership belongs to SemTeams UI;
  SemSource should not grow a parallel frontend.
