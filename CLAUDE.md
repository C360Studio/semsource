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
task tools:install                         # once: revive at the CI-pinned version
task lint                                  # vet + gofmt + revive (warnings fail)
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
| `ASTHandler`    | Go, TS/JS, Java, Python, Svelte        | fsnotify             |
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
- **payload_registry.go** — package-level `RegisterPayloads(reg *payloadregistry.Registry)` listing a `payloadregistry.Registration` per payload type; each payload's `Schema() message.Type` must match its registration's Domain/Category/Version

### Payload Registration

New message types must follow the payload registry pattern: define the type implementing `message.Payload` (`Schema()`, `Validate()`, alias-based `MarshalJSON`/`UnmarshalJSON`) → add it to the package's `RegisterPayloads(reg)` → wire that call into `buildPayloadRegistry()` in `cmd/semsource/run.go`. Registration is explicit at bootstrap — no `init()` side effects, no blank imports. Canonical references: `graph/event_payload.go`, `processor/source-manifest/payload_registry.go`. Use `/new-payload` skill for the full checklist.

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

Current release-candidate shape (the latest public tag is beta.9):

1. `v1.0.0-beta.9` is the onboarding release — the first **non-breaking**
   tag in the beta line (still SemStreams `v1.0.0-beta.161`; no fresh
   storage, no reseed). It closes v1-blocker #184: `docs/QUICKSTART.md` is
   executable truth — CI extracts its `quickstart:single|multi`-marked
   command blocks and runs them verbatim against the public semdev-test
   fixtures (`test/e2e` doc-driver; path-filtered Quickstart workflow), so
   doc drift fails a build. D5 recorded on #184: Docker Compose is the v1
   launcher; `add-one-action-local-start` deferred past v1. It also ships
   the shared status-report contract (#188: `internal/sourcestatus.Report`
   replaces nine duck-typed copies, strict aggregator decode; per-source
   `backpressure` + `boundaries_skipped` now on every status surface) and
   `semsource add ast|repo --project/--version` (#189). New capability
   `onboarding-quickstart`; modified `ingestion-readiness` and
   `cli-onboarding`.
2. `v1.0.0-beta.8` is the git-submodule release (#185, ADR-0012), still on
   SemStreams `v1.0.0-beta.161` (no substrate cutover). Repos with submodules
   ingest completely (clone/pull materialize declared trees at their gitlink
   pins, shallow-first with full-fetch fallback; `submodules: false` opts out),
   loudly (per-path states — materialized / unmaterialized /
   excluded_by_config / declared_but_absent / beyond_cap — on every status
   surface), and with canonical identity (project = resolved-URL slug,
   version = 12-hex gitlink SHA riding `WatchPathConfig.Version`; same pin
   from any parent → byte-identical entity IDs; instance names are
   parent+link-scoped). Ingest walks never cross nested git working trees
   (`internal/gitboundary`); expansion runs at the composition root
   (`internal/subwatch`, boot-configured sources only — runtime adds are
   loud-but-unexpanded). Breaking only for graphs that had accidentally
   ingested submodule code under parent identity (fresh reseed). Known
   upstream dependency: semstreams#986 (config-watcher burst race; SemSource
   defends with a confirm re-put).
3. `v1.0.0-beta.7` targeted SemStreams `v1.0.0-beta.161` — the post-beta.160 reliability and
   lifecycle-control slice on top of
   the intentionally breaking graph/foundation refactor checkpoint (no
   upstream shims; fresh NATS storage mandated on adoption, so deployments upgrade via
   `docker compose down -v` + reseed — the graph re-derives from source). beta.161 makes
   shutdown caller-owned-context: `LifecycleComponent.Stop(ctx)` replaces `Stop(timeout)`,
   `StopAll` takes a bounded context created at the composition root, and stored lifecycle
   contexts are prohibited. It also restores WebSocket path configurability
   (`websocket_path` is honored again; `/ws` stays the default contract path) and attributes
   slow-consumer errors to the responsible subscription. beta.7 adds the asset ingestion
   guards (minified assets never parse into symbols; per-file symbol cap) and typed
   publish-error classification (transients retry idempotently via `Nats-Msg-Id` inside a
   90s budget; measured zero-loss on broker-pause and stream-constriction inductions).
   Ports use the strict
   `PortDefinition` envelope (typed `config` with `kind`); the ownership substrate is deleted —
   SemSource declares local `projection.Contract` intent (named reconcile groups `source` +
   `lifecycle` in `graph/contract.go`) and supersession mutates through the typed CAS client;
   framework component configs defer to beta.160 DefaultConfigs (a declared `ports` section
   replaces defaults wholesale); readiness stays on the `GRAPH_STATUS` KV bucket via
   `OpenCatalogReader`. The `graph.query.searchGraph` and `graph.query.summary` operations
   SURVIVE the query-contract closure (the removals were aggregate Go wrappers and shared
   agentic tool registrations).
4. Core ingestion, governed entity publishing, source manifest/status, fusion
   tools, version diffs, and consumer query integration are present.
5. The default Compose profile is UI-free: `docker compose up` resolves only
   NATS, semembed, and SemSource. In deployment documentation, "headless" means
   this omitted workbench profile; it is not the removed `mode: "headless"`
   configuration value, which must continue to fail validation.
6. The opt-in `ui` profile now belongs to SemSource and serves the local `ui/`
   workbench. This is a breaking replacement for the former sibling SemTeams UI
   mapping; SemTeams owns its packaging and consumes unchanged SemSource HTTP,
   MCP, NATS, GraphQL, and governed graph contracts.
7. Release use requires `SEMSOURCE_UI_IMAGE=<tag>@sha256:<digest>` and no sibling
   checkout or host Node.js. Local development is explicit through
   `docker-compose.ui-dev.yml` or `task ui:smoke:dev`; `task ui:e2e` uses the
   lockfile-matched container runner.
8. OpenSpec task 7.3 is complete. Trusted `main` UI publish/smoke jobs verified
   revision `25b2816d14a147c1d6eb7b54e40668b51ba3574a` at manifest digest
   `sha256:43edacf62e7908681e7bedd193d1b18f3ebe8f3de438d417c6c091517020ea20`
   for `linux/amd64` and `linux/arm64`, including registry, local, Compose-rendered,
   and running-container pin proof in
   [Actions run 29693062800](https://github.com/C360Studio/semsource/actions/runs/29693062800),
   attempt 1. All six workflow jobs passed, including `build-and-push` and
   `ui-release-smoke`; the released browser profile passed 6/6. Graph drill-down
   uses the adopted SemStreams #533 facet in `v1.0.0-beta.153` through the existing
   code-context route, with local and real-profile acceptance; GraphQL is not part
   of that slice.
9. Active follow-ups are query-index readiness/scale, GraphQL capabilities
   alignment, code/version intelligence, and federation validation (#184
   easy-button onboarding closed in beta.9; both v1 blockers are done, and
   the remaining v1 gate is SemStreams reaching v1). The former upstream
   watches semstreams#986 and #977 are both CLOSED (verified 2026-08-27);
   #977 is fixed structurally in SemStreams `v1.0.0-beta.162`, which deletes
   `internal/lifecyclejoin`. We still see that race locally under
   `go test -race -tags=integration` only because we remain pinned to
   beta.161 — adopting beta.162 is breaking (newly provisioned NATS storage,
   no migration path) and needs its own change.

## Custom Agents & Skills

Canonical, platform-neutral definitions live in `.agents/`; `.claude/` and `.codex/`
hold thin adapters that name exactly one canonical file and carry none of its body.
`.agents/README.md` is the map. Run `task agents:check` after touching any of it, or
after moving the semstreams pin.

### Review Agents

**Spawning a role agent whose scope matches the diff is the default execution path
for nontrivial work — no user permission needed.** There is no "don't spawn agents
unless asked" rule in this repository. Spawn both concurrently when a change touches
both failure classes; spawn neither when it touches neither, since a reviewer outside
its scope returns noise and noise is how a real finding gets ignored. The owner session
keeps the decision — these roles return findings, never commits. Massively-parallel
`Workflow` orchestration is a separate question and remains opt-in.

Both are read-only — findings with file:line references and a severity, no fixes
unless separately authorized.

- **go-component-reviewer** (`.agents/contracts/go-component-reviewer.md`) — semstreams
  component implementations against the full checklist: config tags, Discoverable
  interface, factory registration, payload registry, NATS usage, entity identity
- **graph-event-reviewer** (`.agents/contracts/graph-event-reviewer.md`) — entity identity
  construction, event semantics, federation merge behavior, watch/real-time correctness

### Shared decision skills (`.agents/skills/`)

Two are **vendored** from semstreams — framework truth we do not own, and our copy must
match the pinned upstream byte for byte. Two are **deliberate forks** where our conventions
genuinely differ; the manifest records the upstream digest we last reconciled against, so an
upstream edit fails the check instead of diverging silently.

- `/kv-or-stream` — **vendored** — KV Watch vs JetStream Stream for a communication path
- `/orchestration-check` — **vendored** — rule vs workflow vs component boundary
- `/new-payload` — **forked** — we register explicitly at bootstrap; upstream uses `init()`
- `/query-pattern` — **forked** — we ship an MCP gateway; the framework does not

The `openspec-*` entries under `.claude/skills/` are Claude-workflow tooling and stay
platform-specific by design.

## Spec Reference

Full specification: `docs/spec/semsource-spec-v3.md`
