# Corrective implementation tasks

Architect inventory and independent component/graph contract reviews are complete.
Implementation and validation remain pending.
Named tests below are planned behavioral gates, not executed evidence or invented existing results.

## 1. Contract

- [x] Inspect actual ingress, pin settlement/storage APIs and private recovery authority; record this design.
- [x] Obtain independent component and graph/event contract sign-off before Go changes.
- [ ] Owner commits the approved proposal/design/specs/tasks and claims the corrective PR before code.

## 2. Additive proof before removal

- [ ] A: `TestLegacyLifecycleCompatibility` proves missing/empty admitted; nonempty, unreadable and timeout
  refused without writes or private-record decoding; original writer must be stopped.
- [ ] A: `TestIntegrationLegacyLifecycleCompatibility` proves real-NATS retained-message cases, including
  terminal-looking/foreign/tombstone/corrupt data and exact unchanged storage on refusal.
- [ ] A: `TestDesiredRemovalIndependentOfGraph` proves disabled config/manifest receipt with graph degraded.
- [ ] A: `TestDesiredPartialCommitReceipt` proves commit-before-memory error and manifest-failure retry,
  retaining committed flags/error and repeated-remove NOT_FOUND, including an expanded repo child.
- [ ] B: retain/run `TestStopSettlesAcceptedBatch` and source owner Stop/cancellation regressions as red/green
  proof around receipt removal; add `TestPublisherIndependentOfRecoveryStorage` if an extra seam is needed.

## 3. Remove unsupported authority and narrow responses

- [ ] A: delete journal/coordinator/tail/effect/projection replay and root binding/decorator setup; install
  only the bounded existing-only guard before provisioning. `TestRootRefusesBeforeProvisioning` proves order.
- [ ] A: local unexported request/run gates preserve cancellation before admission;
  `TestCanceledAdmissionMakesNoWrites` proves no stale desired write or graph mutation.
- [ ] A: remove generation/projection-phase replies; add constant unavailable projection status to successful
  or partial desired-change responses; align MCP descriptions and returned semantics.
  `TestSourceProjectionUnavailable` proves HTTP/NATS/MCP contract and no graph writes.
- [ ] A: keep lifecycle status HTTP 410 and both source-removal RPC refusal paths stateless; remove periodic
  lifecycle status/repair workers and misleading health/progress claims. Prove auth and refusal payloads.
- [ ] A: retain the ConfigManager desired read seam and request-driven manifest repair without a journal.
  Preserve `TestIntegrationRemovedFileSourceStaysDisabledAfterConfigRestart` and desired identity/retry cases.
- [ ] B: remove publisher receipts, seed seals, lifecycle observers, bindings and seed-proof-only handler
  plumbing. Preserve PubAck/loss semantics, immutable queued payloads and checked source shutdown under race.
- [ ] A after B handoff: delete `sourceintent` and unused private-only tests; prove no production imports,
  record types, bucket writes or alternate recovery loop survive. Keep the single read-only legacy guard.
- [ ] A: preserve `TestSourceRemovedStickyAcrossLegacyOracles`, exact-parent passage behavior, truncation
  refusal, native drain and ordinary deletion/recreation cases; adapt governance tests to explicit unavailable
  source-removal projection without deleting ordinary positive assertions.

## 4. Changed-contract qualification and review

- [ ] Owner: add a separate process report for fresh boot, original/runtime source desired removal and re-add,
  checked exits, persistent tombstones, unchanged content/siblings and explicit unavailable projection.
- [ ] Owner: run the unchanged versioned corpus with exact pin/config/provider inputs; preserve original
  expected answers and classify changed-contract/known-at-pin failures explicitly, never turn them into passes.
- [ ] Owner: verify historical fixtures/results remain byte-identical; new contract evidence has separate names.
- [ ] Component reviewer: approve no substitute authority, truthful config errors, guard and owned lifecycle.
- [ ] Graph/event reviewer: approve retained staleness/identity/content and unavailable mutation semantics.
- [ ] Owner: run unit/race, applicable real-NATS integration/e2e/Garage, lint, agents and OpenSpec gates;
  update explicit CI package lists for surviving suites and run workflow contract tests if edited.
- [ ] Technical writer: document compatibility refusal/fresh-store boundary, retired public fields,
  unavailable projection, #215 deferral and separate #18/#19/#20 decisions without rewriting old evidence.
- [ ] Owner: record final source/binary identities, changed contracts, gate results and independent sign-off.
