# Design: cover-readme-surfaces

## Context

SemSource's README is now product-facing. That means README examples are not
casual snippets; they are promises. For this change, "advertised surface" means
anything a user can copy, connect to, call, or depend on from README:

- shell commands (`semsource ...`, `docker compose ...`, `task ...`, `go test ...`);
- environment-driven modes (`SEMSOURCE_CONFIG`, `SEMSOURCE_TARGET`, `UI_CONTEXT`,
  `C360_PORT`, NATS host-port overrides);
- HTTP and GraphQL routes;
- NATS request/reply subjects;
- MCP tools;
- fused context verbs.

The current coverage splits into three quality levels:

| Level | Meaning | Example today |
| --- | --- | --- |
| Black-box | Built binary or composed stack exercises the user-visible path | `semsource run` e2e |
| Integration | Component uses real NATS or HTTP handler wiring | MCP `add_source` to NATS |
| Unit/config | Parser, handler, config, or factory shape is verified | graph-query port list |

All README surfaces should have at least one named test. User-facing commands and
primary routes should prefer black-box or integration coverage.

## Current Audit Snapshot

### Covered or mostly covered

- `semsource version`: black-box e2e.
- `semsource validate`: black-box e2e.
- `semsource run`: black-box e2e starts the compiled binary, publishes entities,
  and checks source-manifest status/summary.
- Source handlers: broad unit/integration coverage for `ast`, `git`, `docs`,
  `config`, `url`, `image`, `video`, and `audio`.
- Config fields: strong loader/default/validation coverage.
- UI profile: `task ui:smoke` and `task ui:e2e` cover `/health`,
  `/source-manifest/status`, `/graphql`, and the SemTeams shell.

### Partial

- `semsource init --quick`: package-level coverage exists, but not the exact
  binary command chained into `validate` and `run`.
- `semsource add`: one `ast` happy path plus some errors are covered; README
  examples for `repo`, `docs`, and `url` are not directly covered.
- `source-manifest` HTTP: `/sources` and `/health` have happy tests; `/status`,
  `/summary`, and `/predicates` need explicit happy-route assertions.
- NATS query subjects: SemSource config wires the graph-query ports, but most
  listed subjects are not requested in SemSource tests.
- MCP query tools: tool list and guardrails are tested; query happy paths are
  not covered through MCP.
- GraphQL: the UI smoke verifies a GraphQL-shaped response through Caddy, but
  core-profile ServiceManager GraphQL behavior still needs a representative
  route test after the upstream fix lands.

### Missing

- `semsource sources`, interactive `semsource remove`, and
  `semsource remove --index N`.
- Default `docker compose up` / core profile smoke for `:8080` status, sources,
  MCP connection, and a minimal query/tool path.
- A coverage matrix that fails review when the README adds a command without a
  matching test.

## Decisions

### D1 - README surfaces need named evidence

Every executable or addressable README surface must have one of:

1. a named unit/integration/e2e test;
2. a named smoke task;
3. an explicit temporary blocker tied to an upstream issue or release tag.

Undocumented local internals do not need this matrix. README surfaces do.

### D2 - CLI examples get command-level tests

Package tests can cover parser details, but README commands should be tested at
the CLI boundary. For cheap config-mutating commands, `cli` package tests are
enough when they exercise the same argument list as README. For end-to-end setup,
use the compiled binary.

Minimum CLI additions:

- add non-interactive happy-path tests for `add repo`, `add docs`, and `add url`;
- add direct tests for `sources`, `remove`, and `remove --index`;
- add black-box quick-start test for `init --quick`, `validate`, and `run`.

### D3 - Core compose smoke waits for the SemStreams P0 tag

The default Docker path should be proved by a one-command smoke similar to the UI
profile smoke. Because the user has identified a pending SemStreams P0 fix, this
task is tracked now but should only be gated after SemSource adopts the fixed
SemStreams tag.

The smoke should start only the core profile, poll concrete HTTP state, connect
to MCP, and tear down cleanly. It should not rely on log silence.

### D4 - MCP happy paths can start with stubbed NATS, then graduate

MCP query tools can first be tested with NATS stub responders to prove the MCP
tool -> SemSource subject -> response translation. The core compose smoke should
then add at least one real stack path, such as `source_status` and one source
query tool against indexed fixture content.

### D5 - API subject coverage should be representative, not exhaustive semantics

SemStreams owns most graph-query implementation semantics. SemSource owns whether
the documented subjects/routes are reachable in its assembled stack. For subjects
listed in README, the SemSource tests should at least prove:

- product-owned subjects (`graph.query.sources`, `graph.query.status`,
  `graph.query.predicates`, `graph.query.versionDiff`) return the advertised
  shape;
- framework-owned graph-query subjects are configured and a representative subset
  responds in a running stack after the fixed SemStreams tag is adopted.

## Risks & Mitigations

- **Core smoke becomes slow or flaky.** Keep it small, use bounded polling, and
  assert concrete HTTP/MCP state instead of broad graph quality.
- **SemStreams P0 blocks runtime smoke.** Mark those tasks blocked until a fixed
  tag is adopted; keep CLI/package coverage moving.
- **README grows faster than tests.** Add a lightweight coverage matrix in the
  OpenSpec task checklist or docs and update it in the same PR as README changes.
- **Duplicate testing across SemStreams and SemSource.** SemSource tests assembly
  and product-owned surfaces; SemStreams tests substrate behavior.
