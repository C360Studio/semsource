# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Project Overview

SemSource is a graph-first knowledge ingestion service for the SemStream platform. It ingests heterogeneous
sources (code repos, docs, URLs, config files, and media metadata) and publishes governed entity-state payloads
into the SemStreams graph stack for downstream consumers such as SemSpec, SemDragon, and SemOps.

Part of the Complete 360 Studio ecosystem. MIT licensed.

## Technology

- **Language:** Go
- **Platform dependency:** semstreams governed graph model (`github.com/c360studio/semstreams`), including NATS
  JetStream/KV, owner claims, graph-ingest, graph-index, graph-query, and GraphQL gateway processors.
- **Transport:** NATS subjects for `graph.ingest.entity` and `graph.query.*`, GraphQL gateway on `:8082`, service
  manager/status endpoints on `:8080`, and legacy raw WebSocket export on `:7890` where explicitly enabled.
- **Config format:** JSON (`semsource.json`)

## CLI Commands

```bash
semsource init              # Interactive setup wizard → writes semsource.json
semsource run               # Start the ingestion engine
semsource add [type]        # Add a source (interactive or with flags)
semsource remove            # Remove a source (interactive or --index N)
semsource sources           # List configured sources
semsource validate          # Check config without starting
semsource version           # Print version
```

Non-interactive examples:
- `semsource add ast --path ./src --language go --watch`
- `semsource remove --index 2`

Bare `semsource` with no args auto-runs if `semsource.json` exists.

## Build & Test Commands

```bash
go build ./...
go test ./...                              # unit tests only
go test -tags=integration ./...            # include integration tests
go test -tags=e2e ./test/e2e/              # black-box binary tests
go test -run TestName ./path/to/package    # single test
go test -race -tags=integration ./...      # race detection
```

## Architecture

### Component Flow

```
[semsource.json] → [Source Intake] → [Handler Dispatch] → [EntityPayload] → [graph.ingest.entity]
    → [graph-ingest / ENTITY_STATES] → [graph-index] → [graph.query.* / GraphQL]
```

Follows the semstreams pattern: **Listen → Process → Persist → Query**.

### Source Handlers

All handlers implement the `SourceHandler` interface (`Ingest`, `Watch`, `Supports`, `SourceType`).

| Handler | Sources | Watch Mechanism |
|---------|---------|-----------------|
| `GitHandler` | Local/remote repos | git hook / polling |
| `ASTHandler` | Go, TS/JS (reuses SemSpec AST indexer) | fsnotify |
| `DocHandler` | Markdown, plain text | fsnotify |
| `ConfigHandler` | go.mod, package.json, Dockerfile | fsnotify |
| `URLHandler` | HTTP/S URLs | configurable polling |
| `ImageHandler` / `VideoHandler` / `AudioHandler` | media metadata and storage references | fsnotify |

### Entity Identity (6-Part ID)

```
{org}.{platform}.{domain}.{system}.{type}.{instance}
```

Example: `acme.semsource.golang.github.com-acme-gcs.function.NewController`

- `public.*` namespace: deterministic IDs for open-source entities
- `{org}.*` namespace: sovereign to the owning org

IDs must be purely intrinsic (no timestamps, instance IDs, or insertion-order). All IDs must be valid NATS KV keys.

### ID Construction by Entity Type

| Entity Type | Construction |
|------------|--------------|
| Code symbol | `org + semsource + language + canonical_module_path + symbol_type + symbol_name` |
| Git commit | `org + semsource + git + repo_slug + commit + short_sha` |
| URL / doc | `org + semsource + web + domain_slug + doc + sha256(canonical_url)[:6]` |
| Config file | `org + semsource + config + repo_slug + file_type + sha256(content)[:6]` |
| Media asset | `org + semsource + media + source_slug + media_type + sha256(path/content)[:12]` |

### Governed Entity Payloads

SemSource publishes `semsource.entity.v1` payloads with:

- self-subject triples only
- a valid six-part entity ID
- a required indexing profile hint: `content`, `control`, or `trace`
- optional `message.StorageReference` for binary-by-reference media storage

Raw WebSocket SEED/DELTA/RETRACT/HEARTBEAT exports are legacy compatibility behavior and are not the primary
consumer contract for governed graph integrations.

