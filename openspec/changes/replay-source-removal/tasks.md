# Implementation and proof tasks

## 1. Contract and baseline

- [x] Inspect issue #215, current code, pinned ADR-094/095/102/104 and storage/query/mutation APIs.
- [x] Apply `kv-or-stream` and `orchestration-check`; record operational ownership and repair obligations.
- [x] Reproduce unchanged missing-marker failure: additive pre-fix run passes 9/10, no automatic request.
- [x] Architect approves this design for implementation; independent implementation approval remains open.
- [x] Independent Go and graph/event reviewers approve the final contract for implementation only.
- [ ] Journal developer creates `internal/sourceintent/contract.go` from the agreed design; architect
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
  Prove recovery without a new request and cancellation retaining pending work under `-race`.

## 4. Current publication receipts and reactivation

- [ ] Bind publisher observer to exact handle/generation and NEW per-boot seed epoch before Start;
  test no global mutable registry and that prior-epoch receipts cannot grant freshness. Test root
  rejects missing, duplicate, mismatched or altered factory/config/instance bindings.
- [ ] Persist receipts only after PubAck; receipt failures remain retryable, visible and drain-owned.
  Prove `TestPublisherReceiptAfterAck`, receipt-only retry after PubAck (no republish), immutable
  evidence, failure recovery, shutdown deadline, and crash/reseed repair.
- [ ] Seal an exact current-ID manifest only after successful enumeration, all expected publications
  and receipts agree; prove failure/cancellation cannot seal a partial seed, explicit empty seed
  completion, in-flight drain batch accounting, and watch/reseed separation.
- [ ] Compare authoritative source facts and clear only source_removed for proven current IDs inside
  the lifecycle mutation owner. Prove newer stale reasons are preserved and CAS conflicts retry.
- [ ] Prove legacy filesystem/empty-Absent/path-count sweeps cannot clear or replace source_removed,
  while ordinary file_deleted/path_missing/passage_removed behavior remains intact.
- [ ] Prove same-ID current parents/passages become fresh while removed files/symbols/tail passages
  remain stale in `TestReactivationCurrentEntityReceipts`; reject path-wide or UpdatedAt-only evidence.
- [ ] Prove crash after A+B receipts, offline deletion of B, and reseed A-only does not clear B; prove
  partial-removal/re-add and remove/add/remove/add preserve needed selective reactivation.

## 5. Real process acceptance

- [ ] Add an additive private-broker process suite for original-file and runtime-added sources:
  receipt and durable intent, old producer still live, checked process exit, producer absent on new
  boot, exact retained parent/passage markers, sibling entity/content unchanged.
- [ ] Kill after durable intent and before projection; restart the retained broker/application state
  and prove completion without an operator resend. Use explicit state barriers, never fixed sleeps.
- [ ] Prove re-add before first restart supersedes old work with zero old-generation mutation, and
  removal-complete/re-add/new boot clears only actually current entity IDs.
- [ ] Prove multi-page exact expected-ID set, deterministic query/mutation failure recovery and
  duplicate/redelivery with channel-controlled integration seams, plus process restart recovery.
- [ ] Keep original probes/ledgers unchanged; add exact commit, binary SHA, module pin, configuration,
  workload hash, checked process exits and named limitations to new result artifacts.

## 6. Review and delivery

- [ ] Independent Go reviewer approves context, serialization, error and stop ownership.
- [ ] Independent graph/event reviewer approves exact scope, retention, replay and re-add freshness.
- [ ] Pass gofmt, pinned revive/vet, unit/race, complete required integration and process suites,
  `task agents:check`, and strict OpenSpec validation; record legitimate blockers without skipped gates.
- [ ] Update PR #213 / issue #215 with concrete behavior, proof, exact pins and remaining blockers;
  keep SemStreams substrate and SemEngine qualification separate.
