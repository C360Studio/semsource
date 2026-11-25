# A full GRAPH stream at boot no longer crash-loops SemSource

Approved-by: Coby Leuschke (2026-11-25) sha256:8a88c174f38f

Track: design (the boot path; one added Requirement in `ingestion-readiness`).

## Why

source-manifest publishes the source manifest and the predicate schema to the GRAPH stream inside its `Start`, and
returns the error if the stream refuses (`processor/source-manifest/component.go:184,228-230,242-244`). The service
manager then rolls back every component and the process exits (semengine `service/service_manager.go:367-374`). When
boots stack faster than graph-ingest drains, GRAPH sits at its cap, so every restart fails the same way before
graph-ingest can drain it. On beta.11 dogfood that was 14 identical restarts, ended only by `nats stream purge GRAPH`
(#399, ledger F8 on #391).

Both broadcasts are redundant for every reader we can find. The manifest and the predicate schema are served from
memory on `graph.query.sources`, `graph.query.predicates` and HTTP, and no repository on this host reads either subject.

## What Changes

- **A refused start broadcast no longer stops the process.** source-manifest starts its query and HTTP surfaces
  whatever GRAPH says, and retries the manifest and the predicate schema in the background until each is delivered
  or SemSource stops.
- **The refusal is visible once.** One warning when the broadcast starts failing, one line when it is delivered.
- **Readiness is unchanged.** `phase` still means "seeded"; it does not wait for the broadcast.
- **Tests:** an e2e boot into a GRAPH stream already at its cap, which fails on main; unit tests of the retry.

Consumers: SemTeams, SemDragon and semspec read sources, predicates and status through graph query, MCP and HTTP.
Those answers do not change. A reader of `graph.ingest.manifest` on the stream sees the message late, not never.

## Decisions you are making by approving

- **When the ingest stream is full at startup, SemSource keeps running instead of exiting.** It starts, answers
  status and source queries, and lets the graph drain the backlog. Recommended over making startup wait until the
  stream has room, which would hold every status page at "not ready" for that time and still exit if the wait ran out.
- **The two startup broadcasts keep retrying until they are delivered or SemSource stops.** They are never given up
  on, so a stream reader eventually sees them. Recommended over giving up after a fixed time.
- **"Ready" does not wait for those broadcasts.** A consumer polling status sees ready as soon as the sources have
  seeded, even if the broadcast is still queued. Nothing new appears on the status page; the delay shows only in
  the log.
- **The broadcasts stay on the ingest stream.** Moving them elsewhere or dropping them is left for later.

## Non-goals

- Re-offering the whole seed on every boot (F1, #401) and the stream's discard policy (#396).
- Entities a stacked boot can still lose after the publisher's delivery budget: they are counted and degrade the
  phase today, which this change keeps.
- Moving `graph.ingest.manifest/status/predicates` off GRAPH, or dropping them.
- The stale ADR-0003 claim that runtime add/remove republishes the manifest (see design, Open Questions).
- Any SemEngine change.
