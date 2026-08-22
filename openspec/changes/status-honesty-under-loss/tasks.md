# Tasks: status honesty under delivery loss

Ordering follows the repo rule — additive and backward-compatible steps first,
the breaking removal isolated in its own group. Groups 1–3 leave `publish_total`
in place and every existing consumer working; group 4 is the only breaking step.

Note on atomicity: `internal/sourcestatus.Report` is decoded strictly by the
aggregator (`entity-publish-integrity`, *The internal status report is a single
shared contract*), so a producer emitting a field the shared type does not declare
is rejected with an error log. Task 1.1 must land in the same commit as 1.2 — a
partial rollout fails loudly by design.

## 1. Surface the publisher's figures (additive)

- [x] 1.1 Add `offered_total`, `delivered_total`, and `lost_total` to
  `internal/sourcestatus.Report`, leaving `publish_total` in place for now.
  Verify: extend `internal/sourcestatus/report_test.go` (currently sets
  `PublishTotal: 99` at :20) so a round-trip carries all four fields.
- [x] 1.2 Populate the three new fields in all eight source components, in the
  same commit as 1.1 — `ast`, `doc`, `git`, `cfgfile`, `url`, `image`, `audio`,
  `video`. `delivered_total` from `Publisher.Published()`, `lost_total` from
  `Publisher.Lost()`, `offered_total` from the existing source-local counter
  (`entitiesIndexed` in ast-source, `entitiesPublished` in the other seven).
  Verify: `go test ./processor/...` plus a per-source assertion that
  `offered_total >= delivered_total` on a report built after a seed.
- [x] 1.3 Thread the three fields through the aggregator's report mapping in
  `processor/source-manifest/status.go` and the status payload in
  `payload_status.go`. Verify: `TestStatusAggregator_PhaseTransitions`
  (`component_test.go:622`) still passes and the new fields appear on the
  aggregate payload.
- [x] 1.4 Assert the reconciliation identity `offered = delivered + lost +
  in-flight` holds for a source driven through induced loss. Verify: the
  per-source `TestBuildStatusReport_DeliveryFiguresReconcile` tests, driven by
  `internal/statustest.LossyPublisher`, which settles delivery before asserting
  so the identity holds as equality rather than as a bound.

## 2. Per-pass loss baseline (additive)

- [x] 2.1 Record the publisher's loss count at the start of each source's initial
  seed and expose whether the pass completed clean (`Lost()` unchanged across it).
  The natural hook is each component's existing phase-transition path —
  `publishStatusReport`/`setPhase` in ast-source and its siblings. Verify: unit
  test per source shape asserting clean-pass true with no loss, false with
  induced loss.
- [x] 2.2 Carry the clean-pass outcome on the shared report so the aggregator can
  read it without recomputing from monotonic counters (design D2). Verify:
  `internal/sourcestatus/report_test.go` round-trip covers the new signal.

## 3. Readiness degrades on loss (behavioural)

- [x] 3.1 Widen the aggregator so a source whose seed completed with loss degrades
  the aggregate. Trigger on the loss signal from group 2, **not** on `ErrorCount`
  — it folds in `parseFailures` and would degrade permanently on one unparseable
  file (design D1). Verify: new test asserting `phase == degraded` for a source
  reporting `lost_total > 0` with `Phase != errored`.
- [x] 3.2 Assert a non-zero `error_count` from parse failures alone does **not**
  degrade the aggregate. Verify: new test with `ErrorCount > 0`, `lost_total == 0`
  → `phase == ready`. This is the regression guard for 3.1's whole rationale.
- [x] 3.3 Make loss-induced degradation sticky via the aggregator's existing
  `degraded` flag, cleared only by a subsequent clean pass (design D3 — watch
  activity does not clear it). Verify: extend
  `TestStatusAggregator_TimeoutDegradedClearsOnCleanSeed`
  (`status_seedwindow_test.go:71`) with a loss-induced sibling case, asserting it
  does *not* clear on continued watch reports.
- [x] 3.4 Confirm `TestStatusAggregator_ReadyAfterLastSeedCompletes`
  (`status_seedwindow_test.go:36`) still passes for a clean seed — the no-loss
  path is unchanged. Verify: existing test, unmodified.

## 4. Retire `publish_total` (BREAKING)

- [x] 4.1 Remove `PublishTotal` from `internal/sourcestatus.Report`,
  `processor/source-manifest/payload_status.go`, `status.go`, and all eight source
  components. Verify: `grep -rn 'publish_total\|PublishTotal' --include='*.go' .`
  returns nothing outside archived changes.
- [x] 4.2 Update the fixtures that pin the old field —
  `internal/sourcestatus/report_test.go:20` and
  `processor/source-manifest/contract_test.go:62`. Verify: `go test ./internal/...
  ./processor/...` green.
- [x] 4.3 Update the HTTP, MCP, and workbench status surfaces and their tests.
  Verify: `TestWorkbenchCapabilities_ReadyHeadlessContract` and
  `TestWorkbenchCapabilities_SourceErrorsOverrideAggregateReady`
  (`workbench_capabilities_test.go:16,202`) pass against the new field set.

## 5. Verification

- [ ] 5.1 Run the full gate: `task lint` (revive pinned v1.15.0, warnings fail)
  and `task test`, then `go test -race ./...` for the aggregator's concurrent
  report handling.
- [ ] 5.2 Add an e2e assertion that a seed with induced delivery loss reports
  `degraded` and a non-zero `lost_total` on the HTTP status surface, and that a
  clean seed reports `ready` with `lost_total == 0`. Verify: `go test -tags=e2e
  ./test/e2e/`.
- [ ] 5.3 Reproduce #177's evidence shape: assert that a run delivering 42,931 of
  77,802 accepted entities reports `delivered_total: 42931`, `lost_total: 34871`,
  and `phase: degraded` — the exact case that previously reported
  `publish_total: 77802` and `phase: ready`. Verify: table-driven test in
  `processor/source-manifest/`.
- [ ] 5.4 Run `openspec validate status-honesty-under-loss --strict` and confirm
  the implementation satisfies each scenario in both spec deltas.
