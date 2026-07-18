# Historical SemSource UI Profile Feedback for SemTeams UI

> **Historical and superseded.** This note records validation of the former
> `add-ui-profile` implementation. It is not a current integration contract or
> delivery request. The SemSource `ui` flag now launches the SemSource-owned
> workbench from this repository; it no longer builds, mounts, or validates a
> SemTeams checkout. `UI_CONTEXT`, the sibling path, and SemTeams-only route
> expectations below describe the retired profile.

SemTeams now owns its application packaging and consumes SemSource through the
unchanged UI-free HTTP, MCP, NATS, GraphQL, and governed graph contracts. The
current SemSource profile contract lives in the
[`add-opt-in-source-workbench`](../../openspec/changes/add-opt-in-source-workbench/)
change. Future SemTeams application findings belong in the SemTeams repository.

This note originally tracked findings from validating SemSource's
`docker compose --profile ui` against the SemTeams UI checkout. It was an
integration feedback list, not a SemSource specification.

SemTeams UI follow-up issue:
[C360Studio/semteams#244](https://github.com/C360Studio/semteams/issues/244).

## Findings

### 1. Default UI checkout path — SemSource-owned — addressed in `add-ui-profile`

SemSource's existing compose profile targeted `../semstreams-ui`. The current
SemTeams UI checkout is `../semteams/ui`, so the profile now defaults `UI_CONTEXT`
there while keeping the override.

### 2. `/health` route shape — SemSource-owned — addressed in `add-ui-profile`

SemTeams UI's `systemStatus` store polls `/health` and expects JSON with fields such
as `healthy`, `status`, and `message`. SemSource's Caddyfile only exposed
`/semsource/health` as plain text. The UI profile now proxies `/health` to
SemSource's source-manifest health envelope and the Playwright smoke asserts it.

### 3. SemTeams-only routes on SemSource backend — SemTeams UI-owned follow-up

SemTeams UI also attempts SemTeams backend routes such as `/teams-dispatch/*`,
`/teams-loop/*`, `/flowbuilder/*`, and `/graph/triples`. Those are not SemSource
contracts. The SemSource smoke intentionally asserts only the shared shell plus
SemSource-owned `/health`, `/source-manifest/status`, and `/graphql` routes.

Recommended UI-side behavior: when connected to a SemSource profile, missing
SemTeams-only routes should degrade quietly and leave source/graph inspection usable.

### 4. `/graphql` proxy target — SemSource-owned — addressed in `add-ui-profile`

The first Playwright smoke reached `/health` and `/source-manifest/status`, then found
`/graphql` returning Caddy 502s. The live SemSource graph gateway route is served by the
SemStreams ServiceManager at `/graph-gateway/graphql` on port `8080`; `8082` is not a
listening endpoint in the current ServiceManager mode. The Caddy profile now exposes
operator-facing `/graphql` by rewriting to `/graph-gateway/graphql` on `semsource:8080`.

### 5. UI image dependency audit — SemTeams UI-owned follow-up

The first uncached SemTeams UI image build completed, but `npm install` reported 24 audit
findings: 1 low, 7 moderate, 14 high, and 2 critical. This does not block the SemSource
profile smoke, but SemTeams UI should triage the dependency tree before beta.
