# ADR-0011: Route observability by kind of signal, not by volume

- **Status:** Accepted
- **Date:** 2026-08-02

## Context

A full-corpus ingest ran for 30+ minutes producing no log output, no status, and
no metrics. There was no way to tell "seeding slowly under contention" from
"wedged", and the cause is still unknown — which is the point: SemSource emitted
nothing that could distinguish them.

The immediate causes were specific, but they rhymed:

- `ast-source.Start()` published status exactly twice — at the seed's two edges —
  with the counts frozen in between, because the periodic reporter only ran in
  the watch phase.
- The publisher's `published / failed / dropped / retries` counters existed as
  atomics and were logged **only in `Stop()`**.
- Circuit-breaker backoff — the one signal that would have named the condition —
  was logged at `Debug`, on the first attempt only.
- `:9091/metrics` was already served, already configurable, and SemSource
  registered nothing on it.

Measured against the sibling repos, semsource was the outlier: `Error` appeared in
**6 call sites (2.3%)** against ~21% in both semstreams and semspec, while **54%
of its `Debug` calls were failure-shaped** against 20% in semstreams. `Warn` had
swollen to 52% of all log calls, carrying mostly per-item parse failures.

That distribution is the symptom of a single underlying habit: **routing
continuous state through logs, then choosing a log level to manage the volume.**
Per-item failures were logged per item, which floods at corpus scale; so
consequential signals were pushed down to `Debug`, where they became invisible
exactly when they mattered. With `Error` effectively unused there was no severity
ceiling left, so nothing could be marked as the bad one.

## Decision

**Observability is routed by the kind of signal, not by how often it fires.**

| Signal | Home |
| --- | --- |
| Counters, queue depth, progress, lag | Metrics and the status payload |
| Transition healthy→degraded, degraded→healthy | One log line per transition |
| Per-item failure whose count scales with the corpus | A counter plus an aggregate; detail at `Debug` |
| Unrecoverable | `Error` |

Three rules follow, and they are the operative part of this decision:

1. **A condition that silently breaks a user-visible guarantee — readiness
   reporting, liveness, seed completion, delivery — is never logged below
   `Warn`.** If it is too noisy at `Warn`, that is a signal to aggregate it or
   make it edge-triggered, not to lower it.

2. **Volume is controlled by aggregating or by logging transitions, never by
   lowering the level of a consequential event until it disappears.** Lowering
   severity to manage noise is how the original defect was created.

3. **Continuous state is not logged at all.** A count, a depth, or a progress
   figure belongs on a surface that can be sampled — metrics for operators, the
   status payload for consumers — because the useful question is "is this
   advancing?", and only a sampled series answers it.

A corollary worth stating because it was the concrete failure: a long-running
operation must be observable **while it runs**, not only at its edges. Progress
sampling is interval-based and independent of item volume — a per-item signal
would scale its own cost with the corpus and, in the pathological case where
nothing is moving, emit nothing at all.

## Consequences

**Positive.** The three-way distinction that nothing could previously express
becomes readable from outside the process: retries rising with published rising is
slow-but-healthy; retries rising with published flat is stalled; failed or dropped
rising is data loss. Degradation is visible at the default log level without
raising verbosity, and a sustained condition costs one line plus one on recovery.

**Negative / accepted.** Two homes now exist for health information, so a new
signal requires a deliberate choice about which surface it belongs on — that
choice is the point, but it is a cost. Edge-triggered logging also needs a small
piece of state per condition, which is more machinery than an unconditional log
call.

**Deliberately deferred.** The 139 per-item `Warn` call sites — the noise half of
the same habit — are not re-levelled here, and a genuine `Error` tier is not yet
introduced. Both are real and both are separable; folding them in would have
turned an incident fix into a service-wide refactor.

**Scope.** This is a SemSource decision. semstreams and semspec both sit at ~21%
`Error` use and do not show the same inversion, so the drift is ours. No repo in
the family had a written logging convention, which is the most likely reason
SemSource drifted without anyone noticing.

## Alternatives considered

**Re-level the existing `Debug` failure logs to `Warn` and stop.** Rejected: it
restores exactly the flood that caused those levels to be lowered in the first
place, and the next person to find `Warn` too noisy would lower them again. The
volume problem has to be solved by aggregation and edge-triggering, or the
severity fix does not survive.

**Rate-limited logging (at most once per N seconds).** Rejected: it bounds volume
but loses the distinction that matters. An operator cannot tell a condition that
recurred from one that never cleared, and there is no recovery signal at all.

**A single aggregate publish counter rather than per-source-instance series.**
Rejected: an aggregate moves whenever any source is progressing, hiding one
stalled source behind its healthy siblings — reproducing the original failure in
a new place.

**Emit progress every N entities instead of on an interval.** Rejected: N is
corpus-dependent, and the pathological case — no entities moving — emits nothing,
which is the exact scenario the change exists to expose. Time-based sampling
reports a stalled seed *because* the count does not change between samples.
