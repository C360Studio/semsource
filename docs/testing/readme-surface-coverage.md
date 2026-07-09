# README Surface Coverage

This matrix tracks SemSource-owned commands, routes, tasks, and tools advertised
from the root README. The README is product-facing; anything copied from it should
have named evidence, a named implementation gap, or a deliberate ownership note.

Status values:

- `covered`: a named unit, integration, e2e, or smoke test exercises the surface.
- `partial`: related tests exist, but they do not exercise the advertised path.
- `gap`: no direct evidence yet; the matching OpenSpec task owns the work.
- `external`: generic setup outside SemSource's product contract.

Low-level `graph.query.*` subjects and predicate/schema routes belong in
`docs/integration/m5-consumer-integration.md`, not the README. The README should
link to that guide unless a low-level subject is required for first-run usage.

## Current Upstream Blockers

No active README-surface blocker is currently recorded. The former runtime
WebSocket restart blocker, `semstreams#490`, is adopted in SemSource through
`github.com/c360studio/semstreams v1.0.0-beta.144`.

Remaining core compose smoke work is SemSource-owned coverage, not an upstream block.

## Bootstrap And Compose

| Surface | Evidence | Status | Follow-up |
| --- | --- | --- | --- |
| `docker compose up` | `task core:smoke` starts the default profile and probes status/sources/MCP tools | covered | None |
| `docker compose --profile ui up` | `task ui:smoke` | covered | Keep synced with UI profile |
| `task ui:smoke` | `scripts/ui-profile-smoke.sh` | covered | Runs profile + Playwright |
| `task ui:e2e` | `test/ui/ui-profile.spec.cjs` | covered | Requires running UI profile |
| `go install ...@latest` | e2e `buildBinary` compiles `./cmd/semsource` | partial | Consider install smoke |
| `docker run ... nats:2-alpine -js` | `TestE2E_NativeQuickStart` | covered | None |
| `git clone ...` / `cd semsource` | External Git behavior | external | None |

## Native CLI

| Surface | Evidence | Status | Follow-up |
| --- | --- | --- | --- |
| `semsource init` | `go test ./cli -run TestInitWritesValidConfig` | covered | Keep CLI tests current |
| `semsource init --quick` | `TestE2E_NativeQuickStart` | covered | None |
| `semsource run` | `go test -tags=e2e ./test/e2e/ -run TestE2E_RunStartsAndPublishesEntities` | covered | None |
| `semsource validate` | `go test -tags=e2e ./test/e2e/ -run TestE2E_Validate` | covered | None |
| `semsource version` | `go test -tags=e2e ./test/e2e/ -run TestE2E_Version` | covered | None |
| `semsource add ast --path ./src --language go` | `go test ./cli -run TestAddNonInteractiveAST` | covered | None |
| `semsource add repo --url ... --branch main` | `TestAddNonInteractiveRepo` | covered | None |
| `semsource add docs --paths ...` | `TestAddNonInteractiveDocs` | covered | None |
| `semsource add url --urls ... --poll-interval 10m` | `TestAddNonInteractiveURL` | covered | None |
| interactive `semsource add` | None at CLI boundary | gap | Add when interactive flow is product-critical |
| `semsource sources` | `go test ./cli -run TestSources` | covered | None |
| interactive `semsource remove` | `go test ./cli -run TestRemove` | covered | None |
| `semsource remove --index N` | `go test ./cli -run TestRemove` | covered | None |

## HTTP And GraphQL

| Surface | Evidence | Status | Follow-up |
| --- | --- | --- | --- |
| `GET /source-manifest/sources` | `TestHandleSources_GET`; e2e manifest poll; `task core:smoke` | covered | None |
| `GET /source-manifest/status` | e2e poll; UI Playwright assertion | covered | Add direct handler test |
| `GET /source-manifest/health` | `TestHandleHealth_Ready`; UI Playwright health assertion | covered | None |
| `POST /supersession/versionDiff` | NATS diff integration exists; no HTTP route test | partial | OpenSpec task 5.1 |
| `GET /graph-gateway/graphql` | UI profile GraphQL-shaped Playwright assertion | partial | OpenSpec task 5.3 |
| `POST /graph-gateway/graphql` | UI profile `/graphql` POST assertion | partial | OpenSpec task 5.3 |
| `GET/POST /graphql` through UI profile | `test/ui/ui-profile.spec.cjs` | covered | None |
| `ws://localhost:3000/graph` raw stream | WebSocket output wired | partial | Track if README keeps endpoint |

## MCP And Agent Tools

| Surface | Evidence | Status | Follow-up |
| --- | --- | --- | --- |
| `/mcp-gateway/mcp` HTTP endpoint | MCP in-memory and NATS translation tests; `task core:smoke` initialize + tools/list | covered | None |
| `claude mcp add --transport http ...` | `task core:smoke` proves the SemSource endpoint speaks MCP; Claude CLI config is external | external | None |
| `add_source` | `TestIntegration_AddSourceTranslatesToNATS` | covered | None |
| `source_status` | `TestIntegration_SourceStatusMergesSignals` | covered | None |
| `code_context` | `TestIntegration_QueryToolsTranslateToNATS`; fusion NATS integration below MCP | covered | None |
| `code_search` | `TestIntegration_QueryToolsTranslateToNATS`; fusion lens tests below MCP | covered | None |
| `code_impact` | `TestIntegration_QueryToolsTranslateToNATS`; code lens impact tests below MCP | covered | None |
| `doc_context` | `TestIntegration_QueryToolsTranslateToNATS` | covered | None |
| `code_changes` | `TestIntegration_QueryToolsTranslateToNATS`; `TestIntegration_VersionDiff` below MCP | covered | None |

## Fused Context Routes

| Surface | Evidence | Status | Follow-up |
| --- | --- | --- | --- |
| `POST /code-context/context` | fusion NATS/live-graph integration below HTTP route | partial | Add HTTP route test |
| `POST /code-context/callers` | code lens relation tests below HTTP route | partial | Add HTTP route test |
| `POST /code-context/callees` | code lens relation tests below HTTP route | partial | Add HTTP route test |
| `POST /code-context/impact` | `TestCodeLens_ImpactWalksTypeDependencies` | partial | Add HTTP route test |
| `POST /code-context/file` | code context component tests below route | partial | Add HTTP route test |
| `POST /code-context/search` | fusion lens tests below HTTP route | partial | Add HTTP route test |
| `POST /doc-context/<verb>` | docs lens tests below HTTP route | partial | Add HTTP route test |

## Integration-Guide-Only Contracts

| Surface | Owner | Evidence | Status |
| --- | --- | --- | --- |
| exhaustive `graph.query.*` catalog | M5 guide | representative SemSource integration tests | partial |
| product-owned `graph.query.status` | SemSource source-manifest | MCP source-status integration | covered |
| product-owned `graph.query.sources` | SemSource source-manifest | source-manifest HTTP tests only | gap |
| product-owned `graph.query.predicates` | SemSource source-manifest | predicate vocabulary tests only | gap |
| product-owned `graph.query.versionDiff` | SemSource supersession | `TestIntegration_VersionDiff` | covered |
| `/source-manifest/predicates` | M5 consumer integration guide | predicate vocabulary tests only | gap |
| `/source-manifest/summary` | M5 consumer integration guide | e2e summary poll | covered |
