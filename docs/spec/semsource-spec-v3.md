# SemSource — Specification v0.3

> Graph-first knowledge ingestion for the SemStream platform  
> **Status:** Draft | **Version:** 0.3.0 | **Date:** March 2026

---

## Table of Contents

1. [Purpose & Problem Statement](#1-purpose--problem-statement)
2. [Design Goals](#2-design-goals)
3. [Architecture](#3-architecture)
4. [Entity Identity](#4-entity-identity)
5. [Source Handlers](#5-source-handlers)
6. [Real-Time Watching](#6-real-time-watching)
7. [Output Contract](#7-output-contract)
8. [semstreams Integration](#8-semstreams-integration)
9. [Bootstrap Flow](#9-bootstrap-flow)
10. [Milestones](#10-milestones)
11. [Open Questions](#11-open-questions)
12. [Vocabulary References](#appendix-vocabulary-references)

---

## 1. Purpose & Problem Statement

SemSource is a standalone, federated graph-population service for the SemStream platform. Its sole responsibility is ingesting heterogeneous source material — code repositories, documentation, URLs, and structured artifacts — and emitting a normalized, continuously updated knowledge graph stream that any downstream consumer can ingest without coupling to the sourcing pipeline.

The problem SemSource solves is **bootstrapping cost**. Today, populating a knowledge graph requires manual JSON configuration and deep knowledge of the graph entity schema. This friction prevents adoption and forces consumers (SemSpec, SemDragon, future applications) to solve the same sourcing problem independently.

> **Core insight:** The graph is only as useful as what is in it. SemSource makes seeding feel like _"point at things you already have"_ — not _"describe your project to me."_

The immediate forcing function is the SemDragon requirement: a game board must be pre-seeded with real project topology before agents begin work, and it must stay current as the codebase evolves. The need is universal — every project on the platform faces the same bootstrapping problem. SemSource solves it once.

### Immediate Use Case: Parallel Consumers

The MVP must support a single SemSource instance feeding multiple consumers simultaneously. The canonical example: one SemSource instance feeding both SemSpec (dev workflow) and SemDragon (game board) from the same project graph stream. Each consumer connects independently. Neither knows the other exists.

```
SemSource (acme/gcs repo)
    ├──▶  SemSpec   (dev workflow agent context)
    └──▶  SemDragon (game board pre-seed)
```

---

## 2. Design Goals

### 2.1 Primary Goals

- Ingest any supported source type and emit graph entities with zero consumer coupling
- Support real-time watching: file changes, new commits, and URL updates propagate as delta graph events without restart
- Support fan-out: one SemSource instance feeding multiple consumers simultaneously via WebSocket broadcast
- Support federated deployment: multiple SemSource instances feeding one or more consumers
- Deterministic entity identity — two independent instances processing the same source produce identical node IDs
- Operate as a first-class semstreams flow using existing output components
- Eliminate manual JSON configuration as the bootstrapping mechanism

### 2.2 MVP Scope

MVP targets **same-LAN deployment**. SemSource runs its WebSocket server on the local network; consumers connect outbound as clients. No firewall configuration, no TLS, no reverse proxy required.

Cross-network deployment (TLS, reverse proxy) is a documented post-MVP path using the existing nginx configuration pattern in the WebSocket output README. No architectural changes required — it is an operational concern, not a code concern.

### 2.3 Non-Goals (MVP)

- Cross-network TLS termination (operational concern, documented path exists)
- UI for source management (delegated to SemSpec sources feature)
- Authentication / credential management for private sources
- Cross-org entity deduplication beyond the `public.*` namespace convention
- Hierarchical SemSource aggregation (one SemSource consuming another)

---

## 3. Architecture

### 3.1 Position in the Stack

SemSource sits immediately above semstreams infrastructure and below all application-level consumers. It is a peer service — its own flow — not a library embedded in SemSpec or SemDragon.

```
semstreams  (NATS JetStream infrastructure)
    └── semsource flow   (graph population)    ← this repo
            ├── semspec flow    (dev workflow consumer)
            ├── semdragons flow (game board consumer)
            └── [future consumer flows]
```

### 3.2 Transport Model

SemSource uses the **existing `output/websocket` component** as its output. This is a WebSocket server — it binds a port and consumers connect to it as clients.

On the consumer side, each flow uses the **existing `input/websocket`** component in `ModeClient`, connecting outbound to SemSource. This is already validated in `websocket_integration_test.go` — the `TestWebSocketFederation_AckFlow` test demonstrates exactly this server/client pairing.

```
SemSource flow
  └── [handlers] → [normalizer] → output/websocket (server, binds :7890)
                                           ↑
                                    same-LAN network
                                           ↓
SemSpec flow                    SemDragon flow
  └── input/websocket             └── input/websocket
      (ModeClient)                    (ModeClient)
        └── FederationProcessor         └── FederationProcessor
              └── graph ingestion             └── board pre-seed
```

### 3.3 Fan-Out

Fan-out is handled by the WebSocket output's existing multi-client broadcast. SemSpec and SemDragon each maintain their own client connection to the same SemSource endpoint. The server broadcasts every `GraphEvent` to all connected clients simultaneously. No coordination between consumers, no consumer awareness on the SemSource side.

This is the same pattern validated in the integration tests — multiple concurrent clients each receiving the full message stream.

### 3.4 Federation

Multiple SemSource instances can serve different source scopes. Each runs its own WebSocket server. Consumer flows connect to each relevant SemSource instance, running a `FederationProcessor` per connection to apply merge policy before graph ingestion.

```
SemSource A  (repo + AST)  :7890 ──▶ FederationProcessor ──▶ graph ingestion
SemSource B  (docs + URLs) :7891 ──▶ FederationProcessor ──┘
```

### 3.5 Component Flow

SemSource follows the standard semstreams pattern: **Listen → Process → Persist → Publish**.

```
[ semsource.yaml config ]
         │
         ▼
[ Source Intake ]  ←── initial trigger + watch events
         │
         ▼
[ Handler Dispatch ]  ←── routes by source type
         │
    ┌────┴────┬──────────┬──────────┐
    ▼         ▼          ▼          ▼
  [Git]     [AST]     [URL]     [Docs]  [Config] ...
    │         │          │          │
    └────┬────┴──────────┴──────────┘
         ▼
[ Graph Normalizer ]  ←── entity ID construction, namespace routing, dedup
         ▼
[ output/websocket ]  ──▶ broadcast to all connected consumer clients
```

### 3.6 Real-Time Event Loop

The component is designed around a continuous event loop. Initial seeding is the first pass of the same loop — there is no separate "batch mode."

```
on start:
    full_ingest(all configured sources)
    emit(GraphEvent, type=SEED)
    start_watchers(all configured sources)

on file_change(path):
    entities = dispatch(path).ingest(path)
    emit(GraphEvent, type=DELTA, entities=entities)

on git_commit:
    git_entities  = GitHandler.ingest(new_commits)
    ast_entities  = ASTHandler.ingest(changed_files)
    emit(GraphEvent, type=DELTA, entities=git_entities + ast_entities)

on url_change(url):
    entities = URLHandler.ingest(url)
    emit(GraphEvent, type=DELTA, entities=entities)

on consumer_reconnect:
    emit(GraphEvent, type=SEED)  // re-seed on reconnect
```

---

## 4. Entity Identity

### 4.1 The 6-Part ID Scheme

SemSource uses the standard platform 6-part hierarchical entity ID. All IDs are valid NATS KV keys, preserving the KV twofer from the subject hierarchy.

```
Format:  {org}.{platform}.{domain}.{system}.{type}.{instance}
Example: acme.semsource.golang.github.com-acme-gcs.function.NewController
```

| Part | Example | Description |
|------|---------|-------------|
| `org` | `acme` / `public` | Organization slug. `public` is reserved for open-source / intrinsic entities |
| `platform` | `semsource` | Originating service |
| `domain` | `golang` / `git` / `web` | Source domain or language |
| `system` | `github.com-acme-gcs` | Canonical source system, dots/slashes replaced with dashes |
| `type` | `function` / `commit` / `doc` | Entity type |
| `instance` | `NewController` / `a3f9b2` | Symbol name or content hash |

### 4.2 The `public.*` Namespace

The critical federation property is **deterministic identity across independent SemSource instances without coordination**. Two instances processing the same open-source Go package must produce identical entity IDs.

`public` is a reserved first-segment constant for any entity whose identity is intrinsic to the artifact rather than assigned by an organization.

```
# Open-source symbol — identical from any SemSource instance worldwide
public.semsource.golang.github.com-gin-gonic-gin.function.New

# Go stdlib
public.semsource.golang.stdlib-net-http.function.ListenAndServe

# Public doc URL
public.semsource.web.pkg.go.dev.doc.c821de

# Private / org-owned — sovereign to acme
acme.semsource.golang.github.com-acme-gcs.function.NewController
```

**Merge rule:** `public.*` nodes merge unconditionally across any SemSource instance. `{org}.*` nodes are sovereign — the owning org controls identity. No central registry required.

### 4.3 ID Construction Algorithm

The algorithm must be **purely intrinsic** — no timestamps, instance IDs, or insertion-order dependencies.

| Entity Type | Construction |
|------------|--------------|
| Code symbol | `org + semsource + language + canonical_module_path + symbol_type + symbol_name` |
| Git commit | `org + semsource + git + repo_slug + commit + short_sha` |
| URL / doc | `org + semsource + web + domain_slug + doc + sha256(canonical_url)[:6]` |
| Config file | `org + semsource + config + repo_slug + file_type + sha256(content)[:6]` |

**Canonical URL normalization:** lowercase scheme and host, remove trailing slashes, resolve relative refs, strip query params unless semantically load-bearing, strip fragments.

---

## 5. Source Handlers

### 5.1 MVP Handler Set

| Handler | Source Types | Graph Output |
|---------|-------------|--------------|
| `GitHandler` | Local repo, GitHub remote | Commits, authors, file-touch edges, branch refs |
| `ASTHandler` | Go, TS/JS | Symbols, call edges, interface implementations, package deps |
| `DocHandler` | README, Markdown, plain text | Doc nodes, intent/narrative edges to code symbols |
| `ConfigHandler` | go.mod, package.json, Dockerfile | Dependency nodes, version constraints, environment topology |
| `URLHandler` | HTTP/S URLs, doc sites | Web doc nodes, link graph, content fingerprints |

> **Dependency note:** `ASTHandler` reuses the existing SemSpec AST indexer — imported as a dependency, not re-implemented. The indexer already emits graph-compatible entities.

### 5.2 Handler Interface

```go
type SourceHandler interface {
    // SourceType returns the handler identifier
    SourceType() string

    // Ingest processes the source and returns raw graph entities
    // before ID normalization. Called on initial seed and watch events.
    Ingest(ctx context.Context, cfg SourceConfig) ([]RawEntity, error)

    // Watch returns a channel of change events for real-time mode.
    // Returns nil if this handler does not support watching.
    Watch(ctx context.Context, cfg SourceConfig) (<-chan ChangeEvent, error)

    // Supports returns true if this handler can process the given config
    Supports(cfg SourceConfig) bool
}
```

### 5.3 Watch Support by Handler

| Handler | Watch Mechanism | Trigger |
|---------|----------------|---------|
| `GitHandler` | git hook / polling | new commit on tracked branch |
| `ASTHandler` | `fsnotify` on source dirs | file save / write close |
| `DocHandler` | `fsnotify` on doc paths | file save |
| `ConfigHandler` | `fsnotify` on config files | dependency change |
| `URLHandler` | configurable poll interval | content hash change |

### 5.4 Post-MVP Handler Candidates

- GitHub Issues / PR handler (with AI-generated content filtering)
- Python AST handler
- Rust AST handler
- OpenAPI / schema file handler
- MQTT source handler (government transport requirement)
- Jira / Linear issue tracker handler

---

## 6. Real-Time Watching

Real-time watching is an MVP requirement. The existing semstreams component infrastructure already supports reactive event propagation — SemSource is a natural fit for the same pattern.

### 6.1 Event Types

```go
type EventType string

const (
    EventSeed      EventType = "SEED"      // full initial ingest complete
    EventDelta     EventType = "DELTA"     // incremental update
    EventRetract   EventType = "RETRACT"   // entity removed (file deleted, etc.)
    EventHeartbeat EventType = "HEARTBEAT" // liveness signal on quiet periods
)

type GraphEvent struct {
    Type        EventType
    SourceID    string
    Namespace   string
    Timestamp   time.Time
    Entities    []GraphEntity  // upsert semantics on SEED/DELTA
    Retractions []string       // entity IDs to remove on RETRACT
    Provenance  SourceProvenance
}
```

### 6.2 Consumer Guarantees

- **SEED** carries the full current graph for the namespace. Emitted on start and on consumer reconnect.
- **DELTA** events are additive — consumers upsert, never replace the full graph.
- **RETRACT** explicitly signals removed entities (deleted files, removed symbols). Consumers must honor retractions to avoid stale graph state.
- **HEARTBEAT** is emitted on a configurable interval during quiet periods. Consumers use it to detect a dead SemSource instance and trigger reconnect logic.
- The WebSocket output's existing ack/nack protocol handles delivery reliability. Use `at-least-once` delivery mode for graph events.

---

## 7. Output Contract

### 7.1 WebSocket Server (MVP)

SemSource uses `output/websocket` as its output component. No new output components required.

```yaml
# SemSource flow output config
outputs:
  - name: graph_stream
    type: network
    subject: http://0.0.0.0:7890/graph
```

`GraphEvent` payloads are JSON-serialized and wrapped in the standard `MessageEnvelope`:

```json
{
  "type": "data",
  "id": "msg-1707654321000-42",
  "timestamp": 1707654321000,
  "payload": { /* GraphEvent */ }
}
```

### 7.2 Delivery Mode

Use `at-least-once` delivery mode for graph events. Consumers send ack/nack per the existing WebSocket output protocol. This ensures no graph events are silently dropped on slow or reconnecting consumers.

### 7.3 Post-MVP Transport Options

The transport is swappable by changing the output component in the flow config. No SemSource code changes required.

| Transport | Output Component | Use Case |
|-----------|-----------------|----------|
| WebSocket | `output/websocket` | MVP — same LAN |
| HTTP POST | `output/httppost` | Webhook-style push to a known endpoint |
| File | `output/file` | Air-gapped handoff, graph snapshots |
| OTEL | `output/otel` | Provenance and observability export |
| MQTT | _(post-MVP)_ | Government / restricted network environments |

### 7.4 Consumer Merge Policy

Consumers apply these rules when ingesting a `GraphEvent` via `FederationProcessor`:

| Rule | `public.*` | `{org}.*` |
|------|-----------|-----------|
| **SEED** | Upsert unconditionally | Upsert within org namespace |
| **DELTA** | Upsert unconditionally | Upsert within org namespace |
| **RETRACT** | Remove if locally present | Remove only within org namespace |
| **Cross-org overwrite** | N/A | Reject |
| **Edge conflicts** | Union semantics | Union semantics |
| **Provenance** | Always append | Always append |

---

## 8. semstreams Integration

### 8.1 What Already Exists

No new semstreams components are required for MVP. The existing components cover the full pipeline:

| Need | Existing Component |
|------|-------------------|
| SemSource output | `output/websocket` (server mode) |
| Consumer input | `input/websocket` (`ModeClient`) |
| Delivery reliability | WebSocket ack/nack protocol (`at-least-once`) |
| Observability | `output/otel` (post-MVP, already available) |

The `TestWebSocketFederation_AckFlow` integration test already validates the server/client pairing, ack flow, and multi-client broadcast that SemSource depends on.

### 8.2 New Component: `FederationProcessor`

The one new semstreams component required is `FederationProcessor`. It sits in each consumer's flow between the WebSocket Input and graph ingestion.

**Responsibility:** Apply graph merge policy, namespace routing, and entity ID validation. It is transport-agnostic — it receives `GraphEvent` structs regardless of how they arrived.

```go
// semstreams/processor/federation.go

type FederationProcessor struct {
    // Namespace this processor is authoritative for
    Namespace string

    // MergePolicy controls public.* vs org.* merge behavior
    MergePolicy MergePolicy
}

// Process applies merge policy to an incoming GraphEvent and
// emits a resolved GraphEvent ready for consumer ingestion.
func (f *FederationProcessor) Process(ctx context.Context, event GraphEvent) (GraphEvent, error)
```

**Why a shared processor rather than per-consumer logic:**

- Merge policy is identical across SemSpec and SemDragon — implement once
- Transport independence: works with WebSocket input today, MQTT input tomorrow
- Testable in isolation without standing up a full consumer flow
- Future consumers get correct merge behavior for free

### 8.3 Full MVP Pipeline

```
┌─────────────────────────────────────────────┐
│  SemSource flow                             │
│                                             │
│  [Git] [AST] [Docs] [Config] [URL]          │
│       └──────────┬──────────┘              │
│            [Normalizer]                     │
│                  │                          │
│       output/websocket (:7890)              │
└──────────────────┬──────────────────────────┘
                   │  same LAN
          ┌────────┴────────┐
          ▼                 ▼
┌─────────────────┐  ┌─────────────────┐
│  SemSpec flow   │  │  SemDragon flow │
│                 │  │                 │
│ input/websocket │  │ input/websocket │
│  (ModeClient)   │  │  (ModeClient)   │
│       │         │  │       │         │
│ Federation      │  │ Federation      │
│ Processor       │  │ Processor       │
│       │         │  │       │         │
│ graph ingestion │  │ board pre-seed  │
└─────────────────┘  └─────────────────┘
```

### 8.4 Transport Swappability

The `FederationProcessor` sits between transport and consumer in every flow. Swapping transport means changing only the input component — the processor and everything downstream is untouched.

```
# Today: WebSocket (same LAN)
input/websocket → FederationProcessor → graph ingestion

# Tomorrow: MQTT (government network)
input/mqtt → FederationProcessor → graph ingestion

# Air-gap: File handoff
input/file → FederationProcessor → graph ingestion
```

This is why the MQTT request from the government client is a config change, not an architectural change.

---

## 9. Bootstrap Flow

### 9.1 Target UX (post-MVP)

North star. MVP uses a config file but the cognitive model should match this.

```
$ semspec init
> GitHub repo?   github.com/acme/gcs
> Docs/URLs?     docs.acme.io/gcs  (optional)
> Anything else? (enter to skip)

[SemSource] Indexing AST (Go)...
[SemSource] Processing git history...
[SemSource] Fetching docs...
[SemSource] Emitting SEED (1,847 entities, 4,203 edges)
[SemSource] Watching for changes...

[SemSpec]    Graph seeded. Ready.
[SemDragon]  Board pre-seeded. Ready.
```

### 9.2 MVP Config

```yaml
# semsource.yaml
namespace: acme

flow:
  outputs:
    - name: graph_stream
      type: network
      subject: http://0.0.0.0:7890/graph
  delivery_mode: at-least-once
  ack_timeout: 5s

sources:
  - type: git
    url: github.com/acme/gcs
    branch: main
    watch: true

  - type: ast
    path: ./
    language: go
    watch: true

  - type: docs
    paths:
      - README.md
      - docs/
    watch: true

  - type: config
    paths:
      - go.mod
      - Dockerfile
    watch: true

  - type: url
    urls:
      - https://docs.acme.io/gcs
    poll_interval: 300s
```

### 9.3 Consumer Flow Config

```yaml
# semspec flow config (same pattern for semdragons)
flow:
  inputs:
    - name: graph_in
      type: websocket
      mode: client
      url: ws://semsource-host:7890/graph
      reconnect:
        enabled: true
        max_retries: 10
        initial_interval: 1s
        max_interval: 30s

  processors:
    - type: federation
      namespace: acme
      merge_policy: standard

  # ... rest of semspec flow
```

---

## 10. Milestones

### M1 — Repo & Scaffold
- [ ] Create `semsource` repository
- [ ] Establish semstreams dependency
- [ ] Define `GraphEntity`, `GraphEdge`, `GraphEvent` types
- [ ] Implement `SourceHandler` interface including `Watch()`
- [ ] Wire YAML config loader
- [ ] Wire `output/websocket` as flow output

### M2 — Core Handlers
- [ ] `GitHandler`: clone, commit history, file-touch graph, `fsnotify`/polling watch
- [ ] `ASTHandler`: import existing SemSpec indexer, wire to handler interface, `fsnotify` watch
- [ ] `DocHandler`: markdown + plain text ingestion, `fsnotify` watch
- [ ] `ConfigHandler`: go.mod, package.json parsing, `fsnotify` watch
- [ ] `URLHandler`: HTTP/S fetch, content hash change detection, poll interval

### M3 — Graph Normalization & Events
- [ ] Deterministic entity ID construction per Section 4.3
- [ ] `public.*` vs `{org}.*` namespace routing
- [ ] SEED event on initial ingest complete
- [ ] DELTA event emission from watch triggers
- [ ] RETRACT event on file deletion / symbol removal
- [ ] HEARTBEAT on configurable quiet interval
- [ ] Re-SEED on consumer reconnect

### M4 — FederationProcessor
- [ ] `FederationProcessor` in `semstreams/processor/federation.go`
- [ ] Merge policy implementation (`public.*` unconditional, `{org}.*` sovereign)
- [ ] Edge union semantics
- [ ] Provenance append
- [ ] Unit tests for each merge rule

### M5 — Parallel Consumer Validation
- [ ] SemSpec consumer flow: WebSocket Input (ModeClient) → FederationProcessor → graph ingestion
- [ ] SemDragon consumer flow: WebSocket Input (ModeClient) → FederationProcessor → board pre-seed
- [ ] Both consumers running simultaneously from one SemSource instance
- [ ] Verify no cross-consumer interference
- [ ] Live-change test: file edit → DELTA event → both consumers update correctly

### M6 — Federation Validation
- [ ] Two independent SemSource instances, same source repo
- [ ] Verify identical `public.*` entity IDs across instances
- [ ] Verify clean merge on consumer side without duplication
- [ ] Multiple SemSource instances → single consumer flow with multiple WebSocket inputs

---

## 11. Open Questions

1. **SEED replay size** — For large repos the SEED event could be substantial. Should SEED be chunked into a stream of partial payloads rather than one large envelope? Chunking adds complexity but may be necessary for repos with 10k+ entities.

2. **Versioned symbol identity** — Same symbol, different module versions: one entity with a version attribute, or two distinct nodes? Two nodes preserves full history; one mutable node is simpler to query. Affects the ID construction algorithm.

3. **URLHandler crawl scope** — Max depth and domain allowlist to prevent runaway ingestion of doc sites. Configurable per source entry is the obvious approach but needs defaults.

4. **PURL adoption** — Should the `instance` segment of `public.*` code entities adopt the Package URL (purl) standard for SPDX / CycloneDX interoperability? purl syntax may conflict with NATS KV key character constraints — needs investigation.

5. **`FederationProcessor` placement** — Currently specified as a semstreams processor in each consumer flow. Alternative: a standalone federation flow that receives from one or more SemSource instances and fans out to consumer-specific NATS subjects. The standalone flow approach centralizes merge logic but adds a hop. Current design (per-consumer processor) is simpler and sufficient for MVP.

6. **ASTHandler dependency boundary** — Importing the SemSpec AST indexer creates a dependency from semsource → semspec. If the indexer is valuable to both, it should live in a shared package (e.g. `semstreams/ast` or a standalone `semast` repo). Worth resolving before M2.

---

## Appendix — Vocabulary References

- **Package URL (purl)** — canonical identity for software components across ecosystems. `github.com/package-url/purl-spec`
- **SPDX 3.0** — element identity model for federated software bill-of-materials. `spdx.github.io/spdx-spec`
- **RFC 8141** — Uniform Resource Names: persistent, location-independent identifiers. `rfc-editor.org/rfc/rfc8141`
- **fsnotify** — cross-platform filesystem notification for Go. `github.com/fsnotify/fsnotify`
- **semstreams WebSocket Output** — `output/websocket`: server mode, multi-client broadcast, ack/nack protocol
- **semstreams WebSocket Input** — `input/websocket`: `ModeClient`, reconnect with backoff
- **Platform Entity ID Spec** — internal: 6-part hierarchical format, NATS KV key conventions
- **semstreams Component Patterns** — internal: Listen → Process → Persist → Publish model
