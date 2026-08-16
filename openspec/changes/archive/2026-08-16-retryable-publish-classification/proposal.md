# Retryable publish classification

## Why

entitypub's `publishOne` retries only when `err.Error()` equals the literal
string "circuit breaker is open". Every other publish error is terminal on
first contact. Measured on the 2026-08-16 beta.161 OSH acceptance
(semsource#176, release blocker):

- A **120-second broker pause** — the textbook transient — permanently failed
  **5,282 entities** (~40/s) while `publish_retries_total` read 2 and the
  breaker-path WARN promised "entities are not lost while retrying".
- The **GRAPH stream ceiling breach** terminally failed 34,871 publishes on
  first attempt, each logged "entity publish failed **after retries**" — false
  on that path — as 34,871 separate WARN lines (the ADR-0011 noise class).

The classification is doubly wrong upstream-aware: semstreams' client marks
stream-capacity errors (10077) **circuit-neutral by design** — a full stream is
not a broken transport — so the breaker path semsource keys on can never cover
exactly the class that needs patient retrying.

## What Changes

- **Classify by error class, not string.** Retryable: circuit-open
  (`natsclient.ErrCircuitOpen`), not-connected (`ErrNotConnected`),
  timeouts/deadlines (`nats.ErrTimeout`, `context.DeadlineExceeded`),
  no-responders, and stream-capacity refusals (`*jetstream.APIError` code
  10077). Terminal: everything else (marshal failures, invalid subjects,
  context cancellation from shutdown).
- **Bound retrying by a per-entity delivery budget** (attempts + backoff cap),
  so a genuinely dead transport still fails loudly — after the budget, not on
  first contact. Budget exhaustion is the only path allowed to say "after
  retries", and the message states the attempt count.
- **Idempotent publishes as the retry prerequisite**: retries switch to
  `PublishToStreamWithMsgID` with a deterministic per-payload ID (entity ID +
  content hash), because an ambiguous timeout may have succeeded server-side
  and graph-ingest APPENDS triples on merge (ADR-055 §5 T1) — an un-deduped
  retry would double-apply. Dedup holds within the stream's duplicate window,
  which bounds the retry budget's total wall-clock.
- **Aggregate failure logging**: per-entity terminal-failure WARNs collapse to
  one edge-triggered entry naming the first entity and error class, plus a
  running count on the existing failed counter/summary — never one line per
  entity of a flood.
- Backpressure visibility (gauge, retries counter, edge-triggered entry/clear
  lines) now covers ALL retryable classes, closing the D3 blind spot the live
  induction falsified.

## Capabilities

### Modified Capabilities

- `entity-publish-integrity`: the "never drops silently" and backpressure
  requirements gain the class contract — transient transport trouble retries
  within a budget (idempotently), only budget exhaustion or genuinely terminal
  classes fail an entity, and failure logging aggregates.

## Impact

- `internal/entitypub/publisher.go`: classification, budget, msgID stamping,
  aggregated failure logging; `metrics.go` unchanged in shape (the existing
  series now receive the classes they were designed for).
- `processor/*-source`: none — the publisher API is unchanged.
- Closes semsource#176; shrinks #177's blast radius (fewer failed entities to
  misreport); the boot-B induction (pause NATS 120s) becomes the acceptance:
  zero entities failed, retries/backpressure move, recovery clean.
