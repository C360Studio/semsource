## 1. Precondition fixes (D1 + D5)

- [x] 1.1 Fix `TestIntegration_DomainScopedRetrieval_OnTheWire` docs-lens expectation to
      `[web, config]` (orphaned by #84's deliberate docScopeDomains change). Proves: the test
      passes and correctly pins the CURRENT contract.
- [x] 1.2 Attach `.WithSignals(fusionvocab.New())` in `supersession_demote_integration_test.go`
      and `multi_source_lineage_integration_test.go` (mirror the staleness test); fix forward
      anything the real salience surfaces. Proves: both tests green WITH signals attached.

## 2. Integration job in CI (D1)

- [x] 2.1 New `test-integration` job in `.github/workflows/ci.yml`: `go test -tags=integration
      ./internal/governance/ ./processor/mcp-gateway/ ./processor/code-context/`,
      timeout-minutes 15, on PR + main; `build-and-push` depends on it. Proves: job green on
      this PR (its own CI run is the proof).

## 3. Smoke answer-content coverage (D2)

- [x] 3.1 `scripts/core-profile-smoke.sh`: content-asserting tools/call for code_context
      (known symbol → body fragment; bogus symbol → honest miss), code_impact (relations/
      impact shape), doc_context (fixture README content), code_search (embedding-gated
      fixture hit), code_changes (honest no-versions note). Proves: extended smoke runs green
      locally end-to-end (record evidence).
      EVIDENCE: ran locally via `SEMSOURCE_SMOKE_PROJECT_NAME=ci-proof-chain-smoke-$$
      ./scripts/core-profile-smoke.sh` (isolated Compose project, ports 28080/24222/28222
      free-checked first). First attempt caught a real bug in the new checks themselves:
      code_context/code_impact ran before graph-index's structural `index.ready` caught up
      (source-manifest `phase:ready` is a DIFFERENT, earlier signal) — fixed by adding a
      shared `wait_for_status_readiness` poll helper and gating structural queries on
      `index.ready` (mirroring the existing embedding.ready gate for code_search). Second
      attempt: full script green end to end — all new content assertions passed
      (code_context body fragment + honest miss, code_impact impact facet, doc_context
      README content, code_search embedding-gated hit, code_changes honest no-versions
      note), followed by the pre-existing removal round-trip, compose-packaging-hardening
      checks, NATS-recreation durability check, and the tier0 (BM25) reboot check — script
      exited 0 with a clean teardown, no diagnostics dump.

## 4. Unit pins (D3 + D4)

- [x] 4.1 `cmd/semsource/run_test.go`: default-component-set test asserts the COMPLETE
      required spawn map (every product-surface component). Proves: removing any required
      entry fails the test.
- [x] 4.2 GRAPH stream subject pin: explicit subjects, zero overlap with `rpcReplySubjects`,
      no swallowing wildcard. Proves: broadening a subject filter fails the test.
- [x] 4.3 Verify honest-readiness (#78) already pins the mid-seed `seeding` window; reference
      the existing test in the spec sync, or add the missing pin if absent.
      VERIFIED ALREADY PINNED — no new test needed:
      `processor/source-manifest/status_seedwindow_test.go::TestStatusAggregator_MidSeedWindowIsSeeding`
      asserts `buildStatus().Phase == PhaseSeeding` when all sources have reported but one is
      still `SourcePhaseIngesting` (the exact mid-seed window); sibling test
      `TestStatusAggregator_ReadyAfterLastSeedCompletes` pins the ready transition happening on
      the LAST source's completion, not the first report.

## 5. Job timeouts (D6)

- [x] 5.1 `timeout-minutes` on every ci.yml job (~3× baseline: test 10, lint 5, ui-quality 10,
      test-integration 15, build-and-push 30, publish-ui 15, ui-release-smoke 15). Proves:
      rendered workflow carries the ceilings.
