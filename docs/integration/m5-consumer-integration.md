# M5 Consumer Integration Guide

Instructions for connecting SemSpec and SemDragon to SemSource's graph.

## Overview

Consumers query SemSource's graph directly via NATS request/reply endpoints. No WebSocket client setup,
no FederationProcessor registration, and no bridge processor are required on the consumer side. SemSource
manages its own full graph pipeline internally.

### Internal pipeline (SemSource)

```
Source Processors → graph.ingest.entity → graph-ingest → ENTITY_STATES KV
                                                               |
                                                          graph-index → KV indices
                                                               |
                                                          graph-query ← graph.query.*
                                                               |
                                                          graph-gateway ← /graphql
```

### Consumer integration

```
[SemDragon/SemSpec] → graph.query.status        (gate on "ready")
                    → graph.query.entity         (fetch entities by ID)
                    → graph.query.relationships  (traverse the graph)
                    → graph.query.pathSearch     (path queries)
                    → /graphql                   (rich queries, port 8082)
```

## Gating on Readiness

Before querying the graph, consumers must wait for SemSource to complete its initial seed pass.

### Option A: Subscribe to the GRAPH JetStream stream

Subscribe to `graph.ingest.status` on the `GRAPH` stream and watch for a message with
`"phase": "ready"`.

### Option B: Poll via NATS request/reply

Send a NATS request to `graph.query.status` and retry until the response contains
`"phase": "ready"`.

### Option C: Poll via HTTP

```
GET http://localhost:8080/source-manifest/status
```

Retry until `phase` is `"ready"`.

## Status Payload Schema

The status response is a JSON object:

```json
{
  "namespace": "myorg",
  "phase": "ready",
  "sources": [
    {
      "instance_name": "ast-source-0",
      "source_type": "ast",
      "phase": "watching",
      "entity_count": 1842,
      "error_count": 0
    },
    {
      "instance_name": "doc-source-0",
      "source_type": "docs",
      "phase": "watching",
      "entity_count": 37,
      "error_count": 0
    }
  ],
  "total_entities": 1879,
  "timestamp": "2026-03-19T10:22:04Z"
}
```

### Aggregate phases

| Phase | Meaning |
|-------|---------|
| `seeding` | Initial ingest in progress |
| `ready` | All sources completed initial ingest |
| `degraded` | Seed timeout fired before all sources reported |

### Per-source phases

| Phase | Meaning |
|-------|---------|
| `ingesting` | Performing initial ingest |
| `watching` | Watching for changes |
| `idle` | No watch configured |
| `errored` | Error encountered |

## NATS Query Endpoints

All endpoints use NATS request/reply. Send a JSON request body; receive a JSON response.

| Subject | Description |
|---------|-------------|
| `graph.query.entity` | Fetch a single entity by 6-part ID |
| `graph.query.relationships` | Fetch edges for an entity |
| `graph.query.pathSearch` | Traverse paths between two entities |
| `graph.query.status` | Current ingestion status (same as HTTP) |
| `graph.query.sources` | Configured source manifest |
| `graph.query.predicates` | Predicate schema grouped by source type |

These subjects are served by the `graph-query` component running inside SemSource.

## HTTP Endpoints (ServiceManager, default :8080)

Use HTTP as a fallback when NATS is not directly accessible:

| Endpoint | Description |
|----------|-------------|
| `GET /source-manifest/status` | Ingestion status |
| `GET /source-manifest/sources` | Configured source manifest |
| `GET /source-manifest/predicates` | Predicate schema |

## GraphQL Gateway

A GraphQL interface is available at port 8082 via the `graph-gateway` component:

```
GET/POST http://localhost:8082/graphql
```

This is the recommended interface for complex or exploratory queries involving multiple
entity types, relationship traversal, and filtering.

## Federation (Multi-Instance SemSource)

Federation is a SemSource-to-SemSource concern. Consumers do not need to handle it.

When multiple SemSource instances run against the same or overlapping codebases:

- `public.*` entities (deterministic IDs for open-source packages, standard predicates) merge
  automatically across instances.
- `{org}.*` entities are sovereign to their owning namespace and do not merge.

Consumers query a single SemSource instance and receive the already-federated view. No
consumer-side merge logic is needed.

## What Consumers Do NOT Need

| Item | Reason |
|------|--------|
| WebSocket client (`input/websocket`) | SemSource handles its own WebSocket output internally |
| `FederationProcessor` registration | Federation is SemSource-internal |
| Bridge processor | No `GRAPH_EVENTS` or `GRAPH_MERGED` streams to bridge |
| `GRAPH_EVENTS` stream | Not exposed to consumers |
| `GRAPH_MERGED` stream | Not exposed to consumers |
| `federation.ToEntityState()` calls | Consumers receive query responses, not raw event streams |

## SemSpec Integration Checklist

1. Choose a readiness gating strategy (NATS subscribe, NATS poll, or HTTP poll).
2. On `phase: "ready"`, begin querying via `graph.query.*` NATS subjects or HTTP.
3. Use `graph.query.entity` to resolve entities by 6-part ID.
4. Use `graph.query.relationships` to traverse edges between entities.
5. Use `/graphql` (port 8082) for complex multi-hop queries.

SemSpec's own processors (`source-ingester`, `ast-indexer`, `repo-ingester`, `web-ingester`)
continue publishing to their own `graph.ingest.entity` subject independently. SemSource graph
entities supplement the SemSpec graph — entity IDs do not collide because namespace prefixes differ.

## SemDragon Integration Checklist

1. Gate on `phase: "ready"` using any of the three options above.
2. Query source-graph entities (`code`, `git`, `docs`, `config`) via `graph.query.*`.
3. Make these entities available to quest tools and context builders that query ENTITY_STATES.
4. The `seeding` processor bootstraps agents/guilds independently — no changes needed there.

## Integration Test Outline

1. Start SemSource with a test config pointing at a small repo.
2. Subscribe to `graph.query.status` and wait for `phase: "ready"`.
3. Send a `graph.query.entity` request for a known entity ID; assert a valid response.
4. Send a `graph.query.relationships` request; assert edges are present.
5. Confirm `GET /source-manifest/status` returns matching phase and entity count.
