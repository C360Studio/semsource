# Implementation and proof tasks

## 1. Contract and baseline

- [x] Inspect issue #215, current code, pinned ADR-094/095/102/104 and storage/query/mutation APIs.
- [x] Apply `kv-or-stream` and `orchestration-check`; record operational ownership and repair obligations.
- [x] Reproduce unchanged missing-marker failure: additive pre-fix run passes 9/10, no automatic request.
- [x] Architect approves this design for implementation; independent implementation approval remains open.
- [x] Independent Go and graph/event reviewers approve the final contract for implementation only.
- [x] Journal developer creates `internal/sourceintent/contract.go` from the agreed design; architect
  and both developer owners confirm it before parallel edits outside that file.

## 2. Intent and desired-state protocol

- [ ] Add no-eviction product journal, typed schema/key validation, CAS generations, exact read status.
  Prove `TestRemovalJournalRetentionAndCAS`, corrupt/foreign-generation rejection, and startup
  refusal of memory storage, wrong replica policy or positive global MaxMsgs despite successful Ensure.
- [ ] Write failing removal write-order/crash tests, then implement intent-before-disable-before-ack.
  Prove `TestRemovalIntentBeforeDesiredAck` and every prepared/config/manifest/pending crash boundary.
- [ ] Repair partial desired writes without disabling a new enabled config or converting unknown handles
  to successful removal. Prove `TestRemovalPartialWritesRepair` and `TestRemovalUnknownHandle`.
- [ ] Serialize Add/Remove/replay; prove enabled-config-first re-add partial failures, generation CAS,
  and stale completion rejection in `TestRemovalReaddGenerationFence` under race detection.
- [ ] Prove cancellation-aware gate acquisition and coordinator→supersession lock order; receipt
  writes must continue through producer drain after the replay worker stops.

## 3. Retirement, scope, and projection

- [x] Record upstream applied/unresolved-input proof gap in SemStreams #1444. Do not decode private
  GRAPH_INGEST_APPLIED_SEQ values or treat bounded advisory history as a parked-work census.
- [ ] Add explicit TailProof and applied_tail_unproven status. Prove zero current backlog admits only
  current retained projection and never terminal removal completion, including an empty retained set.

- [ ] Add immutable boot admission and current graph-consumer settlement gates. Prove old boot stays
  pending, missing/failed/backlogged consumer stays pending, changed stream generation fails closed,
  and cached readiness is insufficient in `TestRemovalRetirementAndTailBarrier`. Capture stream
  and consumer identity BEFORE removal acknowledgment and test reset between boots; reject observed
  AckNone/AckAll, wrong filter/delivery policy, and recreated consumer despite zero backlog.
- [ ] Derive exact producer scope and prove repo child isolation, Project overrides, authority isolation,
  and same-taxonomy overlap refusal in `TestRemovalExactSourceScope`.
- [ ] Write failing multi-page/truncation/mutation error tests before fixes. Prove full opaque cursor
  traversal, cyclic cursor rejection, partial error propagation and retry in `TestRemovalReplayPages`.
- [ ] Converge exact source_removed markers on parents/passages, preserving retained history and source
  facts; prove duplicates and lost mutation replies converge in `TestRemovalReplayIdempotent`.
- [ ] Implement startup plus periodic failed-work repair, honest status and bounded Stop/join.
  Prove recovery without a new request and cancellation retaining pending work under `-race`. A
  successful frozen-pin retained pass stays pending applied_tail_unproven and repeats after late arrivals.

## 4. Current publication receipts and reactivation