### Governance

SemSource owns source-ingestion predicates through SemStreams owner claims. Standalone mode bootstraps
`OWNER_CLAIMS` and `OWNER_PRESENCE` before graph ingest starts. Headless/embedded mode expects the host platform
to own governance bootstrap.

Ownership is exact-predicate only. Do not use wildcard predicate ownership for SemSource until SemStreams exposes
and tests that contract upstream.

Consumers query SemSource's graph through NATS `graph.query.*` endpoints or the GraphQL gateway. No federation
bridge is required on the consumer side.

## semstreams Component Patterns

New components must follow the semstreams component structure. Use `semstreams/processor/ast-indexer/` as the
canonical reference implementation.

### Required Files per Component

- **config.go** — Config struct with `json` + `schema` tags, `Validate()`, `DefaultConfig()`
- **component.go** — Implements `component.Discoverable` interface (Meta, InputPorts, OutputPorts, ConfigSchema,
  Health, DataFlow)
- **factory.go** — `Register()` with full registration config (Name, Factory, Schema, Type, Protocol, Domain,
  Description, Version)
- **payloads.go** — Payload registered in `init()` via `component.RegisterPayload`; Domain/Category/Version must
  match between `init()` and `Schema()`

### Payload Registration

New message types must follow the payload registry pattern: define type → implement `MarshalJSON` wrapping in
`BaseMessage` → register in `init()` → blank-import in entry point. Use `/new-payload` skill for the full checklist.

## Key Design Decisions

- No separate batch mode — initial seeding is the first pass of the continuous event loop
- SemSource is a source-of-truth writer into the governed graph, not a consumer-side federation shim
- Binary media is stored by reference; graph triples carry governed metadata and storage refs, not raw blobs
- KLV, MISB ST 0601, STANAG 4609, SAPIENT, SKG, streaming-binary behavior, protocol parsing, and protocol
  conformance are SemOps/product concerns, not SemSource migration claims
- MVP targets same-LAN deployment only (no TLS/reverse proxy)
- AST parsers, doc parsers, weburl, and vocabulary packages are self-contained in `source/` (copied from semspec,
  no cross-repo dependency)
- Consumers query SemSource via NATS `graph.query.*` endpoints — no WebSocket ingestion or federation bridge needed
- Graph payloads use SemStreams `message.Triple` and `message.StorageReference`; `graph.EntityPayload` is the
  SemSource domain payload registered as `semsource.entity.v1`
- Status gating: consumers wait for `graph.query.status` → `phase: "ready"` before querying

## Development Milestones

Current roadmap (see spec Section 10 for full details):

1. **M1 — Repo & Scaffold** ✅: semstreams dependency, `SourceHandler` interface, JSON config loader, service wiring
2. **M2 — Core Handlers** ✅: GitHandler, ASTHandler, DocHandler, ConfigHandler, URLHandler
3. **M3 — Graph Normalization** ✅: Deterministic entity IDs, namespace routing, self-subject triples
4. **M4 — Governed Graph Adoption** ✅: owner claims, graph-ingest, graph-index, graph-query, indexing profiles
5. **M5 — Consumer Query Integration** ✅: status gating, NATS query endpoints, GraphQL gateway. Integration guide
   at `docs/integration/m5-consumer-integration.md`
6. **M6 — Binary Source Proof** ✅: synthetic binary-by-reference fixture for storage, governance, indexing, and
   memory-bound behavior only

## Custom Agents & Skills

### Review Agents (`.Codex/agents/`)

- **go-component-reviewer** — Reviews semstreams component implementations against the full component checklist
  (config tags, Discoverable interface, factory registration, payload registry, NATS usage, entity identity)
- **graph-event-reviewer** — Reviews entity identity construction, governed triples, owner/indexing behavior, and
  watch/real-time correctness

### Skills (`.Codex/skills/`)

- `/new-payload` — Step-by-step checklist for adding a new payload type to the registry
- `/orchestration-check` — Determine if logic belongs in a reactive rule, workflow, or component
- `/kv-or-stream` — Decide between KV Watch and JetStream Stream for a communication path
- `/query-pattern` — Choose between GraphQL, MCP, or NATS Direct for a query use case

## Spec Reference

Full specification: `docs/spec/semsource-spec-v3.md`
