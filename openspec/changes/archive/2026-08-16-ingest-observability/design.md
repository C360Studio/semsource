## Context

See `proposal.md` — Why. The constraint that shapes this design is Coby's:
**do not spam info level, but be helpful when critical shit happens.** Those pull
in opposite directions only if logs are the delivery mechanism for everything. The
design's core move is to stop using logs for continuous state.

What already exists and is unused — this change is mostly *wiring*, not building:

- `metric.MetricsRegistry` is constructed at `cmd/semsource/run.go:324` and passed
  to every component as `deps.MetricsRegistry`. Exactly one component reads it.
- `entitypub.WithMetrics(registry, prefix)` exists at
  `internal/entitypub/publisher.go:110` and no source component calls it.
- The publisher already maintains `published / failed / dropped / retries` as
  atomics with accessors; they are logged only in `Stop()`.
- `SourceStatusReport` already carries `EntityCount`, `PublishTotal`,
  `ErrorCount`, and `LastError`, and a periodic reporter already exists — it just
  runs only in the watch phase.

## Goals / Non-Goals

**Goals:**

- Make the difference between "slow" and "stalled" observable from outside the
  process, without raising the log level.
- Reuse the existing registry, publisher counters, status payload, and periodic
  reporter rather than introducing a parallel telemetry path.
- Keep additional load proportional to time, not to entity count.

**Non-Goals (design-level):**

- No new telemetry dependency, exporter, or tracing system.
- No change to what `ready` means, or to any consumer gate.
- No per-entity logging at any level, at any severity.

## Decisions

### D1 — Continuous state goes to metrics and status; logs carry transitions

The failure mode being fixed is *categorical*: SemSource pushes counters and
progress through logs, then discovers the volume is unacceptable, then lowers the
severity until the signal disappears. Raising those severities back would just
restore the noise.

So the split is by kind of signal, not by volume:

| signal | home |
| --- | --- |
| counters, queue depth, progress | metrics + status payload |
| healthy→degraded, degraded→healthy | one log line per transition |
| per-item failure | counter + aggregate; detail at `Debug` |
| unrecoverable | `Error` |

*Rejected:* re-levelling the existing `Debug` failure logs to `Warn` and stopping
there. It fixes visibility and reintroduces exactly the flood that caused the
levels to be lowered in the first place.

### D2 — Progress is sampled on an interval, never emitted per entity

The existing periodic reporter is extended to run during the initial seed rather
than only in the watch phase, publishing the counts the payload already carries.

Interval-based is a correctness property, not a tuning choice: a per-entity or
per-file progress signal on a 28k-entity corpus would add ingest load in exact
proportion to the corpus size — making observability worst precisely where it is
needed most. A fixed interval costs the same on any corpus.

*Rejected:* emitting progress every N entities. N is corpus-dependent, and the
pathological case (no entities moving at all) emits nothing — which is the exact
scenario this change exists to expose. Time-based sampling reports a stalled seed
*because* the count does not change between samples.

### D3 — Retry pressure is its own series, distinct from failure

A publisher retrying every entity reports zero drops, zero failures, and zero
errors while being functionally stalled. That is the incident. Folding retries
into an error or failure count would either mislabel a recoverable condition as a
failure, or leave it invisible.

Kept separate so an operator can distinguish:

- `retries` climbing, `published` climbing → slow, healthy, self-correcting;
- `retries` climbing, `published` flat → **stalled**;
- `failed`/`dropped` climbing → losing data.

The middle row is the one nothing could express before.

### D4 — Degradation is edge-triggered, with explicit recovery

Each degradation signal is backed by a small piece of state: is this condition
currently active? Entering logs `Warn` once; leaving logs recovery. This is what
lets a consequential event sit at `Warn` without flooding — the very tension in
the constraint.

Applies to the signals currently at `Debug` that can silently break a guarantee:
sustained publish backpressure, status-report publishing failure, heartbeat
failure, lifecycle-trigger failure, and unavailable configured paths.

*Rejected:* rate-limited logging (log at most once per N seconds). It bounds
volume but loses the distinction that matters — an operator cannot tell a
condition that recurred from one that never cleared, and there is no recovery
signal at all.

### D5 — Metrics are labelled per source instance

A deployment runs several source components at once. An aggregate publish counter
would show movement whenever *any* source is progressing, hiding a single stalled
source behind its healthy siblings — reproducing the original failure in a new
place. Series are labelled by source instance.

## Risks / Trade-offs

- **The progress sampler becomes its own load** → interval-based and independent
  of entity volume (D2); the interval is the existing reporter's, not a new one.
- **`Warn` becomes noisy again** → transitions only, with recovery (D4). The
  per-item `Warn` flood is explicitly out of scope and left untouched, so this
  change does not worsen it either.
- **Cardinality growth on metrics** → labels are source instances, which are
  configuration-bounded and small; no entity-derived or path-derived labels.
- **This still does not explain the original stall** → correct, and stated as a
  non-goal. Success is that the next occurrence is diagnosable from outside the
  process; the counters in D3 would have distinguished the three cases above.
- **A green build proving nothing** → acceptance must include a booted stack
  where the surfaces are read while seeding is genuinely in progress. Asserting a
  progress field exists is not the same as asserting it *advances*, and only the
  latter distinguishes slow from stalled.

## Migration Plan

None required. Additive status fields, additive metrics, and log severity
changes. Consumers gating on `phase: "ready"` are unaffected — that gate's
semantics are explicitly unchanged. No rollback beyond reverting the change.

## Open Questions

None that affect the specs or the task breakdown. The reporting interval is a
tuning value to be chosen during implementation and stated in the acceptance run,
not a design fork.
