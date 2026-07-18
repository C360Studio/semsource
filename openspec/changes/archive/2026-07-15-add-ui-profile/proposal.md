## Why

SemSource is at beta.1 with a useful local backend stack: `docker compose up`
starts NATS, semembed, and SemSource, and the SemSource HTTP/MCP surface is
available on `:8080`. The repo already contains a `ui` compose profile and a Caddy
single-origin proxy, but that profile is wired to the older `../semstreams-ui`
checkout. The sibling SemTeams UI has now reached alpha.1 and lives locally under
`../semteams/ui`; it has a development Dockerfile and Playwright coverage.

This is the right moment to make SemSource's UI profile a governed integration
surface instead of a convenient compose stanza. SemSource should own the backend
profile and proxy contract; SemTeams UI should own UI behavior. The integration
needs a light Playwright smoke so we can prove that the dashboard boots and reaches
SemSource through the same origin the operator uses.

Research notes from the current checkout:

- `openspec --version` reports `1.5.0`; project context is now in
  `openspec/config.yaml`.
- There are no active OpenSpec changes.
- SemSource `docker-compose.yml` already defines a `ui` profile with Caddy on
  `:3000`, but its default `UI_CONTEXT` is `../semstreams-ui`.
- There is no `../semteams-ui` checkout; the SemTeams UI is present at
  `../semteams/ui`.
- SemTeams UI polls `/health` as JSON and uses `/graphql` for graph exploration.
  SemSource's current Caddyfile exposes `/semsource/health`, not `/health`, so
  backend health can look broken even when the UI page loads.

## What Changes

- Make the `ui` profile target the SemTeams UI checkout by default while retaining
  an overrideable `UI_CONTEXT` for local experiments.
- Keep Caddy as the single entry point on `C360_PORT` and explicitly proxy the
  SemSource surfaces the UI and operators need: `/health`, `/graphql`,
  `/source-manifest/*`, `/code-context/*`, `/doc-context/*`, `/mcp-gateway/*`,
  `/metrics`, and `/graph`.
- Add a light SemSource-owned Playwright e2e smoke that runs against the composed
  `ui` profile, actively polling HTTP state instead of relying on logs.
- Update README and integration notes to describe the SemTeams UI profile, the
  default checkout path, the endpoint contract, and where SemTeams UI feedback is
  recorded.
- Record actionable SemTeams UI integration feedback found during the slice without
  committing changes to the sibling repo from SemSource.

## Non-goals

- No SemTeams UI feature work in this repository. SemSource may document feedback
  and open follow-up issues, but it does not own SemTeams UI routes, components,
  or styling.
- No SemStreams substrate changes. If the graph gateway, health, or query APIs need
  framework changes, they go to `docs/upstream/semstreams-asks.md` / GitHub issues.
- No production authentication or TLS hardening for the UI profile. This remains a
  local/operator profile on the trusted dev network.
- No broad visual regression suite. The e2e coverage is a smoke that proves boot,
  proxy routing, backend health, and a minimal graph/status path.
- No migration away from the core profile. `docker compose up` without `--profile
  ui` remains the shippable backend/agent path with no sibling UI checkout.

## Consumers

- SemTeams UI: consumes SemSource's same-origin proxy and graph/status endpoints
  for operator-facing source/graph inspection.
- SemTeams agents and Claude Code: continue to consume the core MCP/HTTP surfaces;
  the UI profile must not disturb `:8080` direct MCP access.
- SemOps / SemSpec / SemDragon: indirect consumers of the SemSource graph; they are
  unaffected by the local UI profile except for improved operator validation.

## Capabilities

### New Capabilities

- `ui-profile`: a local Docker Compose profile and Caddy proxy contract that hosts a
  SemTeams UI checkout against SemSource's backend stack and proves the integration
  with a light Playwright smoke.

## Impact

- `docker-compose.yml` and `Caddyfile` become the governed UI-profile runtime
  surface.
- `README.md` and integration docs describe the SemTeams UI profile and feedback
  loop.
- `test/ui` or an equivalent SemSource-owned e2e location gains a small Playwright
  harness.
- `Taskfile.yml` gains a focused UI-profile e2e target if one does not already
  exist.
