# Design — retryable-publish-classification

## Context

See proposal.md — Why. Mechanics that matter: `publishOne`
(internal/entitypub/publisher.go) marshals once, then loops `maxAttempts=20`
calling `client.PublishToStream` with exponential backoff (500ms → 10s cap),
retrying ONLY on the literal breaker string. The client seam is the
`NATSPublisher` interface (one method). semstreams' client returns typed
errors (`natsclient.ErrCircuitOpen`, `ErrNotConnected`, wrapped
`*jetstream.APIError`, `nats.ErrTimeout`-class), deliberately keeps
stream-capacity 10077 errors circuit-NEUTRAL, and offers
`PublishToStreamWithMsgID` for duplicate-window dedup (ADR-055 §5 T1: the
ingest merge APPENDS triples, so an ambiguous retry without dedup
double-applies). The GRAPH stream runs the server-default 2m duplicate window.

## Goals / Non-Goals

- **Goal**: transient transport trouble never terminally fails an entity
  inside the delivery budget; terminal trouble and budget exhaustion fail
  loudly and honestly; floods aggregate.
- **Non-goal**: status-surface honesty (`publish_total`, ready-despite-loss) —
  that is semsource#177, separate change.
- **Non-goal**: durability-posture changes to the GRAPH stream (#160/#178).
- **Non-goal**: cross-restart redelivery semantics; the budget lives within
  one process lifetime and one duplicate window.

## Decisions

1. **Classification set** via `errors.Is`/`errors.As`, in one function
   `classifyPublishError(err) publishErrClass` returning retryable/terminal/
   canceled: retryable = `natsclient.ErrCircuitOpen`, `natsclient.ErrNotConnected`,
   `nats.ErrTimeout`, `context.DeadlineExceeded`, `nats.ErrNoResponders`, and
   `*jetstream.APIError` with ErrorCode 10077 (any capacity description —
   mirroring upstream's own `isCircuitNeutralStreamCapacityError` list).
   Canceled = `context.Canceled` when the RUN context is done (shutdown
   drain, not a failure). Terminal = everything else. Alternative — retry
   everything unknown — rejected: a marshal or subject bug retried 20 times
   is noise hiding a code defect; unknown errors stay terminal and the class
   appears in the failure log, so a misclassified transient shows up named.

2. **Budget = attempts AND wall-clock, whichever first.** Keep maxAttempts 20
   and the 500ms→10s backoff, add an elapsed ceiling of **90s** per entity —
   safely inside the 2m duplicate window so a retry can never outlive the
   dedup that makes it safe. Constants, not config: operators tune neither
   today; a knob without a consumer is the #147 lesson inverted.

3. **Idempotency: marshal once, stamp `Nats-Msg-Id` = entityID + ":" +
   sha256(payload bytes)[:12]** via `PublishToStreamWithMsgID` on every
   publish (not only retries — the first attempt must carry the ID for the
   retry's dedup to match it). The `NATSPublisher` interface gains the method;
   both test fakes updated. Content-derived suffix means: watch-event
   republish with changed content → new ID (delivered); unchanged-content
   republish within 2m → deduped server-side, which is strictly less write
   amplification (ast-source's file-hash gate already suppresses most of
   these). Alternative — revision counters — rejected: nothing durable issues
   one; content hash is intrinsic, like entity IDs themselves.

4. **Failure aggregation via the existing `degraded.Condition` pattern**: a
   `publishFailing` condition enters on the first terminal/budget-exhausted
   failure (one default-level WARN naming entity + class), clears on the next
   successful publish (one recovery line). Per-entity failure detail drops to
   Debug. Counters (`failed` atomic + `entities_failed_total`) keep exact
   counts; the seed-end story stays on the status/metrics surfaces where D3
   put it. `enterBackpressure`/`clearBackpressure` now wrap ALL retryable
   classes, not just breaker-open, closing the falsified D3 leg.

5. **Shutdown drain unchanged**: `drainBatch`/`flush` already stop on run-ctx
   cancellation; classification maps that to `canceled`, which neither
   retries nor increments failures — the entity stays in the buffer for
   flush's bounded attempt, exactly as today.

## Risks / Trade-offs

- [Retrying a genuinely wedged stream stalls the seed longer] → the plateau
  returns at worst for the budget (90s/entity, but backpressure is VISIBLE:
  gauge, retries, edge-triggered WARN) and the #175 guard already removed the
  flood that caused the only observed breach.
- [Dedup window shorter than budget on custom streams] → budget constant is
  derived-documented against the 2m default; the stream config is ours
  (graphStreamConfig) and does not set Duplicates. #160's posture work owns
  making that coupling explicit in config.
- [msgID on first attempt changes server-side behavior for rapid identical
  republish] → within-2m identical-content republish is deduped; that path is
  already suppressed by the file-hash gate, and where it isn't, dedup is the
  CORRECT merge semantics (ADR-055).

## Migration Plan

Ship with the blockers' tag; no wire or storage migration (msgID is a header;
the duplicate window is already active server-side). Rollback = revert.
Acceptance re-runs the boot-B induction: 120s broker pause mid-seed must end
with zero failed entities and clean recovery.

## Open Questions

- None blocking.
