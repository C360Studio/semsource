# Corrective implementation tasks

Implementation is committed at `5334716b588f6960ed54c8cafdc7033b01088e8e`; scoped independent production
and process-test reviews are approved. Final desired-state process cases pass 33/33 and 37/37;
the unchanged removal probe remains 9/10 with its intentional missing-marker failure.
Live PR #223 checks govern hosted readiness; green remains required before ready/merge.
Completed checks are recorded in `docs/testing/private-recovery-retirement/README.md`; historical
positive source-removal expectations remain unchanged and are not waived by this corrective contract.

## 1. Contract

- [x] Inspect actual ingress, pin settlement/storage APIs and private recovery authority; record this design.
- [x] Obtain independent component and graph/event contract sign-off before Go changes.
- [x] Owner commits the approved proposal/design/specs/tasks and claims the corrective PR before code.

## 2. Additive proof before removal

- [x] A: `TestLegacyLifecycleCompatibility` proves missing/empty admitted and nonempty/unreadable refused;
  cancellation is honored before admission. Review confirms bounded I/O, no writes/private-record decoding,
  and the prerequisite that the original writer is stopped.
- [x] A: `TestIntegrationLegacyLifecycleCompatibility` proves real-NATS retained-message cases, including
  terminal-looking/foreign/tombstone/corrupt data and exact unchanged storage on refusal.
- [x] A: `TestDesiredRemovalIndependentOfGraph` proves disabled config/manifest receipt with graph degraded.
- [x] A: `TestDesiredPartialCommitReceipt` proves commit-before-memory error and manifest-failure retry,
  retaining committed flags/error and repeated-remove NOT_FOUND, including an expanded repo child.
- [x] B: retain/run `TestStopSettlesAcceptedBatch` and source Stop/cancellation regressions through receipt removal.
  Failing-first queued-payload ownership/rejection tests pass; no extra recovery-storage seam is needed.

## 3. Remove unsupported authority and narrow responses

- [x] A: delete journal/coordinator/tail/effect/projection replay and root binding/decorator setup; install
  only the bounded existing-only guard before provisioning. `TestRootRefusesBeforeProvisioning` proves order.
- [x] A: local unexported request/run gates preserve cancellation before admission;
  `TestCanceledAdmissionMakesNoWrites` proves no stale desired write or graph mutation.
- [x] A: remove generation/projection-phase replies; add constant unavailable projection status to successful
  or partial desired-change responses; align MCP descriptions and returned semantics.
  HTTP/refusal tests plus MCP description and partial-receipt tests prove the separate transport contracts.
- [x] A: keep lifecycle status HTTP 410 and both source-removal RPC refusal paths stateless; remove periodic
  lifecycle status/repair workers and misleading health/progress claims. Prove auth and refusal payloads.
- [x] A: retain the ConfigManager desired read seam and request-driven manifest repair without a journal.
  Preserve `TestIntegrationRemovedFileSourceStaysDisabledAfterConfigRestart` and desired identity/retry cases.
- [x] B: remove publisher receipts, seed seals, lifecycle observers, bindings and seed-proof-only handler
  plumbing. Preserve PubAck/loss semantics, immutable queued payloads and checked source shutdown under race.
- [x] A after B handoff: delete `sourceintent` and unused private-only tests; prove no production imports,
  record types, bucket writes or alternate recovery loop survive. Keep the single read-only legacy guard.
- [x] A: preserve `TestSourceRemovedStickyAcrossLegacyOracles`, exact-parent passage behavior, truncation
  refusal, native drain and ordinary deletion/recreation cases; adapt governance tests to explicit unavailable
  source-removal projection without deleting ordinary positive assertions.

## 4. Changed-contract qualification and review

- [x] Owner: add a separate process report for fresh boot, original/runtime source desired removal and re-add,
  checked exits, persistent tombstones, unchanged content/siblings and explicit unavailable projection.
  Final binary: original 33/33 and runtime-added 37/37, command exit 0.
- [x] Owner: run the unchanged versioned BM25 corpus with exact pin/config and no providers: 61/63, exit 1.
  Both broker-restart failures remain known-at-pin assertions; no neural/semembed requalification is claimed.
- [x] Owner: run the unchanged original source-removal probe: 9/10, exit 1, sole missing-marker failure.
  The changed-contract limitation remains failed; the positive expectation is not waived or rewritten.
- [x] Owner: verify historical fixtures/results remain byte-identical; new contract evidence has separate names.
- [x] Component reviewer: approve no substitute authority, truthful config errors, guard and owned lifecycle.
- [x] Graph/event reviewer: approve retained staleness/identity/content and unavailable mutation semantics.
- [x] Owner: run unit/race, applicable real-NATS integration/e2e/Garage, lint, agents and OpenSpec gates;
  update explicit CI package lists for surviving suites and run workflow contract tests if edited.
- [x] Technical writer: document compatibility refusal/fresh-store boundary, retired public fields,
  unavailable projection, #215 deferral and separate #18/#19/#20 decisions without rewriting old evidence.
- [x] Owner: record committed source and clean-build binary identities; retain independent scoped sign-off.
- [x] Owner: record final desired-state, unchanged BM25 and original removal-probe results separately,
  retaining earlier negative/intermediate failures and exact source/binary attribution.
- [x] Owner: measure all six new production files: 99/114 statements (86.8%), satisfying the ≥80% gate.
  Include independently approved real-NATS readback tests and a separate instrumented original-source
  process rerun (33/33); preserve the primary binary/results and record unit/integration-only 79.8%.
- [x] Owner: publish hosted-check tracking and preserve the cold-image setup failure/cleanup limitation.
  The independently reviewed same-digest pull preflight does not change production or frozen tests.
  Live PR #223 checks and its delivery summary own the final result; hosted green remains an external
  required gate before ready/merge. This completed tracking task does not claim a passing rerun,
  positive source-removal/freshness acceptance or merge approval.
