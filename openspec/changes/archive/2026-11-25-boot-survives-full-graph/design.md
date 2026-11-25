# Design: a full GRAPH stream at boot does not stop SemSource

## Context

Pin semengine `v0.1.0-alpha.8`; main 775ba13. Every line below was opened for this draft.

**What exits (Q1, Q3).** source-manifest `Start` (`processor/source-manifest/component.go:142-176`) runs three steps.
- `startManifest` (`:179-198`) calls `publishManifest` (`:184`). That sets `responseData` first (`:216-219`), then
  returns the GRAPH publish error as `publish manifest: %w` (`:228-230`). The `graph.query.sources` responder is
  registered only after it (`:188`). **Fatal.** It has been fatal since the component was added (42ee19b, 2026-03-10).
- `startPredicateSchema` (`:238-262`) publishes before it sets `predicatesData` (`:242-244` vs `:248`). **Fatal**
  (3f3ce66). The manifest fails first, so this is the latent second exit.
- `startStatusAggregation` (`:266`). The initial status publish only warns (`:272-274`), as do seed-complete,
  recovery, timeout and removal (`:373,380,451`, `ingest.go:279`). The heartbeat republishes every 30 s and logs
  edge-triggered (`:387-412`).

`publishPayload` (`:573-583`) is a bare `PublishToStream`: no retry, no Msg-Id (semengine `natsclient/client.go:1295-1305`).

Nothing else publishes to GRAPH at start. The only `PublishToStream` callers are `component.go:582` and entitypub
(`internal/entitypub/publisher.go:531`). entitypub's `Start` only launches goroutines (`:257-267`). It retries 10077
inside a 90 s budget per entity (`classify.go:59-64`, `publisher.go:63`), then counts the loss. Every source seeds
off its `Start` through seedsup (e.g. `processor/cfgfile-source/component.go:210-216`, `ast-source/component.go:380`).
So the crash loop is the manifest publish alone. That matches the evidence: `source-manifest: publish manifest …
10077` ×14, and nothing else.

**Why one failure kills the process.** Components start concurrently, in two phases (semengine
`service/component_manager_lifecycle.go:15-30,49-54,73-87`). A failure rolls back every component, graph-ingest
included. `startAll` then returns `Service start failed` (`service/service_manager.go:367-374`), and
`cmd/semsource/run.go:208-209` exits. graph-ingest is not a `StoreProvider` (`component/dependencies.go:26-28`;
no processor implements it), so it starts in the same phase as source-manifest. Until every `Start` returns, HTTP
serves only diagnostics and answers everything else `503 NOT READY` (`service/service_manager_http.go:68-73`). The
full routes are committed after the loop (`service_manager.go:387`).

