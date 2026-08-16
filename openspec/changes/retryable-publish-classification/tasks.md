# Tasks — retryable-publish-classification

## 1. Classification

- [ ] 1.1 `classifyPublishError` with the retryable set (ErrCircuitOpen,
      ErrNotConnected, nats.ErrTimeout, context.DeadlineExceeded,
      ErrNoResponders, jetstream.APIError 10077 capacity descriptions),
      canceled (run-ctx shutdown), terminal (everything else) — table-driven
      unit tests per class including wrapped forms
- [ ] 1.2 `publishOne` retries on retryable within the budget (attempts AND
      90s elapsed, whichever first); terminal fails immediately with the class
      named; canceled neither retries nor counts as failure
- [ ] 1.3 Failure wording: first-attempt terminal states the class; budget
      exhaustion states attempts made and elapsed — the words "after retries"
      appear only when retries happened

## 2. Idempotent publishes

- [ ] 2.1 `NATSPublisher` gains `PublishToStreamWithMsgID`; both test fakes
      updated; publisher stamps entityID+":"+sha256(data)[:12] on EVERY
      publish (marshal once, reuse bytes and ID across attempts)
- [ ] 2.2 Unit test: the msgID is deterministic across attempts of one payload
      and differs when payload content differs

## 3. Visibility

- [ ] 3.1 Backpressure entry/clear + retries counter fire for ALL retryable
      classes (test: a capacity-class error drives the gauge and counter,
      which the live induction proved they previously did not)
- [ ] 3.2 Terminal-failure aggregation: `publishFailing` degraded.Condition —
      one default-level WARN on entry (entity + class), one recovery line on
      next success, per-entity detail at Debug; test: N failures produce one
      entry, exact count on the failed counter
- [ ] 3.3 Mutation-verify every guard (isolated runs, sed-hit and revert
      checks, commit before mutating)

## 4. Acceptance

- [ ] 4.1 Boot-B induction re-run on this branch: pause NATS 120s mid-OSH-seed
      → ZERO entities failed (was 5,282), retries and backpressure gauge move
      during the pause, publish resumes on unpause, one backpressure entry +
      one cleared line at default level
- [ ] 4.2 Stream-capacity leg: constrict GRAPH max_bytes mid-seed (nats stream
      edit), confirm refused publishes retry rather than fail, then restore
      and confirm delivery completes with zero failed
- [ ] 4.3 Full unit + race on entitypub, CI-mirror integration set, lint green;
      dedup sanity: grep ingest logs for double-apply symptoms on retried
      entities (none expected — msgID dedup)
