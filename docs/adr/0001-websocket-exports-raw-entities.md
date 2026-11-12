# ADR-0001: WebSocket Output Exports Raw Entities Before ENTITY_STATES

> **Status:** Amended by SemEngine port (2026-11-12; originally Accepted 2026-03-12)
>
> **SemEngine port amendment (2026-11-12, `#221` / `semengine#34`):** SemEngine's `output/websocket`
> (`v0.1.0-alpha.4+`) refuses JetStream input ports (`output/websocket/config.go:161`) and removes
> the `at-least-once` `ack`/`nack` delivery mode (`output/websocket/config.go:29`). Under SemEngine,
> `cmd/semsource/run.go` wires `output/websocket` to watch `KVWatchPort{Bucket: "ENTITY_STATES"}`
> instead of consuming the raw `GRAPH` stream (`graph.ingest.>`). Connected `/ws` clients now receive
> applied-state `put`/`synced` frames from `ENTITY_STATES` (delivered at-most-once and coalesced per
> key) rather than raw `graph.ingest.*` source envelopes (`entity`, `status`, `predicates`). Query
> and governance consumers continue to use `graph.query.*`, HTTP/MCP, or GraphQL.

## Context (Historical SemStreams Baseline, 2026-03-12)

SemSource emits graph entities from 8 source processors (ast, git, doc, cfgfile, url, image, video, audio)
into the GRAPH JetStream stream (`graph.ingest.entity`). Under SemStreams, two components consumed this stream in parallel:

- **graph-ingest** wrote entities to the `ENTITY_STATES` KV bucket, where `graph-index` built structural indexes (OUTGOING, INCOMING, ALIAS, PREDICATE).
- **websocket-output** broadcast entities directly to downstream consumers (SemSpec, SemDragon) via WebSocket.

Under SemEngine (`v0.1.0-alpha.4+`), `output/websocket` watches `ENTITY_STATES` KV directly instead of subscribing to the `GRAPH` JetStream stream:

| Tier | Capability |
|------|------------|
| 0 | Structural (`none`) and statistical (`bm25`) |
| 1 | Neural HTTP embeddings (`semembed`, `semengine#489`) |
| 2 | Community detection (`graph-clustering`); LLM summaries (`semengine#485`) |

## Decision (Amended 2026-11-12)

**Watch `ENTITY_STATES` KV for the WebSocket output (`KVWatchPort{Bucket: "ENTITY_STATES"}`), matching SemEngine's admitted `output/websocket` contract.**

Under SemStreams, WebSocket output consumed the raw `GRAPH` stream. Under SemEngine, `output/websocket` only accepts `KVWatchPort` inputs and emits applied-state `put` and initial-replay `synced` frames at-most-once (merged per entity key) with no `delivery_mode` or `ack`/`nack` framing.

## Consequences

### Positive

- Conforms to SemEngine's single-writer `ENTITY_STATES` architecture (`graph-ingest` is the sole writer; `output/websocket` is a read-only KV watcher).
- Reconnecting `/ws` clients receive the current applied `ENTITY_STATES` snapshot followed by `synced` and incremental `put` updates without replaying superseded raw JetStream messages.

### Negative / Consumer Impact

- **Wire-format break from SemStreams:** `/ws` no longer emits raw `graph.ingest.*` envelopes (`SEED`/`DELTA`/`RETRACT`/`HEARTBEAT` or `ack`/`nack`). It emits SemEngine `output/websocket` KV-watch frames (`put`/`synced`, at-most-once, merged per key) for `ENTITY_STATES`.
- Enrichment data (embeddings in `EMBEDDING_INDEX`, communities in `COMMUNITY_INDEX`) remains in separate KV buckets and is accessed via GraphQL/MCP/`graph.query.*`, not `/ws`.

## Data Flow Reference (SemEngine)

```
Source Processors (8)
    |
    | entitypub.Publisher
    v
graph.ingest.entity (GRAPH JetStream)
    |
    v
graph-ingest ---> ENTITY_STATES KV ---> graph-index ---> KV indices
                       |
                       +---> [optional: graph-embedding (BM25 / HTTP)]
                       |
                       +---> websocket-output (KVWatchPort) ---> ws://0.0.0.0:7890/ws ---> downstream
```

## Related Files

- `cmd/semsource/run.go` — graph subsystem and `websocketComponentConfig` (`KVWatchPort{Bucket: "ENTITY_STATES"}`)
- `internal/entitypub/publisher.go` — buffered entity publisher
- `graph/event_payload.go` — EntityPayload definition
- `docs/upstream/semengine-port-ledger.md` — `output/websocket` crosswalk entry
