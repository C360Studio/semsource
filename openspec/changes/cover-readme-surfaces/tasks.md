# Tasks: advertised README surface coverage

## 1. Establish the coverage matrix

- [ ] 1.1 Create a small README surface coverage matrix that lists every advertised
      command, task, endpoint, subject, and MCP tool with its owning test.
      - Test: `rg "semsource add|docker compose up|mcp-gateway|graph.query" README.md`
        entries have named evidence in the matrix
- [ ] 1.2 Mark any surface blocked by the pending SemStreams P0 fix with the issue,
      expected fixed tag, and follow-up validation command.
      - Test: matrix names the upstream blocker instead of silently omitting the
        surface

## 2. Cover native CLI commands from README

- [ ] 2.1 Add direct non-interactive `cli.Add` tests for the README examples:
      `add repo --url ... --branch main`, `add docs --paths ...`, and
      `add url --urls ... --poll-interval 10m`.
      - Test: `go test ./cli -run 'TestAddNonInteractive(Repo|Docs|URL)'`
- [ ] 2.2 Add config/output tests for `semsource sources`.
      - Test: `go test ./cli -run TestSources`
- [ ] 2.3 Add config mutation tests for `semsource remove --index N` and
      interactive `semsource remove`.
      - Test: `go test ./cli -run TestRemove`
- [ ] 2.4 Add a black-box dispatch or e2e test that runs the compiled binary through
      `semsource init --quick --config <tmp>`, `semsource validate --config <tmp>`,
      and `semsource run --config <tmp>` with a disposable NATS server.
      - Test: `go test -tags=e2e -timeout 300s ./test/e2e/ -run TestE2E_NativeQuickStart`

## 3. Cover default Docker Compose/core profile

- [ ] 3.1 Add a core-profile smoke script/task that starts `docker compose up` for
      the default profile, polls `http://localhost:8080/source-manifest/status`,
      and tears the stack down.
      - Test: `task core:smoke`
- [ ] 3.2 Extend the smoke to assert
      `http://localhost:8080/source-manifest/sources` and the MCP gateway endpoint
      are reachable.
      - Test: `task core:smoke`
- [ ] 3.3 After the fixed SemStreams tag lands, include one real MCP happy path
      against the core stack.
      - Test: `task core:smoke`

## 4. Cover MCP tools advertised to agents

- [ ] 4.1 Add NATS-backed MCP happy-path tests for `code_context`, `code_search`,
      `code_impact`, `doc_context`, and `code_changes` using stub responders where
      appropriate.
      - Test: `go test -tags=integration ./processor/mcp-gateway`
- [ ] 4.2 Keep the existing tool-list and guardrail tests, and update them when the
      README tool list changes.
      - Test: `go test ./processor/mcp-gateway`

## 5. Cover HTTP, GraphQL, and NATS surfaces listed in README

- [ ] 5.1 Add happy-path HTTP handler tests for `/source-manifest/status`,
      `/source-manifest/summary`, and `/source-manifest/predicates`.
      - Test: `go test ./processor/source-manifest`
- [ ] 5.2 Add product-owned NATS request/reply tests for `graph.query.sources`,
      `graph.query.status`, `graph.query.predicates`, and
      `graph.query.versionDiff`.
      - Test: `go test -tags=integration ./processor/source-manifest ./internal/governance`
- [ ] 5.3 Add a representative GraphQL route smoke for the ServiceManager route and
      keep the UI-profile `/graphql` Playwright assertion.
      - Test: `task core:smoke` and `task ui:e2e`
- [ ] 5.4 Decide whether every framework-owned graph-query subject listed in README
      should be tested here or whether README should link to SemStreams for the
      exhaustive subject contract.
      - Test: README and coverage matrix agree on ownership

## 6. Gates

- [ ] 6.1 `openspec validate cover-readme-surfaces --strict`
- [ ] 6.2 `go test ./cli ./cmd/semsource ./config ./processor/source-manifest
      ./processor/mcp-gateway ./processor/code-context`
- [ ] 6.3 `go test -tags=integration ./...`
- [ ] 6.4 `go test -tags=e2e -timeout 300s ./test/e2e/`
- [ ] 6.5 `task core:smoke` after the SemStreams P0 fixed tag is adopted
- [ ] 6.6 `task ui:smoke`
