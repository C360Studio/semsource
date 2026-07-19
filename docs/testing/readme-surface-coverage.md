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

## Current Blockers And Deliberate Gaps

The former runtime WebSocket restart blocker, `semstreams#490`, is adopted in
SemSource through `github.com/c360studio/semstreams v1.0.0-beta.144`.

- Governed graph drill-down is adopted from SemStreams `v1.0.0-beta.153` (#533, PR #577).
  `TestHTTPGraphProjectionCompatibility` covers the real fusion engine through
  `POST /code-context/context` with `want: ["graph"]`; owned TypeScript/component tests cover strict
  facts/edges/evidence parsing, opaque and explicit unresolved handles, graph-local truncation,
  meaningful coherent nonzero revisions, stale-revision protection, classified errors, and
  non-deletion for partial views. Desktop/narrow Sigma and accessibility tests cover the synchronized
  renderer/navigator/detail surface; `task ui:smoke:dev` proves a non-intercepted Caddy-to-SemSource
  graph request and response. GraphQL projection is deliberately outside this slice.
- Released-profile compatibility remains partial until OpenSpec task 7.3
  publishes and tests an immutable registry digest. The CI mechanism and its contract tests are
  complete; a locally built image, local content digest, or unexecuted workflow is not registry
  evidence.

## UI Image Publication And Release Evidence

| Surface                                                              | Evidence                                                                         | Status  | Follow-up                                         |
| -------------------------------------------------------------------- | -------------------------------------------------------------------------------- | ------- | ------------------------------------------------- |
| PR UI/browser/clean-image verification without publication           | `ui-quality`; `task ui:image:release:test` workflow permission/event assertions  | covered | Observe normal PR CI after merge                  |
| Trusted `main` tags: `latest` + `sha-<full-revision>`                | `ui-image-metadata.sh`; metadata and workflow contract tests                     | covered | `latest` remains forbidden as release evidence    |
| Trusted release tags: `v<semver>` + plain `<semver>`                 | `ui-image-metadata.sh`; valid/invalid SemVer contract tests                      | covered | None for mechanism                                |
| Multi-platform publish outputs and OCI version/full revision         | `publish-ui`; workflow and clean-image metadata tests                            | covered | First real trusted publication still pending      |
| Exact tag-to-manifest/platform/label/local-`RepoDigest` verification | `ui-release-image-verify.sh`; release verifier contract tests                    | covered | First real registry manifest still pending        |
| Exact Compose-rendered and running-container pin proof               | `ui-profile-pin-verify.sh`; pin/preflight contract tests; `task ui:smoke` wiring | covered | First released-profile registry run still pending |
| Success run URL/attempt evidence and failure diagnostics             | `ui-release-smoke`; workflow contract tests                                      | covered | Capture first success artifact and run URL        |
| First immutable registry evidence                                    | None yet                                                                         | gap     | Complete OpenSpec task 7.3.3; keep 7.3 open       |

## Bootstrap And Compose

| Surface                                                                   | Evidence                                                                                        | Status   | Follow-up                                                |
| ------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- | -------- | -------------------------------------------------------- |
| `docker compose up`                                                       | Isolated `task core:smoke` probes the exact core service set, status, sources, GraphQL, and MCP | covered  | None                                                     |
| `SEMSOURCE_UI_IMAGE=<tag>@sha256:<digest> docker compose --profile ui up` | Released mode in `scripts/ui-profile-smoke.sh` enforces the immutable reference                 | partial  | Publish and test the registry digest in OpenSpec 7.3     |
| Explicit `docker-compose.ui-dev.yml` profile                              | `task ui:smoke:dev` passes the desktop and narrow Playwright projects                           | covered  | Local development only                                   |
| `task ui:smoke`                                                           | Released lifecycle path in `scripts/ui-profile-smoke.sh`                                        | partial  | Requires the OpenSpec 7.3 registry artifact              |
| `task ui:smoke:dev`                                                       | `scripts/ui-profile-smoke.sh dev` + six desktop/narrow Playwright tests                         | covered  | Builds local `./ui`; no release claim                    |
| `task ui:e2e`                                                             | `scripts/ui-profile-e2e.sh` + `test/ui/ui-profile.spec.cjs`                                     | covered  | Requires a running released or development UI profile    |
| `task ui:image:verify`                                                    | `scripts/ui-image-verify.sh`                                                                    | covered  | Local production-image proof only; not registry evidence |
| `task ui:image:release:test`                                              | Release metadata/workflow/verifier/pin contract test suite                                      | covered  | No registry access and no release claim                  |
| `go install ...@latest`                                                   | e2e `buildBinary` compiles `./cmd/semsource`                                                    | partial  | Consider install smoke                                   |
| `docker run ... nats:2-alpine -js`                                        | `TestE2E_NativeQuickStart`                                                                      | covered  | None                                                     |
| `git clone ...` / `cd semsource`                                          | External Git behavior                                                                           | external | None                                                     |

## Native CLI

| Surface                                            | Evidence                                                                   | Status  | Follow-up                                     |
| -------------------------------------------------- | -------------------------------------------------------------------------- | ------- | --------------------------------------------- |
| `semsource init`                                   | `go test ./cli -run TestInitWritesValidConfig`                             | covered | Keep CLI tests current                        |
| `semsource init --quick`                           | `TestE2E_NativeQuickStart`                                                 | covered | None                                          |
| `semsource run`                                    | `go test -tags=e2e ./test/e2e/ -run TestE2E_RunStartsAndPublishesEntities` | covered | None                                          |
| `semsource validate`                               | `go test -tags=e2e ./test/e2e/ -run TestE2E_Validate`                      | covered | None                                          |
| `semsource version`                                | `go test -tags=e2e ./test/e2e/ -run TestE2E_Version`                       | covered | None                                          |
| `semsource add ast --path ./src --language go`     | `go test ./cli -run TestAddNonInteractiveAST`                              | covered | None                                          |
| `semsource add repo --url ... --branch main`       | `TestAddNonInteractiveRepo`                                                | covered | None                                          |
| `semsource add docs --paths ...`                   | `TestAddNonInteractiveDocs`                                                | covered | None                                          |
| `semsource add url --urls ... --poll-interval 10m` | `TestAddNonInteractiveURL`                                                 | covered | None                                          |
| interactive `semsource add`                        | None at CLI boundary                                                       | gap     | Add when interactive flow is product-critical |
| `semsource sources`                                | `go test ./cli -run TestSources`                                           | covered | None                                          |
| interactive `semsource remove`                     | `go test ./cli -run TestRemove`                                            | covered | None                                          |
| `semsource remove --index N`                       | `go test ./cli -run TestRemove`                                            | covered | None                                          |

## HTTP And GraphQL

| Surface                                                                                         | Evidence                                                                         | Status  | Follow-up                                                                                         |
| ----------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- | ------- | ------------------------------------------------------------------------------------------------- |
| `GET /source-manifest/sources`                                                                  | `TestHandleSources_GET`; e2e manifest poll; `task core:smoke`                    | covered | None                                                                                              |
| `GET /source-manifest/status`                                                                   | `TestHandleStatus_GET`; e2e poll; UI Playwright assertion                        | covered | None                                                                                              |
| `GET /source-manifest/health`                                                                   | `TestHandleHealth_Ready`; UI Playwright health assertion                         | covered | None                                                                                              |
| `GET /source-manifest/capabilities`                                                             | Capability contract + real-NATS circuit tests                                    | covered | None                                                                                              |
| `POST /supersession/versionDiff`                                                                | `TestIntegration_VersionDiff` NATS and HTTP route assertions                     | covered | None                                                                                              |
| `GET /graph-gateway/graphql`                                                                    | `task core:smoke` ServiceManager route probe                                     | covered | None                                                                                              |
| `POST /graph-gateway/graphql`                                                                   | `task core:smoke` GraphQL-shaped POST assertion                                  | covered | None                                                                                              |
| `POST /graphql` through UI profile                                                              | `test/ui/ui-profile.spec.cjs`                                                    | covered | None                                                                                              |
| `GET /graphql` through UI profile                                                               | Caddy route is configured; no profile-level GET assertion                        | partial | Add only if the playground remains an advertised release surface                                  |
| `GET /health` through UI profile                                                                | `test/ui/ui-profile.spec.cjs` health JSON assertion                              | covered | None                                                                                              |
| `GET /metrics` through UI profile                                                               | `test/ui/ui-profile.spec.cjs` non-HTML successful response assertion             | covered | None                                                                                              |
| `POST /code-context/context`, `/code-context/impact`, `/doc-context/context` through UI profile | `test/ui/ui-profile.spec.cjs` allowlist assertions                               | covered | Other fusion verbs are unit/integration-covered but not advertised here as profile-smoke coverage |
| `POST /code-context/context` with `want: ["graph"]`                                             | Backend compatibility, UI contract/model/component, and real-profile smoke tests | covered | Existing HTTP surface; no parallel endpoint or GraphQL claim                                      |
| `POST /mcp-gateway/mcp` through UI profile                                                      | `test/ui/ui-profile.spec.cjs` non-HTML/non-404 assertion                         | covered | Full MCP initialize/tools proof remains in `task core:smoke`                                      |
| UI `/` and loaded `/_app/*` assets                                                              | `test/ui/ui-profile.spec.cjs` visible shell/search flow                          | covered | None                                                                                              |
| Retired/unknown profile routes return JSON 404                                                  | `test/ui/ui-profile.spec.cjs` route rejection matrix                             | covered | Includes former SemTeams-style/product-placeholder paths                                          |
| `ws://localhost:3000/graph` raw stream                                                          | Browser-native Playwright WebSocket open/close assertion through Caddy           | covered | None                                                                                              |

## MCP And Agent Tools

| Surface                               | Evidence                                                                                  | Status   | Follow-up |
| ------------------------------------- | ----------------------------------------------------------------------------------------- | -------- | --------- |
| `/mcp-gateway/mcp` HTTP endpoint      | MCP in-memory and NATS translation tests; `task core:smoke` initialize + tools/list       | covered  | None      |
| `claude mcp add --transport http ...` | `task core:smoke` proves the SemSource endpoint speaks MCP; Claude CLI config is external | external | None      |
| `add_source`                          | `TestIntegration_AddSourceTranslatesToNATS`                                               | covered  | None      |
| `source_status`                       | `TestIntegration_SourceStatusMergesSignals`                                               | covered  | None      |
| `code_context`                        | `TestIntegration_QueryToolsTranslateToNATS`; fusion NATS integration below MCP            | covered  | None      |
| `code_search`                         | `TestIntegration_QueryToolsTranslateToNATS`; fusion lens tests below MCP                  | covered  | None      |
| `code_impact`                         | `TestIntegration_QueryToolsTranslateToNATS`; code lens impact tests below MCP             | covered  | None      |
| `doc_context`                         | `TestIntegration_QueryToolsTranslateToNATS`                                               | covered  | None      |
| `code_changes`                        | `TestIntegration_QueryToolsTranslateToNATS`; `TestIntegration_VersionDiff` below MCP      | covered  | None      |

## Fused Context Routes

| Surface                      | Evidence                                                                    | Status  | Follow-up |
| ---------------------------- | --------------------------------------------------------------------------- | ------- | --------- |
| `POST /code-context/context` | `TestFusionHTTPErrorContract_PublicHandlers`                                | covered | None      |
| `POST /code-context/callers` | `TestFusionHTTPErrorContract_AllRegisteredRoutes`; code lens relation tests | covered | None      |
| `POST /code-context/callees` | `TestFusionHTTPErrorContract_AllRegisteredRoutes`; code lens relation tests | covered | None      |
| `POST /code-context/impact`  | `TestFusionHTTPErrorContract_AllRegisteredRoutes`; impact tests             | covered | None      |
| `POST /code-context/file`    | `TestFusionHTTPErrorContract_AllRegisteredRoutes`                           | covered | None      |
| `POST /code-context/search`  | `TestFusionHTTPErrorContract_AllRegisteredRoutes`; fusion lens tests        | covered | None      |
| `POST /doc-context/<verb>`   | `TestFusionHTTPErrorContract_PublicHandlers`; route matrix                  | covered | None      |

## Integration-Guide-Only Contracts

| Surface                                 | Owner                         | Evidence                                                       | Status  |
| --------------------------------------- | ----------------------------- | -------------------------------------------------------------- | ------- |
| exhaustive `graph.query.*` catalog      | M5 guide                      | representative SemSource integration tests                     | partial |
| product-owned `graph.query.status`      | SemSource source-manifest     | `TestIntegration_QuerySubjects`; MCP source-status integration | covered |
| product-owned `graph.query.sources`     | SemSource source-manifest     | `TestIntegration_QuerySubjects`                                | covered |
| product-owned `graph.query.predicates`  | SemSource source-manifest     | `TestIntegration_QuerySubjects`; predicate vocabulary tests    | covered |
| product-owned `graph.query.versionDiff` | SemSource supersession        | `TestIntegration_VersionDiff`                                  | covered |
| `/source-manifest/predicates`           | M5 consumer integration guide | `TestHandlePredicates_GET`                                     | covered |
| `/source-manifest/summary`              | M5 consumer integration guide | `TestHandleSummary_GET`; e2e summary poll                      | covered |
