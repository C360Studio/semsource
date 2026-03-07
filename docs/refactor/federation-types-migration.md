# Refactor: Adopt `semstreams/federation` Types — Complete

## What Changed

Replaced all semsource-local graph types with shared `semstreams/federation` types. The codebase now uses `federation.Entity`, `federation.Event`, `federation.Edge`, `federation.Provenance`, and `federation.Store` directly — no aliases, no local copies.

### Deleted Files
- `graph/types.go` — GraphEntity, GraphEdge, SourceProvenance structs (replaced by federation types)
- `graph/event.go` — GraphEvent, EventType, Validate (replaced by federation types)
- `graph/store.go` — Store with Upsert/Remove/Snapshot/Count (replaced by federation.Store)
- `graph/store_test.go` — redundant with semstreams federation store tests
- `processor/federation/` — entire directory (6 files), federation is consumer-side concern

### Kept
- `graph/event_payload.go` — `GraphEventPayload` is a concrete struct (not alias) because `federation.EventPayload.Schema()` returns `Domain: "federation"` while our payload registry requires `Domain: "semsource"`. The semstreams `RegisterPayload` function enforces Schema/registration domain consistency.
- `graph/event_test.go` — payload-specific tests (Schema domain, JSON round-trip, Validate delegation)

### Updated Files (import `federation` instead of `graph`)
- `engine/engine.go` — uses `federation.Store`, `federation.Entity`, `federation.Provenance`
- `engine/emitter.go` — `Emitter` interface operates on `*federation.Event`
- `engine/seed.go`, `delta.go`, `retract.go`, `heartbeat.go` — build `federation.Event` directly
- `engine/engine_test.go`, `delta_retract_test.go` — all assertions use `federation.*` types
- `normalizer/normalizer.go` — returns `*federation.Entity`, builds `federation.Edge`/`federation.Provenance`
- `normalizer/normalizer_test.go` — compile-time type assertion uses `*federation.Entity`

## Why

- Eliminates type duplication between semsource and semstreams
- Consumers (semspec, semdragon) unmarshal events directly into `federation.Event` — zero conversion
- `federation.Store` is strictly better (deep edge/properties clone, `Get()`, `SnapshotMap()`)
- The merge logic in `semstreams/processor/federation` operates on `federation.Entity` natively

## Payload Domain Note

If the semstreams team adds a configurable-domain variant of `EventPayload.Schema()`, the `graph/` package can be eliminated entirely. Until then, `GraphEventPayload` must remain a concrete struct to satisfy the `"semsource"` domain constraint in the payload registry.
