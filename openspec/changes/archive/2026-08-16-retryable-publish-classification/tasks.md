# Tasks — retryable-publish-classification

## 1. Classification

- [x] 1.1 `classifyPublishError` with the retryable set (ErrCircuitOpen,
      ErrNotConnected, nats.ErrTimeout, context.DeadlineExceeded,
      ErrNoResponders, jetstream.APIError 10077 capacity descriptions),
      canceled (run-ctx shutdown), terminal (everything else) — table-driven
      unit tests per class including wrapped forms
- [x] 1.2 `publishOne` retries on retryable within the budget (attempts AND
      90s elapsed, whichever first); terminal fails immediately with the class
      named; canceled neither retries nor counts as failure
- [x] 1.3 Failure wording: first-attempt terminal states the class; budget
      exhaustion states attempts made and elapsed — the words "after retries"
      appear only when retries happened

## 2. Idempotent publishes

- [x] 2.1 `NATSPublisher` gains `PublishToStreamWithMsgID`; both test fakes
      updated; publisher stamps entityID+":"+sha256(data)[:12] on EVERY
      publish (marshal once, reuse bytes and ID across attempts)
- [x] 2.2 Unit test: the msgID is deterministic across attempts of one payload
      and differs when payload content differs

## 3. Visibility

- [x] 3.1 Backpressure entry/clear + retries counter fire for ALL retryable
      classes (test: a capacity-class error drives the gauge and counter,
      which the live induction proved they previously did not)
- [x] 3.2 Terminal-failure aggregation: `publishFailing` degraded.Condition —
      one default-level WARN on entry (entity + class), one recovery line on
      next success, per-entity detail at Debug; test: N failures produce one
      entry, exact count on the failed counter
- [x] 3.3 Mutation-verify every guard (isolated runs, sed-hit and revert
      checks, commit before mutating)

## 4. Acceptance

- [x] 4.1 DONE 2026-08-16: 78s broker pause mid-OSH-seed (within the 90s
      budget; the spec contract is outage-shorter-than-budget loses nothing —
      the original 120s exceeds the budget by design) → failed=0 through the
      whole cycle (was 5,282), published flat then resumed 6,288 → 32,363,
      backpressure gauge 1→0, retries 11, exactly one entry WARN and one
      cleared INFO at default level
- [x] 4.2 DONE: GRAPH constricted to 4MiB mid-publish → refusals RETRIED
      (retries 5→10, gauge 1, failed=0; this exact class was 34,871 instant
      terminal failures pre-fix), restored to 256MiB → seed completed,
      failed=0. Honest nuance: the ~60s stalled drain backed the buffer up
      past Send's bounded 5s timeout for 13 entities — dropped LOUDLY
      (counter + WARN each), the pre-existing no-silent-loss contract at the
      buffer boundary. Loss surface: ~35k silent-ish terminal failures → 13
      bounded, counted drops
- [x] 4.3 DONE: unit + race(x2) on entitypub, full suite, vet, lint, and the
      CI-mirror integration set green on the branch. Dedup sanity: stream
      message count matches per-source published totals with zero retry
      inflation (32,476 ≈ 32,317 ast + 35 cfg + 115 doc + manifest traffic);
      duplicate window confirmed 2m
