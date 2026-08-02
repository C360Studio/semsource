# Asynchronous source seeding: a source's Start must not black out the service

## Why

`ast-source.Start()` runs its entire initial seed synchronously. On a 1,932-file
Java corpus that is 10+ minutes, and for that whole window **neither the HTTP
status surface nor the metrics endpoint binds at all** — `wget` from inside the
container gets *connection refused* on both ports.

The chain is proven (two goroutine dumps, plus debug logs showing every other
component started within ~4.5s):

```
Manager.StartAll                     (sequential; component-manager is first)
  └─ ComponentManager.Start
      └─ startAllComponents
          └─ startComponentsBarrier   batch.Wait(), no timeout
              └─ startComponent       <- our seed runs here, synchronously
...
completeHTTPSetup()                   never reached -> :8080 and :9091 never bind
```

The unbounded barrier is the framework's half and is filed upstream as
[semstreams#867](https://github.com/C360Studio/semstreams/issues/867). **This
change is our half**, and it is the half that actually removes the symptom:
a lifecycle `Start()` is a hook, not a place to do minutes of work.

It also unblocks the previous change. `ingest-observability` (#123) added seed
progress and publish metrics that are *published correctly and cannot be read*,
because the surfaces serving them do not exist during the seed. That work only
becomes useful once this lands.

## What Changes

- Source components' `Start()` performs **only** fast, deterministic setup —
  config validation, publisher construction, resolver wiring — and returns. The
  initial seed, and everything sequenced after it (watcher startup, periodic
  reindex), moves into a supervised goroutine.
- **BREAKING (internal lifecycle only)**: a seed failure can no longer be
  returned from `Start()`. It surfaces where delivery truth already lives — the
  source's `error_count` / `last_error` and a log at `Warn` — rather than as a
  component start error. Nothing in the public HTTP/MCP/NATS contract changes.
- `Stop()` cancels the seed goroutine and **waits for it** before stopping the
  publisher, so shutdown during a seed cannot publish into a stopped publisher.
- The path-availability retry (`retry.Persistent`, ~30 attempts) moves inside the
  goroutine too. Today a source whose paths never appear holds the barrier for
  the entire retry window with the surfaces dark.
- Applies to every source component that seeds synchronously, not only
  `ast-source`.

## Capabilities

**New Capabilities**: none.

**Modified Capabilities**:

- `ingestion-readiness` — owns the promise that phases and readiness sub-signals
  "tell the truth on every surface". A surface that has not bound cannot tell the
  truth at all, so the guarantee needs a availability clause: starting a source
  must not prevent the service's surfaces from becoming reachable. This is also
  what makes the progress requirement added in #123 verifiable end to end.

`runtime-telemetry` is **not** changed: what it says the service exposes remains
correct. The defect is that the surfaces are unreachable during startup, which is
a readiness/availability property, not a telemetry one.

## What this does not do

- **It does not fix the framework barrier.** With this change a slow SemSource
  component no longer trips it, but any other slow component still would. That is
  semstreams#867 and deliberately not worked around here beyond not tripping it.
- **It does not make seeding faster.** The container takes 600s+ where a
  parse-only benchmark of the same corpus takes 8.4s, and a goroutine dump caught
  it in body-store `hashBody`. That is a separate performance question, and this
  change makes it *observable* rather than solving it.
- **It does not change readiness semantics.** `ready` still means every source
  finished its initial seed. The difference is that the process is now reachable
  while it is *not* ready — which is the state readiness exists to report.

## Impact

- **Code**: `Start()` / `Stop()` in the source components (`ast-source`,
  `doc-source`, `cfgfile-source`, `git-source`, `url-source`, and the
  image/audio/video sources), plus the seed-goroutine supervision.
- **Surfaces**: `/source-manifest/status` and `:9091/metrics` become reachable
  within seconds of process start instead of after the seed. Consumers gating on
  `phase: "ready"` are unaffected — they now get an honest "not ready yet"
  instead of a connection refusal.
- **Risk**: `Start()` returning before the seed means the component reports
  started while still working. That is exactly what `phase` already encodes, but
  anything that conflated "started" with "seeded" must be found and corrected.
- **Depends on**: nothing upstream. This is entirely within SemSource.

## Non-goals

- Changing the meaning of `ready`, or adding a new readiness gate.
- Any workaround for the framework barrier beyond not tripping it.
- Seed performance, body-store hashing, or embedding throughput.
- Re-levelling the 139 per-item `Warn` calls (still deferred from #123).
