# Tasks: Version change detection (what changed between X and Y)

## 1. Diff computation (reuse correspondence.go)

- [x] 1.1 In `processor/supersession/`, add `versiondiff.go`: a pure function that
      takes the enumerated `[]candidate` (from `candidateFromEntity`) plus `from`
      and `to` version strings and returns a structured changeset. Filter to
      candidates whose `version` is `from` or `to`; group both sides by `corrKey`
      (reuse `groupByCorrespondence` or key directly); classify per D3
      (added/removed/changed/unchanged/indeterminate) reusing `classifyChange`'s
      hash+kind comparison. Pure + table-testable (no NATS).
- [x] 1.2 Define the request/response types (`VersionDiffRequest{Project, From, To,
      WantBodies *bool, Budget}`, `VersionDiffResponse{Project, From, To, Ready,
      Counts, Changes[], Truncated}`, `Change{Name, Path, Type, Package, Status,
      FromID, ToID, FromBody, ToBody}`) with JSON tags.

## 2. Query surface (NATS + HTTP)

- [x] 2.1 In the supersession component, subscribe `graph.query.versionDiff`
      (request/reply): decode the request, enumerate versioned candidates for the
      project via `QueryPrefixAll` (surface `truncated`), run the 1.1 diff, hydrate
      bodies (task 3), and return the response JSON. Gate on index readiness →
      honest envelope (D7); zero entities for a version → note, not false-empty (D6).
- [x] 2.2 Add `RegisterHTTPHandlers` (or extend it) with `POST /supersession/versionDiff`
      — bounded body, same request/response, 503 until started (mirror
      code-context's HTTP handler).
- [x] 2.3 N/A — no `rpcReplySubjects` guard exists; verified the GRAPH stream binds only
      `graph.ingest.*` (run.go `graphStreamConfig`), so `graph.query.versionDiff` (a core
      req/reply on the supersession component, like `graph.supersession.run`) has no stream overlap.

## 3. Verbatim before/after bodies (budget-capped)

- [x] 3.1 Add a `fusion.BodyResolver` to the supersession component (prefer
      `deps.StoreRegistry`, else attach the CONTENT objectstore bucket — mirror
      `code-context/component.go bodyResolver`). Hydrate `from_body`/`to_body` from
      each candidate's `code.body.store` + `code.body.key` handle.
- [x] 3.2 Enforce the budget (`max_symbols`, `max_body_bytes`, sane defaults): stop
      hydrating past the cap, set `truncated: true`, and count dropped symbols.
      `want_bodies=false` skips hydration entirely.

## 4. MCP tool

- [x] 4.1 In `processor/mcp-gateway/`, add a `code_changes` tool: `ChangesInput{
      Project, From, To string}`, handler forwards `{project, from, to}` to
      `graph.query.versionDiff` and returns the JSON verbatim (model on
      `codeContext`/`fusionQuery` in `query_tools.go`). Description: "What changed
      between two versions of a source — added/removed/changed symbols with before/
      after bodies." Register in `buildServer()`.

## 5. Tests

- [x] 5.1 Unit (`processor/supersession/versiondiff_test.go`): table-driven over the
      pure 1.1 function — added (in `to` only), removed (in `from` only), changed
      (both, differing same-kind hash), unchanged (equal hash), indeterminate
      (missing hash / kind mismatch); and correct counts. No NATS.
- [x] 5.2 Integration (`//go:build integration`,
      `internal/governance/version_diff_integration_test.go`): over the real graph
      stack, ingest a symbol at two versions (same project, distinct `system`/version)
      — one unchanged, one with a differing body — plus one added-in-`to` and one
      removed-in-`to`; call `graph.query.versionDiff` via NATS and assert the
      statuses, counts, and that a changed symbol carries both `from_body` and
      `to_body`. Prove the honest path: an unknown `to` version returns a noted
      empty, not a false diff.

## 6. Docs

- [x] 6.1 `docs/adr/0008-...md`: mark row 69 ("what changed v1.9 → v1.10") as
      realized by this change; note rename tracking (row 71) and commit-changeset
      (#4/row 70) remain deferred.
- [x] 6.2 README + `docs/integration/mcp-quickstart.md`: add `code_changes` to the
      tool list / cheat-sheet; add `graph.query.versionDiff` to the NATS subject
      table and the HTTP endpoint table.

## 7. Gates

- [x] 7.1 `go build ./...`, `go test ./...` green; `go test -tags=integration
      ./internal/governance/ ./processor/supersession/` green.
- [x] 7.2 `task lint` zero warnings (revive v1.15.0), gofmt, go vet.
- [x] 7.3 `openspec validate version-change-detection` green; `/opsx:verify` before
      archive.
