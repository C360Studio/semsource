# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SemSource is a graph-first knowledge ingestion service for the SemStream platform. It ingests heterogeneous sources (code repos, docs, URLs, config files) and emits a normalized, continuously updated knowledge graph stream via WebSocket broadcast to downstream consumers (SemSpec, SemDragon).

Part of the Complete 360 Studio ecosystem. MIT licensed.

## Technology

- **Language:** Go
- **Platform dependency:** semstreams (NATS JetStream infrastructure, `github.com/c360studio/semstreams`)
- **Transport:** WebSocket server (output/websocket from semstreams) on port 7890
- **Config format:** YAML (`semsource.yaml`)

## Build & Test Commands

```bash
go build ./...
go test ./...
go test -run TestName ./path/to/package   # single test
go test -race ./...                        # race detection
go test -coverprofile=coverage.out ./...   # coverage
```

## Architecture

### Component Flow

```
[semsource.yaml] → [Source Intake] → [Handler Dispatch] → [Graph Normalizer] → [output/websocket]
```

Follows the semstreams pattern: **Listen → Process → Persist → Publish**.

### Source Handlers

All handlers implement the `SourceHandler` interface (`Ingest`, `Watch`, `Supports`, `SourceType`).

| Handler | Sources | Watch Mechanism |
|---------|---------|-----------------|
| `GitHandler` | Local/remote repos | git hook / polling |
| `ASTHandler` | Go, TS/JS (reuses SemSpec AST indexer) | fsnotify |
| `DocHandler` | Markdown, plain text | fsnotify |
| `ConfigHandler` | go.mod, package.json, Dockerfile | fsnotify |
| `URLHandler` | HTTP/S URLs | configurable polling |

### Entity Identity (6-Part ID)

```
{org}.{platform}.{domain}.{system}.{type}.{instance}
```

Example: `acme.semsource.golang.github.com-acme-gcs.function.NewController`

- `public.*` namespace: deterministic IDs for open-source entities, merge unconditionally across instances
- `{org}.*` namespace: sovereign to the owning org

IDs must be purely intrinsic (no timestamps, instance IDs, or insertion-order). All IDs must be valid NATS KV keys.

### ID Construction by Entity Type

| Entity Type | Construction |
|------------|--------------|
| Code symbol | `org + semsource + language + canonical_module_path + symbol_type + symbol_name` |
| Git commit | `org + semsource + git + repo_slug + commit + short_sha` |
| URL / doc | `org + semsource + web + domain_slug + doc + sha256(canonical_url)[:6]` |
| Config file | `org + semsource + config + repo_slug + file_type + sha256(content)[:6]` |

### Event Types

- **SEED** — full graph on start and consumer reconnect
- **DELTA** — incremental upserts from watch events
- **RETRACT** — explicit entity removal
- **HEARTBEAT** — liveness signal during quiet periods

### Federation

`FederationProcessor` sits in each consumer flow between WebSocket input and graph ingestion. Applies merge policy: `public.*` merges unconditionally, `{org}.*` is sovereign. Transport-agnostic.

## semstreams Component Patterns

New components must follow the semstreams component structure. Use `semstreams/processor/ast-indexer/` as the canonical reference implementation.

### Required Files per Component

- **config.go** — Config struct with `json` + `schema` tags, `Validate()`, `DefaultConfig()`
- **component.go** — Implements `component.Discoverable` interface (Meta, InputPorts, OutputPorts, ConfigSchema, Health, DataFlow)
- **factory.go** — `Register()` with full registration config (Name, Factory, Schema, Type, Protocol, Domain, Description, Version)
- **payloads.go** — Payload registered in `init()` via `component.RegisterPayload`; Domain/Category/Version must match between `init()` and `Schema()`

### Payload Registration

New message types must follow the payload registry pattern: define type → implement `MarshalJSON` wrapping in `BaseMessage` → register in `init()` → blank-import in entry point. Use `/new-payload` skill for the full checklist.

## Key Design Decisions

- No separate batch mode — initial seeding is the first pass of the continuous event loop
- Fan-out via WebSocket output's built-in multi-client broadcast
- `at-least-once` delivery using WebSocket ack/nack protocol
- MVP targets same-LAN deployment only (no TLS/reverse proxy)
- ASTHandler imports existing SemSpec indexer as dependency (may move to shared package)
- `FederationProcessor` lives in each consumer flow (not centralized) — simpler, sufficient for MVP

## Development Milestones

Current roadmap (see spec Section 10 for full details):

1. **M1 — Repo & Scaffold**: semstreams dependency, core types (`GraphEntity`, `GraphEdge`, `GraphEvent`), `SourceHandler` interface, YAML config loader, output/websocket wiring
2. **M2 — Core Handlers**: GitHandler, ASTHandler, DocHandler, ConfigHandler, URLHandler
3. **M3 — Graph Normalization & Events**: Deterministic entity IDs, namespace routing, SEED/DELTA/RETRACT/HEARTBEAT emission
4. **M4 — FederationProcessor**: Merge policy, edge union semantics, provenance append
5. **M5 — Parallel Consumer Validation**: SemSpec + SemDragon consuming simultaneously
6. **M6 — Federation Validation**: Multiple SemSource instances producing identical `public.*` IDs

## Custom Agents & Skills

### Review Agents (`.claude/agents/`)

- **go-component-reviewer** — Reviews semstreams component implementations against the full component checklist (config tags, Discoverable interface, factory registration, payload registry, NATS usage, entity identity)
- **graph-event-reviewer** — Reviews entity identity construction, event semantics, federation merge behavior, and watch/real-time correctness

### Skills (`.claude/skills/`)

- `/new-payload` — Step-by-step checklist for adding a new payload type to the registry
- `/orchestration-check` — Determine if logic belongs in a reactive rule, workflow, or component
- `/kv-or-stream` — Decide between KV Watch and JetStream Stream for a communication path
- `/query-pattern` — Choose between GraphQL, MCP, or NATS Direct for a query use case

## Spec Reference

Full specification: `docs/spec/semsource-spec-v3.md`
