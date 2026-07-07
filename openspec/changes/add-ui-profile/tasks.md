# Tasks: SemTeams UI profile and smoke

## 1. Confirm current contracts

- [x] 1.1 Verify OpenSpec 1.5 context is read from `openspec/config.yaml` and update
      stale OpenSpec docs that still point to `openspec/project.md`.
      - Test: `openspec validate add-ui-profile --strict`
- [x] 1.2 Verify the sibling UI checkout path and dev contract:
      `../semteams/ui/package.json`, `../semteams/ui/Dockerfile.dev`, and
      `../semteams/ui/playwright.config.ts`.
      - Test: documented finding in the change/design or integration notes

## 2. Wire the SemTeams UI profile

- [x] 2.1 Change the `ui` profile default `UI_CONTEXT` from `../semstreams-ui` to
      `../semteams/ui`; keep the environment override and volume mounts working for
      SvelteKit/Vite development.
      - Test: `docker compose --profile ui config` includes the expected UI build
        context when `UI_CONTEXT` is unset
- [x] 2.2 Update Caddy route handling so `/health` returns SemTeams UI-compatible
      JSON and SemSource routes remain same-origin (`/graphql`,
      `/source-manifest/*`, `/code-context/*`, `/doc-context/*`, `/mcp-gateway/*`,
      `/metrics`, `/graph`).
      - Test: focused Playwright/API assertions in task 3
- [x] 2.2a Serve `/health` from SemSource's source-manifest health envelope instead
      of a static Caddy response so the UI status reflects backend reachability.
      - Test: `go test ./processor/source-manifest` and `task ui:smoke`
- [x] 2.3 Preserve the core profile: `docker compose up` without `--profile ui` still
      starts the backend/MCP stack with no sibling UI checkout.
      - Test: `docker compose config` succeeds with no `UI_CONTEXT` checkout needed
- [x] 2.4 Keep both NATS host ports overrideable so a separate local NATS stack does
      not block the UI-profile smoke.
      - Test: `NATS_HOST_PORT=14222 NATS_MONITOR_HOST_PORT=18222 task ui:smoke`

## 3. Add light Playwright e2e

- [x] 3.1 Add a SemSource-owned Playwright harness (for example under `test/ui/`) and
      a Task target such as `task ui:e2e` that runs against `C360_PORT` with a
      bounded timeout.
      - Test: `task ui:e2e -- --help` or the package's equivalent command resolves
        locally
- [x] 3.2 Implement the smoke:
      - poll `GET /health` until JSON reports `healthy: true`;
      - poll `GET /source-manifest/status` until status JSON has `namespace` and
        `phase`;
      - load `/` and assert the SemTeams UI shell is visible;
      - verify `/graphql` is routed to the graph gateway with a GraphQL-shaped POST
        response.
      - Test: `task ui:e2e` passes against `docker compose --profile ui up`
- [x] 3.3 Make failures actionable: include the final HTTP status/body for each
      polled endpoint and do not rely on log silence as success.
      - Test: review the Playwright failure messages or a forced failing run
- [x] 3.4 Add a one-command UI profile smoke that starts the compose profile, runs
      `task ui:e2e`, and tears the stack down by default.
      - Test: `task ui:smoke`
- [x] 3.5 Strengthen the UI smoke so `/health` must be the live SemSource
      source-manifest health envelope, not a static proxy stub.
      - Test: `task ui:smoke`
- [x] 3.6 Add a fail-fast `task ui:smoke` preflight for the configured UI checkout,
      Dockerfile, and Playwright installation.
      - Test: `UI_CONTEXT=/private/tmp/does-not-exist task ui:smoke`

## 4. Documentation and feedback loop

- [x] 4.1 Update README Docker Compose docs for the SemTeams UI profile, the
      `../semteams/ui` default, and the route map.
      - Test: markdown lint/readthrough; links point to existing files
- [x] 4.2 Add or update an integration feedback note for SemTeams UI findings found
      during this slice.
      - Test: note names concrete findings and whether each belongs to SemSource or
        SemTeams UI
- [x] 4.3 Update any stale `semstreams-ui` wording that now describes the SemTeams UI
      profile while keeping historical asks honest where they still refer to the
      old generic UI.
      - Test: `rg "semstreams-ui|semteams-ui|semteams/ui|UI_CONTEXT" README.md docs docker-compose.yml Caddyfile`

## 5. Gates

- [x] 5.1 `openspec validate add-ui-profile --strict`
- [x] 5.2 `go test ./...`
- [ ] 5.3 `go test -tags=e2e -timeout 300s ./test/e2e/`
      - Result 2026-07-06: failed in existing `TestE2E_RuntimeSourceAdd` with
        `panic: duplicate metrics collector registration attempted` from
        `github.com/c360studio/semstreams/output/websocket.newMetrics` during a
        runtime websocket-output component restart. This is outside the UI profile
        route/smoke path; filed upstream as
        [semstreams#490](https://github.com/C360Studio/semstreams/issues/490).
- [x] 5.4 `task lint` or the repo's revive/gofmt/go vet equivalent
- [x] 5.5 UI profile smoke: `docker compose --profile ui up` plus `task ui:e2e`, then
      teardown the stack
- [x] 5.6 One-command UI profile smoke: `task ui:smoke`
