# GRAPH removes consumed messages — tasks

## 1. Approval

- [ ] 1.1 Owner approves `proposal.md` and `design.md` and rules on Open Questions 1–4.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Repro and test harness (before the change; green on main)

- [ ] 2.1 The two e2e tests that pre-create GRAPH and fetch from an unfiltered consumer
  (`test/e2e/e2e_test.go:677-700,760` and `:1064-1080,1162`) stop doing so and read ENTITY_STATES instead. This must
  land before 3.1, because workqueue refuses that consumer (10099/10100) and the pre-created limits stream trips the
  drift refusal. Proved by: `TestE2E_RunStartsAndPublishesEntities` and `TestE2E_OSH_JavaMavenIngest` green on main.
- [ ] 2.2 Measure the GRAPH bytes one seed of the e2e fixture publishes (stream info after ready, on main).
- [ ] 2.3 `TestE2E_WarmRestartWithinStreamWindow`, using the boot/stop pattern of `test/e2e/discovery_restart_test.go:135-170`
  on one NATS. It sets `streams.GRAPH.max_bytes` to about 1.5× the bytes from 2.2, boots to ready, stops, and boots
  again at once. It asserts: phase `ready`, no failed or dropped counts in source status, and the entity count equal
  across both boots. It must fail on main (10077 / `degraded`).

## 3. Retention (breaking: needs a fresh GRAPH stream)

- [ ] 3.1 `graphStreamConfig` declares `Retention: "workqueue"` (D1; the value follows the ruling)
  (`cmd/semsource/run.go:966-995`). The comment keeps the refuse-don't-evict rationale and corrects "terminal" to
  "retried within the delivery budget, then failed" (`internal/entitypub/classify.go:60-65`). The fixtures at
  `internal/sourcespawn/sourcespawn_test.go:41-53` and `internal/governance/source_removal_integration_test.go:47-59`
  move in step. Proved by: a unit test pinning retention, discard `new`, the age bound and the byte bound, next to
  `TestGraphStreamConfig_SubjectsExplicit_NoRPCOverlap` (`cmd/semsource/run_test.go:237`); 2.3 green.
- [ ] 3.2 After 2.3's first boot reaches ready, the GRAPH stream holds no `graph.ingest.entity` message (per-subject
  stream info). Proved by: an assertion added to 2.3.
- [ ] 3.3 A message published before graph-ingest binds is applied after the bind. Proved by: an integration test over
  `internal/natstest` that publishes to a fresh GRAPH, then starts graph-ingest, then reads the entity.
- [ ] 3.4 A config carrying `streams.GRAPH.retention` fails to load and names the field (`config/loader.go:29`). Proved
  by: a loader test.
- [ ] 3.5 A pre-existing limits-retention GRAPH stream refuses boot with the drift remedy
  (semengine `config/stream_drift.go:35-39`). Proved by: an e2e assertion on the process exit and its log line.

## 4. Docs and spec

- [ ] 4.1 `docs/integration/m5-consumer-integration.md:43-46`: Option A no longer says to subscribe on GRAPH; it points
  to Options B and C.
- [ ] 4.2 Adoption-note line under #386 (design, Migration Plan).
- [ ] 4.3 `openspec validate --all` and `task spec:check` green. Archive the change before the PR is ready.

## 5. Gate

- [ ] 5.1 The OSH warm-restart proof (#388) passes on this branch. That is beta.11's gate, recorded on #388.
