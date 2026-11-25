# A full GRAPH stream at boot no longer crash-loops SemSource — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` and `design.md` (2026-11-25, sha256:8a88c174f38f) with the recommended
  answers: D1 (a) non-fatal with background retry; D2 retry until delivered or Stop; D3 `ready` does not wait for
  the broadcasts, no new status field; ADR-0003's stale republish claim goes to a follow-up issue. Approved-by line
  entered by claude at the owner's instruction.

## 2. Repro (before the change; must fail on main)

- [x] 2.1 `TestE2E_BootIntoFullGraphStream` in `test/e2e/graph_retention_test.go`, reusing `writeRetentionFixture`,
  `writeRetentionConfig`, `graphStreamState` and `assertCleanReady` (`:28-108`), `waitForDrained`
  (`test/e2e/e2e_test.go:1639`) and `upgradeStartWriter` (`test/e2e/upgrade_path_test.go:255`). Boot 1 reaches ready while a core
  subscription on `graph.ingest.entity` captures the published envelopes. After Stop, the test republishes the
  captured envelopes with no Msg-Id until GRAPH answers 10077, then boots again. It asserts four things: the process
  is still running 30 s after boot; status reaches `ready` with no loss (`assertCleanReady`); GRAPH holds no
  `graph.ingest.entity` message; and the `graph.ingest.manifest` subject count is higher than before boot 2.
  Premise: re-applying identical entity content leaves the ENTITY_STATES count unchanged, as
  `TestE2E_WarmRestartWithinStreamWindow` (`:116-170`) already shows. Proved by: on main it fails, with the process
  exiting and its log carrying `publish manifest` and 10077 (record the line in the PR).

## 3. source-manifest start (additive)

- [x] 3.1 `startManifest` and `startPredicateSchema` (`processor/source-manifest/component.go:179-262`) register their
  responders and set `predicatesData` before the first publish attempt. A refused publish no longer returns from
  `Start`; it hands the broadcast to a worker under `c.workers`, and the worker's cancel joins `cancelFuncs` before
  any later return in `Start`. The worker retries with a capped backoff (500 ms to 10 s) until it delivers or is
  cancelled, re-marshalling the current manifest each attempt (`:203-219`). Proved by: a unit test using a publish
  seam alongside `readinessRequest`. With the seam refusing, `Start` returns nil and `graph.query.sources`,
  `graph.query.predicates` and `GET /source-manifest/sources` answer. After the seam accepts, exactly one delivery
  is recorded per broadcast. Run with `-race`.
- [x] 3.2 The refusal uses a `degraded.Condition` (`internal/degraded/condition.go:29-52`): `Enter` on the first
  refusal, `Clear` on delivery. Proved by: a unit test that counts log records over three refused attempts (one
  warning) and the delivery (one recovery line).
- [x] 3.3 Stop ends a worker that is still retrying, within Stop's context (`component.go:599-634`). Proved by: a unit
  test whose seam refuses forever; `Stop` returns nil and `workers.Wait` joins, with no sleep in the test.
- [x] 3.4 The aggregate phase does not depend on delivery. Proved by: a unit test where all sources report seeded
  while the seam still refuses; status `phase` is `ready`.
- [x] 3.5 2.1 passes on this branch.

## 4. Spec and docs

- [x] 4.1 Every new test cites `ingestion-readiness / A full ingest transport stream SHALL NOT stop SemSource from
  starting` (`ateam test-spec`).
- [x] 4.2 The comment on `Start` (`component.go:140-141`) and the type comment (`:44-48`) say the broadcasts are
  best-effort and retried. The ADR-0003 follow-up follows Open Question 4.
- [x] 4.3 `task spec:check` green. Archive the change before the PR is ready.
  ADR-0003 follow-up filed as #405. `TestE2E_BootIntoFullGraphStream` added to ci.yml e2e-local.
