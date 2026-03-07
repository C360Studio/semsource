# M5 Consumer Integration Guide

Instructions for connecting SemSpec and SemDragon to SemSource's graph event stream.

## Overview

SemSource emits `GraphEvent` payloads (SEED/DELTA/RETRACT/HEARTBEAT) via `output/websocket` on `:7890/graph`. Each consumer needs to:

1. Add `input/websocket` in ModeClient connecting to semsource
2. Route received messages through the `FederationProcessor` from `semstreams/processor/federation` (merge policy filter)
3. Feed the filtered entities into their existing `graph-ingest` pipeline -> `ENTITY_STATES` KV

The federation processor and its shared types (`federation.Entity`, `federation.Event`, etc.) live in `semstreams/federation` and `semstreams/processor/federation`. **SemSource does not ship its own federation processor** — it is a consumer-side concern.

## Wire Format

SemSource emits events as JSON. The wire format is structurally identical to `federation.Event`:

```json
{
  "type": "SEED|DELTA|RETRACT|HEARTBEAT",
  "source_id": "semsource",
  "namespace": "acme",
  "timestamp": "2026-03-07T...",
  "entities": [...],
  "retractions": [...],
  "provenance": { "source_type": "engine", "source_id": "semsource", ... }
}
```

Consumers unmarshal directly into `federation.Event`. Entity fields (`id`, `triples`, `edges`, `provenance`, `additional_provenance`) map 1:1 to `federation.Entity`.

## SemSpec Integration

### What exists today

- `source-ingester`, `repo-ingester`, `web-ingester`, `ast-indexer` all publish to `graph.ingest.entity` (JetStream subject on `GRAPH` stream)
- `graph-ingest` component subscribes to `graph.ingest.entity` -> writes to `ENTITY_STATES` KV
- No WebSocket input currently configured

### What to add

#### 1. Register the FederationProcessor

In whatever file semspec registers components (look for existing `Register()` calls), add:

```go
import "github.com/c360studio/semstreams/processor/federation"
federation.Register(registry)
```

Also register the federation event payload for semspec's domain:

```go
import fedtypes "github.com/c360studio/semstreams/federation"
fedtypes.RegisterPayload("semspec")
```

#### 2. Add two components to the flow config JSON

Add to `configs/e2e.json` (and other deployment configs):

**WebSocket input (ModeClient):**

```json
"semsource-input": {
  "name": "semsource-input",
  "type": "input",
  "enabled": true,
  "config": {
    "mode": "client",
    "client": {
      "url": "ws://localhost:7890/graph",
      "reconnect": {
        "enabled": true,
        "max_retries": 0,
        "initial_interval": "1s",
        "max_interval": "60s",
        "multiplier": 2.0
      }
    },
    "ports": {
      "inputs": [],
      "outputs": [
        {
          "name": "ws_data",
          "type": "jetstream",
          "subject": "semsource.graph.events",
          "stream_name": "GRAPH_EVENTS"
        }
      ]
    }
  }
}
```

**FederationProcessor:**

```json
"federation-processor": {
  "name": "federation-processor",
  "type": "processor",
  "enabled": true,
  "config": {
    "local_namespace": "acme",
    "merge_policy": "standard"
  }
}
```

The federation processor consumes from `semsource.graph.events` (GRAPH_EVENTS stream) and publishes merged events to `semsource.graph.merged` (GRAPH_MERGED stream).

#### 3. Bridge federation output to graph-ingest

The federation processor outputs merged `federation.Event` payloads to `semsource.graph.merged`. These need to be converted to individual entity publishes on `graph.ingest.entity` so the existing `graph-ingest` component picks them up.

Use `federation.ToEntityState()` to convert each `federation.Entity` to a `graph.EntityState` for storage:

```go
import fedtypes "github.com/c360studio/semstreams/federation"

for _, entity := range event.Entities {
    state := fedtypes.ToEntityState(entity, fedtypes.NewFederationMessageType())
    // publish state to graph.ingest.entity
}
```

Two options for the bridge:

