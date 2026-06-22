# M5 Consumer Integration Guide

Instructions for connecting SemSpec and SemDragon to SemSource's graph.

## Overview

Consumers query SemSource's graph directly via NATS request/reply endpoints or GraphQL. No WebSocket
client setup, no FederationProcessor registration, and no bridge processor are required on the consumer
side. In standalone mode, SemSource owns the full graph pipeline and binds its governed ownership
contract. In headless mode, the host app owns graph infrastructure and governance.

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
[SemDragon/SemSpec] → graph.query.status          (gate on "ready")
                    → graph.query.entity           (fetch entities by ID)
                    → graph.query.batch            (fetch several entities)
                    → graph.query.prefix           (page by ID prefix)
                    → graph.query.summary          (graph counts)
                    → graph.query.relationships    (traverse the graph)
                    → graph.query.pathSearch       (path queries)
                    → /graphql                     (rich queries, port 8082)
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
| `graph.query.entityByAlias` | Resolve an entity by alias |
| `graph.query.batch` | Fetch multiple entities |
| `graph.query.relationships` | Fetch edges for an entity |
| `graph.query.pathSearch` | Traverse paths between two entities |
| `graph.query.hierarchyStats` | Summarize hierarchy shape |
| `graph.query.prefix` | Page entities by ID prefix |
| `graph.query.spatial` | Spatial query surface |
| `graph.query.temporal` | Temporal query surface |
| `graph.query.semantic` | Semantic query surface |
| `graph.query.similar` | Similarity query surface |
| `graph.query.localSearch` | Local graph search |
| `graph.query.globalSearch` | Global graph search |
| `graph.query.summary` | Graph summary counts |
| `graph.query.searchGraph` | Search graph result expansion |
| `graph.query.status` | Current ingestion status (same as HTTP) |
| `graph.query.sources` | Configured source manifest |
| `graph.query.predicates` | Predicate schema grouped by source type |

The structural graph subjects are served by `graph-query`. The SemSource-specific status, source,
and predicate subjects are served by `source-manifest`.

Compatibility note: SemStreams beta.114 routes `graph.query.capabilities` from the GraphQL gateway,
but graph-query does not currently register a responder for it. SemSource should not be treated as
advertising that subject until the upstream responder contract is restored.

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

## Multi-Instance SemSource

Multiple SemSource instances are a graph-substrate concern. Consumers do not merge raw streams.

When multiple SemSource instances run against the same or overlapping codebases:

- Deterministic 6-part IDs keep entity identity stable across runs.
- SemSource declares exact predicate ownership for `semsource.entity.v1` in standalone mode.
- Headless hosts bind their own graph governance and decide how SemSource entities join the host graph.

Consumers query graph state through `graph.query.*` or GraphQL. No consumer-side stream merge logic is
needed.

## What Consumers Do NOT Need

| Item | Reason |
|------|--------|
| WebSocket client (`input/websocket`) | Query consumers use `graph.query.*` or GraphQL |
| `FederationProcessor` registration | Identity and ownership are handled by the graph substrate |
| Bridge processor | No raw event stream needs to be bridged into consumer storage |
| `GRAPH_EVENTS` stream | Not exposed to query consumers |
| `GRAPH_MERGED` stream | Not exposed to query consumers |
| `federation.ToEntityState()` calls | Consumers receive query responses, not raw event streams |

## Headless Mode: Host-Owned Governance And GRAPH Stream Configuration

When SemSource runs in `mode: headless`, it shares the host application's NATS JetStream
infrastructure rather than provisioning its own. SemSource skips stream provisioning and ownership
bootstrap in this mode; the host owns the `GRAPH` stream definition and governed graph contracts.

The host's `GRAPH` stream subject filter **must enumerate the data-plane subjects
explicitly**. Do not use a wildcard like `graph.ingest.>`.

```text
Subjects: [
  "graph.ingest.entity",
  "graph.ingest.batch",
  "graph.ingest.manifest",
  "graph.ingest.status",
  "graph.ingest.predicates",
]
```

This matches the subject list semsource's own `EnsureStreams` writes in standalone mode.

### Why the wildcard breaks the curator workflow

`graph.ingest.add.{namespace}` and `graph.ingest.remove.{namespace}` are NATS
request/reply subjects served by `source-manifest`. They are **control plane**, not data
plane. If the stream filter captures them (e.g. via `graph.ingest.>`), JetStream sends a
`PubAck` to the request's reply inbox the moment the message lands in the stream — racing
against, and always beating, the real `AddReply` produced by `source-manifest` (which has
to write to KV first).

The caller then decodes the `PubAck` envelope (`{"stream":"GRAPH","seq":N}`) as the
response, sees zero components, and assumes the add failed. The KV write that would have
triggered component spawn never happens.

### Boot-time detection

Semsource scans every JetStream stream on startup in headless mode. If any stream's
subject filter captures `graph.ingest.add.*` or `graph.ingest.remove.*`, semsource logs
a loud `ERROR` line at boot identifying the stream and the offending filter — runtime
adds will misbehave silently otherwise.

## SemSpec Integration Checklist

1. Choose a readiness gating strategy (NATS subscribe, NATS poll, or HTTP poll).
2. On `phase: "ready"`, begin querying via `graph.query.*` NATS subjects or HTTP.
3. Use `graph.query.entity` to resolve entities by 6-part ID.
4. Use `graph.query.batch`, `graph.query.prefix`, and `graph.query.summary` for bulk context loading.
5. Use `graph.query.relationships` to traverse edges between entities.
6. Use `/graphql` (port 8082) for complex multi-hop queries.

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
4. Send a `graph.query.prefix` request; assert pagination returns SemSource IDs.
5. Send a `graph.query.summary` request; assert SemSource entity and predicate counts are included.
6. Send a `graph.query.relationships` request; assert edges are present when the fixture has them.
7. Confirm `GET /source-manifest/status` returns matching phase and entity count.