- [x] Architect records frozen public conditional-reconcile gap in
  [SemStreams #1445](https://github.com/C360Studio/semstreams/issues/1445); no copied client or raw subject shim.
- [ ] Return conditional_reconcile_unavailable and issue zero clear mutations when current publication
  evidence matches a retained source_removed entity. Prove a source write between fingerprint read
  and the public client's internal reread cannot be hidden by shared lifecycle serialization.
- [ ] Inventory exact current-epoch sealed batches with ListSeeds; validate each seal, digest and
  receipt set, and refuse partial/corrupt/foreign records. A successful initial prerequisite from the
  same epoch is mandatory before considering a separately sealed live target.
- [ ] Rebind every matching enabled Reactivate history on boot, including Complete records, with a new
  epoch and Pending initial proof; keep periodic inventory active after completed batches.
- [ ] Validate optional InitialManifest/InitialReceipts independently for live target requests. Prove
  failed initial plus successful live B remains pending; do not replace this proof with a boolean.

- [ ] Bind publisher observer to exact handle/generation and NEW per-boot seed epoch before Start;
  test no global mutable registry and that prior-epoch receipts cannot grant freshness. Test root
  rejects missing, duplicate, mismatched or altered factory/config/instance bindings.
- [ ] Persist receipts only after PubAck; receipt failures remain retryable, visible and drain-owned.
  Prove `TestPublisherReceiptAfterAck`, receipt-only retry after PubAck (no republish), immutable
  evidence, failure recovery, shutdown deadline, and crash/reseed repair.
- [ ] Seal an exact current-ID manifest only after successful enumeration, all expected publications
  and receipts agree; prove failure/cancellation cannot seal a partial seed, explicit empty seed
  completion, in-flight drain batch accounting, and watch/reseed separation.
- [ ] Intended acceptance, blocked by #1445: compare authoritative source facts and clear only
  source_removed through an admitted mutation fenced to the SAME observed revision. Preserve newer
  stale reasons; revision conflicts require a new read and recomputation, not a hidden retry.
- [ ] Prove legacy filesystem/empty-Absent/path-count sweeps cannot clear or replace source_removed,
  while ordinary file_deleted/path_missing/passage_removed behavior remains intact.
- [ ] Intended acceptance, blocked by #1445: same-ID current parents/passages become fresh while
  removed files/symbols/tail passages remain stale in `TestReactivationCurrentEntityReceipts`.
  Keep positive tests/results visible as failed or blocked, never reinterpret refusal as freshness.
- [ ] Prove crash after A+B receipts, offline deletion of B, and reseed A-only does not clear B; prove
  partial-removal/re-add and remove/add/remove/add preserve needed selective reactivation.
- [ ] Intended live-B freshness acceptance remains blocked: specify and prove publication/withdrawal
  eligibility, including later B recreation and an old B receipt followed by deletion or failed
  enumeration. A sticky source_removed revision or historical matching receipt is insufficient.

## 5. Real process acceptance

- [ ] Add an additive private-broker process suite for original-file and runtime-added sources:
  receipt and durable intent, old producer still live, checked process exit, producer absent on new
  boot, exact retained parent/passage markers, sibling entity/content unchanged.
- [ ] Kill after durable intent and before projection; restart the retained broker/application state
  and prove retained-marker convergence without an operator resend, while asserting pending
  applied_tail_unproven rather than fabricating terminal completion. Use explicit state barriers, never fixed sleeps.
- [ ] Prove re-add before first restart supersedes old work with zero old-generation mutation. Keep
  removal/re-add/new-boot positive clear assertions as intended acceptance, blocked by #1445; append
  explicit pending/no-clear evidence without changing them into passing freshness assertions.
- [ ] Prove multi-page exact expected-ID set, deterministic query/mutation failure recovery and
  duplicate/redelivery with channel-controlled integration seams, plus process restart recovery.
- [ ] Keep original probes/ledgers unchanged; add exact commit, binary SHA, module pin, configuration,
  workload hash, checked process exits and named limitations to new result artifacts.

## 6. Review and delivery

- [ ] Independent Go reviewer approves amended retained-pass/pending-tail boundary plus context,
  serialization, error and stop ownership.
- [ ] Independent graph/event reviewer approves amended retained-pass/pending-tail boundary, exact
  scope, retention, replay and re-add freshness.
- [ ] Pass gofmt, pinned revive/vet, unit/race, complete required integration and process suites,
  `task agents:check`, and strict OpenSpec validation; record legitimate blockers without skipped gates.
- [ ] Update PR #213 / issue #215 with concrete behavior, proof, exact pins and remaining blockers;
  keep SemStreams substrate and SemEngine qualification separate.

## 7. Explicit completion gates

- [ ] A supported stream-incarnation-aware applied/unresolved-input contract proves the old producer's
  complete accepted input set, including MaxDeliver/terminal-rejection outcomes and retention gaps.
  This cannot pass with the current frozen pin; do not close #215 or mark migration fully qualified.

- [ ] A supported public contract-bound conditional reconcile accepts the caller's exact revision,
  preserves classified conflict/unknown-commit outcomes, and performs no hidden reread or retry
  (SemStreams #1445). Then rerun the positive freshness workload and independent graph review.
- [ ] Continuing publication/withdrawal eligibility is specified and proven before live batches can
  grant freshness. Current ListSeeds inventory and pending repair do not satisfy this gate.

## 8. Independent review follow-up: generation and commit uncertainty

- [ ] Replace product mutation RPC dispatch with a synchronous single-bound projector; permanently
  refuse the old endpoint. Prove delayed old requests cannot mutate after re-add.
- [ ] Persist an exact binding/entity/attempt fence before every possible graph mutation; preserve
  public commit outcomes and reject generic internal outcomes as proof of non-commit.
- [ ] Block source generation changes before config writes while any effect is unresolved, including
  refresh/re-add and unsafe enabled boot admission. Prove caller-loss and crash/restart boundaries.
- [ ] Persist terminal evidence only for the exact current attempt after verified or safely rejected
  outcome. Prove wrong attempt, CAS error and unknown commit retain unresolved work; include a
  resolution commit-then-error test. Only authoritative journal terminal evidence, never graph
  readback or elapsed time, can release admission after a lost resolution-write reply.
- [x] Track upstream terminal outcome resolution and backend commit ambiguity in SemStreams #1446.
- [ ] Obtain independent implementation approval for the amended synchronous/fence boundary.
