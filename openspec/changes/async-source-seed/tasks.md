## 1. Audit before changing anything

- [x] 1.1 Find every source component whose `Start()` performs its initial seed
      synchronously; confirm the list rather than assuming it matches the eight
      that build a publisher
- [x] 1.2 (result: every reader is a live/stopped guard or `Health()`; none
      conflated it with "seeded", so moving it was safe) Find every reader of `running` (and of component "started" state) that
      actually means "finished seeding" — D4 names this the primary breakage risk
- [x] 1.3 Find tests that call `Start()` and then assert on seeded state; they
      will race once the seed is asynchronous and must await it explicitly
- [x] 1.4 Check whether any non-source component also does unbounded work in
      `Start()` — the barrier does not care which component holds it

## 2. Split Start at the first unbounded wait

- [x] 2.1 Keep in `Start()`: config validation, publisher construction and start,
      the first status report, and the progress reporter
- [x] 2.2 Move into the seed goroutine: the path-availability retry, the initial
      seed, watcher startup, and periodic reindex — together, because watchers
      must follow the seed (D1)
- [x] 2.3 Set `running` when `Start()` returns; `phase` continues to distinguish
      seeding from watching (D4)
- [x] 2.4 Apply to every component found in 1.1, not only `ast-source`

## 3. Failure and shutdown

- [x] 3.1 Route seed failure to `error_count` / `last_error` plus a `Warn`, and
      confirm the source does not report ready afterwards (D2)
- [x] 3.2 Keep config-validation and publisher-construction failures synchronous
      so a bad config still fails the start fast (D2)
- [x] 3.3 Give the seed goroutine a cancellable context and a done channel;
      `Stop()` cancels, waits, and only then stops the publisher (D3)
- [x] 3.4 Bound the wait by the timeout `Stop` already receives, so a wedged seed
      cannot hang shutdown

## 4. Tests

- [x] 4.1 `Start()` returns promptly even when the seed is slow — assert on the
      return, with a deliberately slow fixture seed
- [x] 4.2 Seed failure after `Start()` returns is visible: error count non-zero,
      `last_error` populated, source not ready
- [x] 4.3 Invalid config still fails `Start()` synchronously — plus doc-source's
      existing body-store tests, which pin that an unavailable body store still
      fails the start hard
- [x] 4.4 `Stop()` during an in-flight seed returns only after the seed stops,
      and nothing publishes after the publisher is stopped — run under `-race`
- [x] 4.5 Phase still transitions seeding → watching, and `ready` is not reported
      while seeding
- [x] 4.6 Fix the tests found in 1.3 to await the seed rather than assuming
      `Start()` completed it
- [x] 4.7 Verify each new guard by mutating the implementation and confirming the
      test fails; a guard that passes vacuously is not a guard

## 5. Acceptance on a live stack

- [x] 5.1 Boot on a corpus large enough that seeding takes minutes — the defect
      does not reproduce on a small one, so a small-corpus pass proves nothing
- [x] 5.2 Poll `/source-manifest/status` within seconds of start: it must ANSWER,
      reporting the seeding phase, where it previously refused the connection
- [x] 5.3 Scrape `:9091/metrics` during the seed and confirm the publish counters
      are served
- [x] 5.4 Confirm the progress count ADVANCES between polls — measured
      0 -> 10,339 -> 24,835 -> 36,337 -> 40,853, which closes
      `ingest-observability` tasks 4.1/5.1
- [ ] 5.7 RESIDUAL GAP found during acceptance: the publish count then PLATEAUED
      for 60s+ while the seed was still working (goroutine dump: body-store
      `hashBody`). With retries/backpressure/errors all zero we can now tell it is
      not FAILING, but a plateau still cannot distinguish "parsing, not yet
      publishing" from "hung". Needs a liveness counter that advances during
      parsing, not only on publish
- [x] 5.5 Confirm `phase: "ready"` still appears only after seeding completes
- [x] 5.6 Record the time from process start to first successful status response:
      **connection refused for 10+ minutes -> answered in 5 seconds**

## 6. Documentation

- [x] 6.1 Note in `docs/upstream/semstreams-asks.md` that SemSource no longer
      trips semstreams#867, while the framework trap itself remains open
- [ ] 6.2 Record the follow-up: source status over a last-value KV bucket
      (the `graph/readiness` pattern) so seed progress survives an HTTP outage —
      today it is core-NATS fire-and-forget aggregated in memory behind an HTTP
      route, so nothing can read it when that route is down (D6)
- [ ] 6.3 Close out the `ingest-observability` change: its section 7 blocker is
      resolved by this change, so it can be verified and archived