**Who reads the broadcasts (Q1).** The manifest is served from memory on `graph.query.sources` (`component.go:188-192`)
and `GET /source-manifest/sources` (`:741-755`). The predicates are served the same way (`:253`, `handlePredicates`
`:907`). No Go, TS or Svelte code under the sibling repos on this host reads `graph.ingest.manifest`, `.predicates` or
`.status`. semteams only mentions `.status` in a comment (`semteams/cmd/semteams/tools/addsource/payloads.go:14`), and
the sibling configs only declare GRAPH on their own NATS. Since 3604a9c (#213), runtime add/remove no longer
republishes the manifest, though ADR-0003 (`docs/adr/0003-programmatic-source-add-api.md:233-238`) still says it does.
Under workqueue (`run.go:987`), nothing consumes these subjects; they age out after an hour.

**Spec constraints.** `ingestion-readiness` already holds that starting a source does not keep the surfaces from
binding (`openspec/specs/ingestion-readiness/spec.md:96-133`), and that `ready` means seeded (`:11-25`).
`tier0-consumer-qualification` pins the five GRAPH subjects (`spec.md:63-72`).

## Goals / Non-Goals

Goals: a GRAPH stream at its cap never stops the process. graph-ingest drains it, and the surfaces answer.
No silent loss is added. Non-goals: see the proposal.

## Decisions

### D1. Start-time broadcasts are non-fatal and retried in the background (recommended)

| Option | Full GRAPH at boot | Surfaces during the wait | Forecloses |
| --- | --- | --- | --- |
| **A. non-fatal + background retry** | boots; graph-ingest drains; the broadcast lands once there is room | answer at once | a reader that treats the manifest message as a boot barrier (none found) |
| B. retry inside `Start` until GRAPH accepts, bounded | boots only if the drain finishes inside the bound, else exits as today, just slower | every route `503 NOT READY` until all Starts return (`service_manager_http.go:73`) | breaks the availability rule (`ingestion-readiness` `:96-133`) in spirit |
| C. take the broadcasts off GRAPH (core publish or KV) | boots | answer | collides with the five-subject Requirement and #394's non-goal; a core publish on a stream subject is still captured, and silently dropped at the cap |
| D. drop the start broadcasts | boots | answer | the documented push channel; a contract change for unknown readers |

B does work mechanically: graph-ingest shares the phase, so it drains while source-manifest blocks. But the wait
is unbounded in practice (one seed took ~60 s to drain), and every page reads "not ready" meanwhile.

Cost of A: about 40 lines in source-manifest. The responders and `predicatesData` are set before the first attempt.
One worker per broadcast runs under `c.workers`, with its cancel in `cancelFuncs`. Stop already cancels and then
joins (`:599-634`). An ambiguous timeout that is retried may store the manifest twice, which is harmless.

Adopter seam: a consumer that queries sources, predicates or status sees no change. A stream reader sees the
message late. An operator who does nothing finds the instance recovering on its own instead of looping. The log
shows the delay as one warning on entry and one line on delivery.

What would change my mind: a consumer that gates on the stream message itself. That points to D3-B.

### D2. Retry until delivered or Stop (recommended), not for a fixed time

A: capped backoff (500 ms to 10 s, entitypub's shape), forever, ended by Stop. Cost: one goroutine that is idle
while blocked. B: give up after N minutes and log. The served copy keeps working, so giving up only loses the push,
but it adds a constant and a second terminal state. Each attempt re-marshals the current manifest (`:203-219`), so a
late delivery is never stale.

### D3. Readiness does not wait for the broadcast (recommended)

A: `phase` keeps meaning seeded (`ingestion-readiness` `:11-25`). Nothing is added to the status payload. B: add a
`broadcast_pending` field to status. That is new exported surface for a channel nobody reads, and phase would have
to choose whether it gates. Choose B if an owner wants the delay to be observable without the log.

### D4. Status broadcasts stay as they are

They already only warn, and the heartbeat redelivers within 30 s (`:387-412`). No change.

## Risks / Trade-offs

- **A stacked boot can still lose entities.** entitypub retries each one for 90 s while graph-ingest drains. If
  the backlog outlasts that, the loss is counted and `degraded` is sticky. That is visible, not silent, and it
  belongs to #401/#396. It is not measured for a 268 MB backlog.
- **The repro's filler must be entities graph-ingest applies.** Whether a message graph-ingest terminates leaves a
  workqueue stream is not established, so junk filler might never drain.

## Migration Plan

No stream or config change and no reseed. The beta.11 adoption note needs nothing.

## Open Questions

1. **Owner ruling: D1.** Non-fatal with a background retry, or B, C or D?
2. **Owner ruling: D2.** Retry until delivered or Stop, or give up after a bound?
3. **Owner ruling: D3.** May `ready` be reported before the manifest broadcast lands, with no status field?
4. **Owner ruling: ADR-0003 `:233-238`.** File a follow-up for the stale republish claim, or fix it here as a one-line
   doc edit?
5. Measured in task 2, not ruled: on main, the process exits within seconds of boot against a full GRAPH.
