# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SemSource is the source-knowledge ingestion service for the C360 sem* family. It
ingests repos, docs, URLs, config files, and media into governed SemStreams graph
facts, then exposes source/query/status surfaces for agents and operator UIs. The
raw WebSocket export still exists for stream-oriented consumers, but the primary
read contracts are graph query, MCP, HTTP status, and GraphQL through the UI
profile.

Part of the Complete 360 Studio ecosystem. MIT licensed.

## Spec-driven development (OpenSpec)

SemSource uses **OpenSpec** for non-trivial work. The CLI and Claude integration
are installed — slash commands `/opsx:new`, `/opsx:continue`, `/opsx:apply`,
`/opsx:verify`, `/opsx:archive` (and the backing `openspec-*` skills); plus
`openspec list` / `openspec validate`. Three homes, three jobs — put a thing in
the right one:

| Home                                  | Holds                                                                                        | Drifts?                                |
| ------------------------------------- | -------------------------------------------------------------------------------------------- | -------------------------------------- |
| `openspec/specs/<capability>/spec.md` | **Current truth** — what a capability does _today_ (`Requirement` + `GIVEN/WHEN/THEN`)       | No — every change edits it via a delta |
| `openspec/changes/<id>/`              | **Proposed target state** — `proposal.md` + `tasks.md` + spec deltas; archived on completion | Resolves on archive                    |
| `docs/adr/`                           | **Genuine decisions only** — irreversible choices + cross-repo contracts (the _why_)         | No — history                           |

Rules of the road:

- **Non-trivial or cross-cutting work starts with a change** (`/opsx:new`):
  proposal + tasks + spec deltas _before_ code. Small mechanical fixes don't need one.
- **Specs are seeded lazily** — write a capability's spec when a change first
  touches it, distilled from code + existing docs and **verified against code**.
  Do NOT backfill; an unverified spec is just another drifting doc.
- **ADRs are pure decision records** — record an irreversible/cross-repo decision
  as a one-page ADR; the _mechanics_ it implies live in the capability's spec.
- **Read `openspec/config.yaml`'s `context:` first** when scoping anything — it
  carries the Purpose and the **Product Boundary** (SemSource owns source
  ingestion, not the SemStreams substrate) plus the per-artifact rules and
  non-negotiables shown to the tool. (OpenSpec 1.5 folded the former
  `openspec/project.md` into this context.)

## Technology

- **Language:** Go
- **Platform dependency:** semstreams (governed entity state, component/service APIs, and NATS
  JetStream infrastructure via `github.com/c360studio/semstreams`).
- **Transport:** SemStreams ServiceManager over NATS JetStream/KV, HTTP/MCP status
  and tool routes, GraphQL via the UI profile, plus the raw WebSocket graph stream
  (`output/websocket`) on port 7890 from the external service.
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
[semsource.json] -> [Source Intake] -> [Handler Dispatch] -> [Entity Publisher]
    -> [SemStreams graph-ingest / ENTITY_STATES] -> [graph query / MCP / GraphQL]