**Option A (recommended): Small bridge processor.** A lightweight processor that subscribes to `semsource.graph.merged`, unwraps the `federation.Event`, and for each entity uses `ToEntityState()` to publish to `graph.ingest.entity`. For RETRACT events, publish delete messages. This keeps `graph-ingest` unchanged.

**Option B: Add a second input to graph-ingest.** Modify `graph-ingest` to also subscribe to `semsource.graph.merged` and handle `federation.Event` payloads directly. More invasive but eliminates the bridge.

#### 4. SemSpec's own processors continue working independently

The `source-ingester`, `ast-indexer`, `repo-ingester`, and `web-ingester` still publish directly to `graph.ingest.entity`. SemSource provides _additional_ external graph entities via the WebSocket -> federation -> bridge -> graph-ingest path. Both feed into the same `ENTITY_STATES` KV. The 6-part entity IDs ensure no collisions.

### SemSpec data flow after integration

```
[semsource :7890/graph]
    | WebSocket
[input/websocket ModeClient]
    | semsource.graph.events
[federation-processor] (merge policy filter)
    | semsource.graph.merged
[bridge processor] (federation.ToEntityState -> individual entities)
    | graph.ingest.entity
[graph-ingest] -> ENTITY_STATES KV
```

## SemDragon Integration

### What exists today

- `seeding` processor bootstraps agents/guilds into board KV
- `graph-ingest` component writes to `ENTITY_STATES` KV
- No WebSocket input currently configured
- SemDragon relies entirely on external sources (semsource) for source graph entities

### What to add

#### 1. Register the FederationProcessor

Same as semspec:

```go
import "github.com/c360studio/semstreams/processor/federation"
federation.Register(registry)

import fedtypes "github.com/c360studio/semstreams/federation"
fedtypes.RegisterPayload("semdragon")
```

#### 2. Add WebSocket input + FederationProcessor to flow config

Add to `config/semdragons-e2e-openai.json` (and other deployment configs). Same two component configs as semspec above, but with the `local_namespace` matching semdragon's org namespace.

#### 3. Bridge federation output to graph-ingest

Same pattern as semspec. The bridge processor uses `federation.ToEntityState()` to unwrap `federation.Event` entities into individual `graph.EntityState` publishes on `graph.ingest.entity`.

#### 4. Board pre-seeding from external graph

The `seeding` processor currently bootstraps agents/guilds. After integration, the ENTITY_STATES KV will also contain source-graph entities from semsource (code symbols, git commits, docs, config files, URLs). The seeding processor doesn't need to change -- it operates on its own domain entities. But any downstream processors that query the graph (quest tools, context builders, etc.) will now see semsource entities in ENTITY_STATES and can use them.

### SemDragon data flow after integration

```
[semsource :7890/graph]
    | WebSocket
[input/websocket ModeClient]
    | semsource.graph.events
[federation-processor] (merge policy filter)
    | semsource.graph.merged
[bridge processor] (federation.ToEntityState)
    | graph.ingest.entity
[graph-ingest] -> ENTITY_STATES KV
    |
[seeding, quest tools, etc.] -- can now query source entities
```

## Shared Deliverables (Both Teams)

### Bridge processor

Both teams need the same small processor. Consider putting it in `semstreams` as a shared component (`processor/graph-event-bridge/`) since both consumers need it, or each team can implement their own. The logic is ~50 lines using `federation.ToEntityState()`:

- Subscribe to `semsource.graph.merged`
- For SEED/DELTA: iterate `event.Entities`, call `federation.ToEntityState(entity, federation.NewFederationMessageType())`, publish to `graph.ingest.entity`
- For RETRACT: iterate `event.Retractions`, publish delete messages to `graph.ingest.entity`
- For HEARTBEAT: no-op (or forward as a liveness signal)

### JetStream streams

Both consumers need `GRAPH_EVENTS` and `GRAPH_MERGED` streams created. Add to NATS stream provisioning (usually in the flow config or startup scripts).

### Integration test

Start semsource with a test YAML config, start the consumer with the new flow config, verify entities appear in `ENTITY_STATES` KV after semsource emits a SEED event.
