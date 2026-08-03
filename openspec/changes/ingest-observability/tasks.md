## 1. Publisher counters as metrics

- [x] 1.1 Register `published / failed / dropped / retries` on the
      `metric.MetricsRegistry` the publisher is already handed, labelled by source
      instance (D5)
- [x] 1.2 Keep `retries` a distinct series — never folded into failures (D3)
- [x] 1.3 Call the existing `entitypub.WithMetrics(registry, prefix)` from every
      source component that builds a publisher; it exists today and is never called
- [x] 1.4 Confirm the metrics endpoint serves SemSource series on a running
      instance, not just platform process/runtime defaults

## 2. Progress during seeding

- [x] 2.1 Run the existing periodic status reporter during the initial seed, not
      only in the watch phase (`processor/ast-source/component.go:822`)
- [x] 2.2 Carry the cumulative confirmed-delivery count in each progress report,
      reusing the fields `SourceStatusReport` already has
- [x] 2.3 Keep sampling interval-based and independent of entity volume (D2)
- [x] 2.4 Verify the aggregate `phase: "ready"` gate is bit-for-bit unchanged —
      progress reporting must not make a seeding source look ready
- [x] 2.5 Apply to every source component that performs an initial seed, not only
      `ast-source`

## 3. Degradation escalation

- [x] 3.1 Add the small "is this condition active?" state each signal needs, so
      entry logs once and recovery is reported (D4)
- [x] 3.2 Sustained publish backpressure → `Warn` on entry, recovery on clear
      (`internal/entitypub/publisher.go:291`, currently `Debug` on attempt 0 only)
- [x] 3.3 `failed to publish status report` → transition-based `Warn` (8 components)
- [x] 3.4 `lifecycle trigger failed (staleness marking degraded, not fatal)` →
      transition-based `Warn` (3 components)
- [x] 3.5 `heartbeat publish failed` → transition-based `Warn`
- [x] 3.6 `waiting for … paths to become available` → edge-triggered `Warn` with
      recovery (3 components). Correction: the wait is bounded by
      `retry.Persistent` (~30 attempts), not indefinite as first written — but a
      source that never reaches its paths still seeds nothing, silently

## 4. Tests

- [x] 4.1 (verified live in async-source-seed: 0 -> 10,339 -> 24,835 -> 36,337 ->
      40,853) Progress advances: seed a fixture corpus, sample status repeatedly,
      assert the count strictly increases — asserting the field merely *exists*
      does not distinguish slow from stalled
- [ ] 4.2 Stalled seed: with delivery blocked, assert the count does NOT advance
      while the phase stays seeding
- [x] 4.3 Retry pressure: with the transport applying backpressure, assert
      `retries` rises while `failed`/`dropped` stay flat (D3)
- [x] 4.4 Transition logging: assert one entry line and one recovery line across
      many degraded events — not one per event (D4)
- [x] 4.5 Readiness gate unchanged: existing readiness tests still pass untouched
- [x] 4.6 Verify each new guard by mutating the implementation and confirming the
      test fails; a guard that passes vacuously is not a guard

## 5. Acceptance on a live stack

> **Partial. The change is verified at the component level but NOT end-to-end for
> a large seed.** Reproduced on the full OSH Core corpus: the seed runs (goroutine
> dump shows `ast-source.Start -> publishParseResult -> bodiesForResult ->
> hashBody`, working, not deadlocked) and the new progress reporters ARE running
> during it — but **both `:8080` and `:9091` are unreachable for 10+ minutes**, so
> nothing can read what they publish. Why the HTTP surfaces stay down during a
> large seed is unproven and is the next question; see task 7.

- [x] 5.1 (done via async-source-seed acceptance: status answered in 5s, count
      advanced 0 -> 40,853) Boot a stack on a corpus large enough that seeding takes minutes; poll
      the status surface during the seed and record the advancing count
- [x] 5.2 Scrape the metrics endpoint and record the publish counters — 15
      SemSource series served where there were previously zero, per source
      instance, matching the status payload exactly (lib-ogc, 14,388 entities)
- [ ] 5.3 Induce backpressure and confirm the three-way distinction is readable
      from outside the process: slow-healthy, stalled, losing data (D3)
- [ ] 5.4 Confirm the default log level surfaces the degraded condition without
      raising verbosity

## 7. Blocked on an unproven cause — the surfaces themselves

- [x] 7.1 SOLVED: unbounded `startComponentsBarrier` + a synchronous seed in
      `Start()`; `Manager.StartAll` is sequential and binds HTTP only afterwards.
      Fixed our half in `async-source-seed`; framework half is semstreams#867.
      Original text: Determine why `:8080` (status) and `:9091` (metrics) are both
      unreachable while a large seed runs. Component starts are concurrent and
      `source-manifest` IS running (its heartbeat goroutine is live in the dump),
      so "a blocking Start() starves the others" is NOT the explanation
- [x] 7.2 RESOLVED — the progress data is now readable (5s to first response).
      Original text: Until 7.1 is answered, the progress data added by this change is
      unreadable in exactly the scenario that motivated it — state that plainly
      rather than claiming the incident is closed
- [x] 7.3 DONE (PR #125). It was never hashing — sha256 over the same 25,960
      bodies is 43ms and the whole CPU side is 7.5s. The cost was one SYNCHRONOUS
      object-store Put per body-bearing entity. Deduped (12.3% of Puts rewrote an
      identical content-addressed key) plus bounded concurrency: initial index
      1,087s -> 474s (2.3x), identical 77,802 entities and 0 parse failures on
      both sides. Lesson recorded: one goroutine sample is not a profile

## 6. Documentation

- [x] 6.1 Write the ADR: route by kind of signal — continuous state to
      metrics/status, transitions to `Warn`/`Error`, per-item failures aggregated;
      control volume by aggregating, never by lowering severity
- [x] 6.2 Record the measured baseline that motivated it (semsource `Error` at
      2.3% and failure-shaped `Debug` at 54%, against ~21%/20% in the siblings) so
      the decision is anchored to evidence rather than taste
- [x] 6.3 Note the deferred follow-up: re-levelling the 139 per-item `Warn` calls
      and introducing a genuine `Error` tier
