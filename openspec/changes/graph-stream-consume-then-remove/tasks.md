# GRAPH removes consumed messages — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` and `design.md` (2026-11-25, sha256:aa3ca23c50c9) with the recommended
  answers: D1 workqueue; D2 memory storage stays, file storage is a follow-up; D4 bounds unchanged; PR #394 says
  `Addresses #160` and the named durability posture is a follow-up issue. Approved-by line entered by claude at the owner's instruction.

## 2. Repro and test harness (before the change; green on main)

- [x] 2.1 The two e2e tests that pre-create GRAPH and fetch from an unfiltered consumer
  (`test/e2e/e2e_test.go:677-700,760` and `:1064-1080,1162`) stop doing so and read ENTITY_STATES instead. This must
  land before 3.1, because workqueue refuses that consumer (10099/10100) and the pre-created limits stream trips the
  drift refusal. Proved by: `TestE2E_RunStartsAndPublishesEntities` and `TestE2E_OSH_JavaMavenIngest` green on main.
- [x] 2.2 Measure the GRAPH bytes one seed of the e2e fixture publishes (stream info after ready, on main).
- [x] 2.3 `TestE2E_WarmRestartWithinStreamWindow`, using the boot/stop pattern of `test/e2e/discovery_restart_test.go:135-170`
  on one NATS. It sets `streams.GRAPH.max_bytes` to about 1.5× the bytes from 2.2, boots to ready, stops, and boots
  again at once. It asserts: phase `ready`, no failed or dropped counts in source status, and the entity count equal
  across both boots. It must fail on main (10077 / `degraded`).
- [x] 2.4 setup03a `pendingBrokerRestart` and `captureRestartGuard` (`test/setup03a/qualification_test.go`) stop reading
  GRAPH by sequence (found after 3.1: workqueue removes the applied Run envelope the probe replays). The harness holds a
  core subscription on `graph.ingest.entity` from each connect and replays the latest captured envelope whose payload is
  the Run entity; the restart-guard evidence records the captured envelopes and states that the stream sequences are not
  recoverable. Proved by: `TestKnownAnswerCorpus` green on bm25 and foundation with the workqueue binary and with main's.
- [x] 2.5 `TestE2E_SubmoduleIngest` (`test/e2e/submodule_test.go:84-98`) stops pre-creating GRAPH and reads
  ENTITY_STATES, as in 2.1. Proved by: the test green.

## 3. Retention (breaking: needs a fresh GRAPH stream)

- [x] 3.1 `graphStreamConfig` declares `Retention: "workqueue"` (D1; the value follows the ruling)
  (`cmd/semsource/run.go:966-995`). The comment keeps the refuse-don't-evict rationale and corrects "terminal" to
  "retried within the delivery budget, then failed" (`internal/entitypub/classify.go:60-65`). The fixtures at
  `internal/sourcespawn/sourcespawn_test.go:41-53` and `internal/governance/source_removal_integration_test.go:47-59`
  move in step. Proved by: a unit test pinning retention, discard `new`, the age bound and the byte bound, next to
  `TestGraphStreamConfig_SubjectsExplicit_NoRPCOverlap` (`cmd/semsource/run_test.go:237`); 2.3 green.
- [x] 3.2 After 2.3's first boot reaches ready, the GRAPH stream holds no `graph.ingest.entity` message (per-subject
  stream info). Proved by: an assertion added to 2.3.
  Note: the test also requires the second boot to advance GRAPH by at least one seed, and logs the non-entity bytes left after the first seed (7,790 B, design Open Question 5).
- [x] 3.3 A message published before graph-ingest binds is applied after the bind. Proved by: an integration test over
  `internal/natstest` that publishes to a fresh GRAPH, then starts graph-ingest, then reads the entity.
- [x] 3.4 A config carrying `streams.GRAPH.retention` fails to load and names the field (`config/loader.go:29`). Proved
  by: a loader test.
- [x] 3.5 A pre-existing limits-retention GRAPH stream refuses boot with the drift remedy
  (semengine `config/stream_drift.go:35-39`). Proved by: an e2e assertion on the process exit and its log line.

## 4. Docs and spec

- [x] 4.1 `docs/integration/m5-consumer-integration.md:43-46`: Option A no longer says to subscribe on GRAPH; it points
  to Options B and C.
- [ ] 4.2 Adoption-note line under #386 (design, Migration Plan).
- [ ] 4.3 `openspec validate --all` and `task spec:check` green. Archive the change before the PR is ready.

## 5. Gate

- [ ] 5.1 The OSH warm-restart proof (#388) passes on this branch. That is beta.11's gate, recorded on #388.
