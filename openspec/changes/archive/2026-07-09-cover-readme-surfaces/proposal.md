## Why

The README now presents SemSource as a beta product with a broad executable
surface: native CLI setup, Docker Compose bringup, MCP tools, operator-facing
source-manifest HTTP routes, GraphQL, fused code/doc context routes, UI profile
smoke, and local test commands.

The low-level NATS subject catalog and predicate/schema routes are consumer
integration details, not README lead material. The README should point to the M5
consumer integration guide for those contracts instead of enumerating them.

That is the right product story, but the coverage is uneven. The current audit
found strong handler/source coverage, good e2e coverage for `version`, `validate`,
and `run`, and a real UI-profile smoke. It also found gaps in the commands and
routes a new user is most likely to copy from the README:

- `semsource add repo/docs/url`, `semsource sources`, and `semsource remove`
  are advertised but do not have direct command-level tests.
- `semsource init --quick` is tested as a package function but not as a black-box
  quick-start flow that feeds `validate` and `run`.
- `docker compose up` is the primary path, but there is no core-profile smoke
  proving the `:8080` status/MCP surface.
- MCP query tools are listed and schema-checked, but most do not have happy-path
  tests through the agent-facing tool surface.
- Several HTTP/GraphQL/product-owned NATS surfaces are wiring-checked but not
  behavior-checked as advertised product routes or integration-guide contracts.

The previously blocking SemStreams runtime restart fix is now adopted in
SemSource through `github.com/c360studio/semstreams v1.0.0-beta.144`. This change
tracks the remaining SemSource-owned coverage gaps.

## What Changes

- Define an advertised-surface coverage rule for README commands, tasks, tools,
  and operator-facing endpoints.
- Keep low-level NATS subjects and predicate/schema routes in integration docs,
  with explicit owner/test/blocker tracking there.
- Add focused test tasks for the native CLI source-management examples.
- Add a black-box native quick-start e2e that proves `init --quick -> validate ->
  run` using a compiled binary.
- Add a default Docker Compose/core smoke for the primary `docker compose up`
  path.
- Add MCP happy-path coverage for the agent tools listed in the README.
- Add behavior checks for the advertised source-manifest, GraphQL, and
  product-owned NATS query surfaces that are currently only partially covered.
- Keep the UI-profile smoke as already-covered evidence, while preserving it in
  the coverage matrix.

## Non-goals

- No SemStreams substrate fix in this repo. If a route or query fails because of
  a future SemStreams issue, SemSource tracks the dependency explicitly and files
  it upstream.
- No attempt to e2e every possible graph query combination. The requirement is a
  representative happy path for every advertised surface, with deeper behavior
  covered in component-specific tests.
- No expansion of the README surface while this change is open unless the new
  surface lands with tests in the same slice.
- No production deployment hardening, authentication redesign, or TLS work.

## Consumers

- New SemSource users copying commands from the README.
- SemTeams UI and operator workflows that depend on documented status/GraphQL/UI
  routes.
- Agent users connecting through MCP and expecting the listed tools to work.
- SemOps, SemSpec, and other sem* consumers that rely on status, graph query, and
  fused context surfaces.

## Capabilities

### New Capabilities

- `advertised-surface-coverage`: a coverage contract tying README-advertised
  commands and routes to named tests before SemSource treats them as beta product
  surface.

## Impact

- Tests under `cli/`, `cmd/semsource`, `test/e2e/`, `processor/mcp-gateway/`,
  `processor/source-manifest/`, and possibly a new core smoke script/task.
- README wording may be tightened if a surface is not ready to test yet.
- Taskfile may gain a `core:smoke` or similar default-profile smoke target.
- OpenSpec and CI evidence should clearly show which advertised surfaces are
  covered, which are SemSource gaps, and which are blocked upstream.
