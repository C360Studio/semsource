# Ingest observability: make a stalling seed diagnosable

## Why

A full-corpus ingest (27,892 entities) ran for 30+ minutes producing **no log
output, no status, and no metrics** — there was no way to tell "seeding slowly
under contention" from "wedged". The cause is still unknown, and that is the
point: SemSource emitted nothing that could distinguish them.

This is not a missing feature so much as a mismatch. SemSource routes *continuous
state* through logs and then picks a log level to control the volume. Both halves
fail at once: per-item failures are logged per item (noise at corpus scale), so
degradation signals get pushed down to `Debug` (silence exactly when it matters).

Measured against the sibling repos, semsource is the outlier — `Error` is used in
**6 call sites (2.3%)** against ~21% in both semstreams and semspec, and **54% of
its `Debug` calls are failure-shaped** against 20% in semstreams. With no severity
ceiling in use, nothing can be marked as the bad one.

## What Changes

- **Seeding reports progress.** `ast-source.Start()` publishes status exactly
  twice — `"ingesting"` before the initial index and `"watching"` after it — with
  the whole seed in between. The periodic reporter only runs in the watch phase.
  A seeding source SHALL sample progress on an interval instead, so a frozen
  count becomes distinguishable from a moving one.
- **The publisher's counters become observable.** `published / failed / dropped /
  retries` exist as atomics and are logged **only in `Stop()`**. They are exported
  on the metrics endpoint and surfaced in source status. `retries` is the
  circuit-breaker signal — the single number that would have answered the incident.
- **Existing metrics wiring gets switched on.** `entitypub.WithMetrics(registry,
  prefix)` already exists and **no source component calls it**; the
  `metric.MetricsRegistry` is already constructed and handed to every component
  and is used by exactly one. Enabling it yields buffer `writes/reads/overflows`.
- **Degradation escalates on state transition, not per event.** Signals that can
  silently break a user-visible guarantee move from per-event `Debug` to a
  `Warn` on the healthy→degraded edge (and `Info` on recovery), so they are
  visible at the default level without logging on every occurrence.
- **An ADR records the convention** — route by *kind of signal*, not by volume.

Not breaking: additive fields on a status payload, additive metrics, and log
severity changes. No consumer contract is removed or renamed.

## Capabilities

**New Capabilities**:

- `runtime-telemetry` — what SemSource exposes for operators, and the severity
  contract for logs. Owns the metrics surface and the rule that continuous state
  goes to metrics/status while logs carry transitions and unrecoverable events.

**Modified Capabilities**:

- `ingestion-readiness` — owns per-source phases and counts on every surface.
  Today it guarantees the *terminal* truth (`ready` means seeded) but says nothing
  about the interval in between. Adds progress observability during seeding.
- `entity-publish-integrity` — already guarantees the publisher never drops
  silently, and that holds: the drop path increments a counter and logs `WARN`.
  What is uncovered is **backpressure that does not drop** — the circuit-breaker
  retry path, which is bounded (20 attempts, 10s cap) and effectively invisible.

## What is deliberately not in scope

The wider logging-discipline problem is real but separable, and mixing it in
would turn an incident fix into a repo-wide refactor:

- **Re-levelling the 139 per-item `Warn` calls** (`cfgfile: parse X failed` and
  friends) into aggregated summaries. This is the noise half of the mismatch and
  deserves its own pass.
- **Introducing a genuine `Error` tier** across the service.
- Any change to semstreams or semspec: both already sit at ~21% `Error` use, so
  the drift is ours. The ADR is written for SemSource, not proposed upstream.

## Evidence

| finding | location |
| --- | --- |
| status published twice around the whole seed | `processor/ast-source/component.go:237`, `:306` |
| periodic reporter is watch-phase only | `processor/ast-source/component.go:822` |
| counters logged only at shutdown | `processor/ast-source/component.go:838` |
| circuit-breaker backoff at `Debug`, attempt 0 only | `internal/entitypub/publisher.go:291` |
| metrics option exists, never called | `internal/entitypub/publisher.go:110` |
| registry built and passed to every component | `cmd/semsource/run.go:324`, `:343` |
| zero metrics registered anywhere in semsource | (no `RegisterCounter`/`prometheus.New*`) |

Other `Debug`-level signals that can silently break a guarantee: `failed to
publish status report` (8 components), `lifecycle trigger failed (staleness
marking degraded, not fatal)` (3), `heartbeat publish failed`, and `waiting for …
paths to become available` (3 — can hang a seed indefinitely).

## Impact

- **Code**: `internal/entitypub` (counter export), the source components
  (`WithMetrics` wiring, progress sampling during seed, severity escalation),
  `processor/source-manifest` (progress fields in the status payload).
- **Surfaces**: `:9091/metrics` gains SemSource metrics — today it serves none.
  `/source-manifest/status` gains progress fields during seeding; consumers that
  gate on `phase: "ready"` are unaffected, since the gate semantics do not change.
- **Docs**: a one-page ADR on signal routing.
- **Risk**: the progress sampler must not itself become a publish storm on a
  large corpus — it is interval-based, not per-entity.

## Non-goals

- Diagnosing the original stall. Its cause remains unknown; this change makes the
  *next* one diagnosable rather than retro-fitting a theory to the last one.
- Distributed tracing or a new telemetry dependency — the Prometheus registry and
  the status payload already exist and are the intended homes.
- Changing readiness semantics. `ready` continues to mean every source finished
  its initial seed; this adds visibility *before* that point, not a new gate.
- Per-entity logging at any level.
