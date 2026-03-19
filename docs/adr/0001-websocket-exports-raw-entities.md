# ADR-0001: WebSocket Output Exports Raw Entities Before ENTITY_STATES

> **Status:** Accepted | **Date:** 2026-03-12

## Context

SemSource emits graph entities from 8 source processors (ast, git, doc, cfgfile, url, image, video, audio)
into the GRAPH JetStream stream (`graph.ingest.entity`). Two components consume this stream in parallel:

- **graph-ingest** writes entities to the `ENTITY_STATES` KV bucket, where `graph-index` builds structural indexes (OUTGOING, INCOMING, ALIAS, PREDICATE).
- **websocket-output** broadcasts entities directly to downstream consumers (SemSpec, SemDragon) via WebSocket.

Both consume the same raw stream. The WebSocket output never sees data written to or derived from ENTITY_STATES.

Semstreams is moving its graph components to support a model registry with tiered processing:

| Tier | Capability |
|------|------------|
| 0 | Structural (current) |
| 2 | Statistical indexes |
| 3 | LLM embeddings, summaries, NLQ |

If tier 2/3 enrichment processors add triples to ENTITY_STATES (e.g., vector embeddings, summaries), those enrichments will be invisible to the WebSocket export.

## Decision

**Keep the current architecture: WebSocket output consumes the raw GRAPH stream.**

SemSource is a source component. Sources emit facts, not interpretations. Downstream consumers are responsible for their own enrichment tiers.

## Consequences

### Positive

- Simple, low-latency export path (no enrichment pipeline delay).
- Clean separation of concerns: source emits raw data, consumers enrich as needed.
- No coupling to semstreams tier capabilities that may change.

### Negative

- If a downstream consumer expects pre-enriched entities, it cannot get them from the WebSocket export.
- Enrichment data (embeddings, communities) is only accessible via GraphQL/MCP, not the WebSocket stream.

### Enrichment Architecture (Updated 2026-03-12)

Enrichment components write to **separate KV buckets**, not to ENTITY_STATES:

| Component | Output KV | What It Stores |
|-----------|----------|----------------|
| graph-embedding | `EMBEDDINGS_CACHE` | Vector embeddings per entity |
| graph-clustering | `COMMUNITY_INDEX` | Community detection results |

ENTITY_STATES holds raw entity triples regardless of tier. The WebSocket export divergence is about missing derived data in parallel KV buckets, not about ENTITY_STATES containing different triples. Downstream consumers access enrichment via GraphQL/MCP queries, not the entity stream.

## Data Flow Reference

```
Source Processors (8)
    |
    | entitypub.Publisher
    v
graph.ingest.entity (GRAPH JetStream)
    |
    +---> graph-ingest ---> ENTITY_STATES KV ---> graph-index ---> KV indices
    |                                                  |
    |                                        [enrichment: graph-embedding (BM25, default)]
    |
    +---> websocket-output ---> ws://0.0.0.0:7890/graph ---> downstream
```

### Enrichment Architecture

`graph-embedding` (BM25) is wired by default in `graphSubsystemComponents()` with
`coalesce_ms: 200`. Embeddings are written to `EMBEDDINGS_CACHE`, not to `ENTITY_STATES`.
The WebSocket export is unaffected — it always reflects the raw entity stream.

## Related Files

- `cmd/semsource/run.go` — graph subsystem and websocket component wiring
- `internal/entitypub/publisher.go` — buffered entity publisher
- `graph/event_payload.go` — EntityPayload definition