```

Follows the semstreams pattern: **Listen → Process → Persist → Publish**.

### Source Handlers

All handlers implement the `SourceHandler` interface (`Ingest`, `Watch`, `Supports`, `SourceType`).

| Handler         | Sources                                | Watch Mechanism      |
| --------------- | -------------------------------------- | -------------------- |
| `GitHandler`    | Local/remote repos                     | git hook / polling   |
| `ASTHandler`    | Go, TS/JS (reuses SemSpec AST indexer) | fsnotify             |
| `DocHandler`    | Markdown, plain text                   | fsnotify             |
| `ConfigHandler` | go.mod, package.json, Dockerfile       | fsnotify             |
| `URLHandler`    | HTTP/S URLs                            | configurable polling |

### Entity Identity (6-Part ID)

```
{org}.{platform}.{domain}.{system}.{type}.{instance}
```

Example: `acme.semsource.golang.github.com-acme-gcs.function.NewController`

- `public.*` namespace: deterministic IDs for open-source entities, merge unconditionally across instances
- `{org}.*` namespace: sovereign to the owning org

IDs must be purely intrinsic (no timestamps, instance IDs, or insertion-order). All IDs must be valid NATS KV keys.

### ID Construction by Entity Type

| Entity Type | Construction                                                                     |
| ----------- | -------------------------------------------------------------------------------- |
| Code symbol | `org + semsource + language + canonical_module_path + symbol_type + symbol_name` |
| Git commit  | `org + semsource + git + repo_slug + commit + short_sha`                         |
| URL / doc   | `org + semsource + web + domain_slug + doc + sha256(canonical_url)[:6]`          |
| Config file | `org + semsource + config + repo_slug + file_type + sha256(content)[:6]`         |

### Event Types

- **SEED** — full graph on start and consumer reconnect
- **DELTA** — incremental upserts from watch events
- **RETRACT** — explicit entity removal
- **HEARTBEAT** — liveness signal during quiet periods

### Governed graph integration

SemSource publishes governed entity state through the SemStreams substrate. Consumers query the
result through `graph.query.*`, HTTP/MCP, or GraphQL; they do not register a federation processor or
bridge SemSource's raw event stream into consumer-owned storage.

## semstreams Component Patterns

New components must follow the semstreams component structure. Use
`semstreams/processor/ast-indexer/` as the canonical reference implementation.

The AST source accepts only complete `watch_paths` entries; component JSON is decoded strictly:

```json
{
  "watch_paths": [
    {
      "path": "./service",
      "org": "acme",
      "project": "service",
      "languages": ["go"],
      "excludes": ["vendor"]
    }
  ],
  "watch_enabled": true
}
```

Move the former top-level `repo_path`, `org`, `project`, `version`, `languages`, and
`exclude_patterns` values into each `watch_paths` entry (`exclude_patterns` becomes `excludes`).

### Required Files per Component

- **config.go** — Config struct with `json` + `schema` tags, `Validate()`, `DefaultConfig()`
- **component.go** — Implements `component.Discoverable` interface (Meta, InputPorts, OutputPorts, ConfigSchema, Health, DataFlow)
- **factory.go** — `Register()` with full registration config (Name, Factory, Schema, Type, Protocol, Domain, Description, Version)
- **payloads.go** — Payload registered in `init()` via `component.RegisterPayload`; Domain/Category/Version must match between `init()` and `Schema()`

### Payload Registration

New message types must follow the payload registry pattern: define type → implement `MarshalJSON` wrapping in `BaseMessage` → register in `init()` → blank-import in entry point. Use `/new-payload` skill for the full checklist.

## Key Design Decisions

- No separate batch mode — initial seeding is the first pass of the continuous event loop
- Raw-stream fan-out via WebSocket output remains available from the external service
- `at-least-once` delivery using WebSocket ack/nack protocol
- MVP targets same-LAN deployment only (no TLS/reverse proxy)
- AST parsers, doc parsers, weburl, and vocabulary packages are self-contained in `source/` (copied from semspec, no cross-repo dependency)
- Consumers query SemSource via NATS `graph.query.*` endpoints — no WebSocket ingestion or federation bridge needed
- SemStreams owns governed graph state and mutation/query contracts; SemSource owns source ingestion and projection
- The `graph/` package contains SemSource's domain-specific payload and graph integration helpers
- Status gating: consumers wait for `graph.query.status` → `phase: "ready"` before querying

## Current Roadmap

The public roadmap lives in `ROADMAP.md`; use that instead of the historical
milestone list in `docs/spec/semsource-spec-v3.md`.

Current release-candidate shape (the latest public tag is still beta.4):

1. `v1.0.0-beta.4` targets SemStreams `v1.0.0-beta.144`.
2. Core ingestion, governed entity publishing, source manifest/status, fusion
   tools, version diffs, and consumer query integration are present.
3. The default Compose profile is UI-free: `docker compose up` resolves only
   NATS, semembed, and SemSource. In deployment documentation, "headless" means
   this omitted workbench profile; it is not the removed `mode: "headless"`
   configuration value, which must continue to fail validation.
4. The opt-in `ui` profile now belongs to SemSource and serves the local `ui/`
   workbench. This is a breaking replacement for the former sibling SemTeams UI
   mapping; SemTeams owns its packaging and consumes unchanged SemSource HTTP,
   MCP, NATS, GraphQL, and governed graph contracts.
5. Release use requires `SEMSOURCE_UI_IMAGE=<tag>@sha256:<digest>` and no sibling
   checkout or host Node.js. Local development is explicit through
   `docker-compose.ui-dev.yml` or `task ui:smoke:dev`; `task ui:e2e` uses the
   lockfile-matched container runner.
6. The first published immutable workbench digest is still OpenSpec task 7.3.
   CI now contract-tests PR quality/clean-image gates, trusted main/tag publication,
   exact-manifest verification, and released-profile pin proof, but no real registry
   digest/run evidence exists yet. Until it does, released-profile compatibility is
   not proven. Graph drill-down uses the adopted SemStreams #533 facet in
   `v1.0.0-beta.153` through the existing code-context route, with local and
   real-profile acceptance; GraphQL is not part of that slice.
7. Active follow-ups are the workbench release pin, query-index
   readiness/scale, GraphQL capabilities alignment, code/version intelligence,
   and federation validation.

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
