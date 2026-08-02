## Context

See `proposal.md` — Why. The framework half is
[semstreams#867](https://github.com/C360Studio/semstreams/issues/867); this is
the SemSource half, and it is the one that removes the symptom.

The shape of the problem in code (`ast-source`, and the same pattern in every
other seeding source):

```go
func (c *Component) Start(ctx context.Context) error {
    c.publisher.Start(ctx)
    c.publishStatusReport(ctx, "ingesting")
    c.startProgressReporting(ctx)
    retry.Do(ctx, retry.Persistent(), ...)   // paths may not exist yet, ~30 attempts
    // ---- initial index: minutes on a large corpus ----
    c.publishStatusReport(ctx, "watching")
    // ---- watchers, periodic reindex ----
    c.running = true
    return nil
}
```

Everything from the retry onward is long-running, and all of it is inside the
lifecycle hook the framework barriers on.

## Goals / Non-Goals

**Goals:**

- `Start()` returns in milliseconds, so the runtime's startup path completes and
  the surfaces bind.
- Nothing that is currently reported stops being reported — a seed failure must
  remain visible, just through a different channel.
- Shutdown during a seed stays safe.

**Non-Goals (design-level):**

- No workaround for the framework barrier beyond not tripping it.
- No change to `phase` values, readiness semantics, or any consumer contract.
- No attempt to make the seed itself faster.

## Decisions

### D1 — Split `Start()` at the first unbounded wait

`Start()` keeps everything deterministic and fast: publisher construction and
start, the first status report, and the progress reporter. It hands the rest —
path retry, initial seed, watcher startup, periodic reindex — to one supervised
goroutine, then returns.

The split point is deliberately **the first unbounded wait**, not "the seed".
The path-availability retry precedes the seed and is itself up to ~30 attempts;
leaving it in `Start()` would mean a source whose paths never appear still blacks
out the surfaces, which is the same defect with a different trigger.

*Rejected:* moving only the parse/publish loop and leaving watcher startup in
`Start()`. Watchers must be registered after the seed to avoid double-processing
files the seed already handled, so leaving them behind would require `Start()` to
wait for the seed anyway — the ordering constraint is what forces the whole tail
to move together.

### D2 — Seed failure surfaces as delivery truth, not a start error

`Start()` currently returns an error when the initial index fails, and the
framework marks the component failed. Once the seed is asynchronous that return
path is gone, so the failure has to go somewhere it is still seen.

It goes where the same class of information already lives: the source's
`error_count` and `last_error`, plus a log at `Warn`. That is the channel
`entity-publish-integrity` already uses for parse failures and publish
rejections, so a consumer checking "did this source actually work" reads one
place rather than two.

What still fails `Start()` synchronously: config validation and publisher
construction — deterministic, immediate, and genuinely a misconfiguration rather
than a runtime condition. **Fast failures must stay fast**; making everything
asynchronous would turn a bad config into a silent runtime surprise.

*Rejected:* retrying the seed internally forever on failure. It converts a
visible error into an invisible loop, which is the failure mode this whole line
of work exists to remove.

### D3 — `Stop()` cancels the seed and waits for it

The seed goroutine gets a cancellable context and a done channel. `Stop()`
cancels, waits for the goroutine to exit, and only then stops the publisher.

The ordering is load-bearing: `publisher.Stop()` closes the buffer, so a seed
still running would publish into a closed publisher. Waiting is what makes
"stop during seed" safe rather than racy, and it is why a bare `cancel()` is not
enough.

The wait is bounded by the `Stop(timeout)` the framework already passes, so a
wedged seed cannot hang shutdown indefinitely.

### D4 — `running` becomes true when `Start()` returns

Today `running` is set after the seed, conflating "started" with "seeded". Those
are now genuinely different states and the component already has a field that
distinguishes them: `phase`. `running` means the component is live; `phase`
distinguishes seeding from watching. Any code reading `running` to mean "finished
seeding" is a bug this change must find rather than preserve.

### D5 — One supervised goroutine per component, not a shared worker

Each source supervises its own seed. A shared pool would couple sources' failure
and shutdown behaviour, and would reintroduce a barrier of its own — one slow
source delaying another's seed — which is the exact shape being removed.

### D6 — Not solved by the platform's readiness primitive, but it is the right pattern

`semstreams/graph/readiness` already implements what source status wants: a
last-value KV bucket with `Publisher` / `Watcher` / `Set` and Prometheus gauges,
readable over NATS with `nats kv get`. Its `BootstrapComplete` field is even
documented as "finished its INITIAL BUILD in the current process lifetime" —
effectively our definition of a seeded source.

It cannot be reused as-is: `Publisher.Publish` is typed to
`graph.IndexStatusResponse`, and the keys and `BucketGraphStatus` are graph-owned.
Publishing source status through it would either overload a graph contract or
write into a graph-owned bucket, both across the Product Boundary. The bucket
seam (`StatusWriter`, a one-method `Put`) is pluggable, so the payload type is
the only genuine blocker.

**It would not remove the need for this change either.** Even with source status
in KV, a `Start()` that blocks for minutes still holds the component barrier and
still prevents the HTTP and metrics listeners from binding. Async seeding fixes
the cause; a KV status path would only make the symptom survivable.

It is nonetheless the more robust observability answer and is recorded as a
follow-up: during the incident NATS was healthy throughout, `source-manifest` had
started successfully and was receiving status reports — the data existed and only
the read path was gone, because SemSource transports source status over **core
NATS** (`semsource.internal.status`, fire-and-forget, no retention) and aggregates
it in memory behind an HTTP route. A KV last-value bucket would have been readable
the whole time. Generalising `readiness.Publisher` beyond the graph payload is
framework-shaped and is filed as
[semstreams#868](https://github.com/C360Studio/semstreams/issues/868).

## Risks / Trade-offs

- **Something conflates "started" with "seeded"** → D4 names this as the primary
  audit target. Tests that call `Start()` and immediately assert on seeded state
  will now race, and must be made to await the seed explicitly. That is a real
  and expected cost of this change, not an incidental breakage.
- **A seed failure becomes easier to miss** → mitigated by D2 routing it to
  `error_count`/`last_error` plus a `Warn`. Acceptance must assert the failure is
  *visible*, not merely that the process survived.
- **Shutdown races the seed** → D3, with the framework's existing stop timeout as
  the bound.
- **Readiness could regress silently** → `ready` semantics are untouched, and
  acceptance re-runs the existing readiness tests plus a live check that a
  seeding source does not report ready.
- **A green build proving nothing** → this defect was invisible to a fully green
  suite and to a small-corpus stack; it only appeared on a large corpus under
  contention. Acceptance is a booted stack on a corpus big enough that seeding
  takes minutes, asserting the surfaces answer *during* that window. Anything
  smaller reproduces nothing.

## Migration Plan

None externally. No configuration, payload, subject, or endpoint changes. The
observable difference is that the status and metrics surfaces answer within
seconds of process start instead of refusing connections until the seed finishes.

## Open Questions

None that affect the specs or the task breakdown. Whether other components beyond
the source set do long work in `Start()` is worth an audit, but it is discovery
work inside this change rather than a design fork.
