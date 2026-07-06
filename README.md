# SemSource

Graph-first knowledge ingestion for the [SemStreams](https://github.com/C360Studio/semstreams)
ecosystem. Point it at code, docs, configs, URLs, or media; it builds governed graph state in
SemStreams and exposes it through `graph.query.*`, GraphQL, and source status APIs.

Drop a SemSource instance next to any project you want to index. Run one or many; each produces
deterministic 6-part entity IDs and exact predicate ownership claims so SemStreams can store, index,
and query the graph consistently. SemSource runs as a standalone external service and bootstraps its
own ownership contract.

> **Public beta (`v1.0.0-beta.1`).** See [ROADMAP.md](ROADMAP.md) for what's in the beta, the known
> limitations, and what's coming next.

## Prerequisites

SemSource runs on the SemStreams ServiceManager and uses NATS JetStream/KV for graph
state, ownership, and query indices — so **a NATS server is always required**.

- **Docker** — the recommended path; `docker compose up` bundles NATS + embeddings and
  needs nothing else installed.
- To run the CLI **natively** instead: **Go 1.26.3+** (to build/install) and a reachable
  **NATS server** (JetStream enabled).
- `ffmpeg` / `ffprobe` on your `PATH` — only if you ingest `video` / `audio` sources.

## Quick Start

### Fastest — Docker Compose (bundles NATS + tier-1 semantic search)

```bash
git clone https://github.com/C360Studio/semsource.git && cd semsource
docker compose up            # indexes the current directory; MCP gateway on :8080
```

One command, no NATS to run yourself. See [Docker Compose](#docker-compose) for profiles,
ports, config, and connecting an agent.

### Native CLI

```bash
# Install (requires Go 1.26.3+)
go install github.com/c360studio/semsource/cmd/semsource@latest

# SemSource needs NATS (JetStream + KV) — start one if you don't have it:
docker run --rm -p 4222:4222 nats:2-alpine -js

# Auto-detect your project, write semsource.json, and start ingesting
semsource init --quick
semsource run
```

Or run the interactive wizard for full control: `semsource init`.

## What It Does

SemSource ingests heterogeneous sources and maintains a continuously updated governed graph:

```
[Your Code] ─┐
[Your Docs] ─┤
[Config]    ─┼─→ [SemSource processors]
[URLs]      ─┤              │
[Media]     ─┘              ▼
                    graph.ingest.entity
                             │
                             ▼
                       ENTITY_STATES
                             │
                             ▼
                    graph-query / GraphQL
                             │
        ┌────────────────────┼────────────┐
        ▼                    ▼            ▼
     [SemSpec]          [SemDragon]  [Your App]
```

Every entity gets a deterministic 6-part ID (`org.platform.domain.system.type.instance`), semantic
triples, provenance, and an indexing profile (`content`, `control`, `signal`, or `trace`). Query
consumers wait for `phase: "ready"` and then use NATS request/reply or GraphQL. A legacy raw
WebSocket export still exists in standalone mode, but it is not the primary consumer contract.

## Source Types

| Type | What it indexes | Watch |
|------|----------------|-------|
| `ast` | Go, TypeScript, Java, Python, Svelte code symbols | fsnotify |
| `git` | Commits, authors, branches | polling |
| `repo` | Full remote repo (clones and analyzes code + docs + config) | polling |
| `docs` | Markdown, plain text | fsnotify |
| `config` | go.mod, package.json, pom.xml, Dockerfile, etc. | fsnotify |
| `url` | HTTP/S pages | configurable polling |
| `image` | PNG, JPG, etc. (metadata + optional thumbnails) | fsnotify |
| `video` | Keyframe extraction via ffmpeg | fsnotify |
| `audio` | Audio metadata via ffprobe | fsnotify |

## CLI Reference

```
semsource init [--quick]              Setup wizard — creates semsource.json
semsource run [--log-level debug]     Start the ingestion engine
semsource add <type> [flags]          Add a source
semsource remove [--index N]          Remove a source
semsource sources                     List configured sources
semsource validate                    Check config without starting
semsource version                     Print version
```

### Managing Sources

```bash
# Add sources (non-interactive)
semsource add ast --path ./src --language go
semsource add repo --url github.com/org/repo --branch main
semsource add docs --paths "docs/,README.md"
semsource add url --urls "https://api.example.com/docs" --poll-interval 10m

# Add sources (interactive)
semsource add

# List what's configured
semsource sources

# Remove a source (interactive)
semsource remove

# Remove by index (from 'semsource sources' output)
semsource remove --index 2
```

## Docker Compose

The compose file ships two profiles. The default is the **MVP core** — everything you need for a
working, semantically-searchable graph in one command, with no sibling repo required.

### MVP core (default) — tier-1 semantic search out of the box

```bash
docker compose up
```

Brings up three services and indexes the current directory (`SEMSOURCE_TARGET`, default `.`):

| Service | Port | Description |
|---------|------|-------------|
| SemSource | `localhost:8080` | Ingest + ServiceManager HTTP API **+ MCP gateway** (published to host) |
| semembed | internal `:8081` | OpenAI-compatible embeddings (tier-1 semantic search) |
| NATS | internal `:4222` | JetStream and KV substrate |

SemSource runs on [`configs/mvp.json`](configs/mvp.json) — `embedder_type: http` wired to semembed, so
`code_search` is true semantic (paraphrase-robust) from the first index, not just BM25. Point an agent at
the MCP gateway:

| URL | Description |
|-----|-------------|
| `http://localhost:8080/mcp-gateway/mcp` | MCP gateway — add this to Claude Code / your agent |
| `http://localhost:8080/source-manifest/status` | Readiness/status (poll until `phase: ready`) |
| `http://localhost:8080/source-manifest/sources` | Configured source manifest |

> **First-index note:** semembed embedding is CPU-heavy on the initial pass (arctic-embed-s / ONNX). The
> compose caps it at `SEMEMBED_CPUS` (default `2`) for local dev — raise it on a server. Structural
> queries (`code_context`/`callers`/`impact`/`byName`) are ready before embeddings finish; poll
> `source-manifest/status` for `index.ready` / `embedding.ready`. See
> [`configs/tiers/README.md`](configs/tiers/README.md).

### UI profile — add the monitoring dashboard

```bash
docker compose --profile ui up
```

Layers the SemStreams UI dashboard and a Caddy reverse proxy on top of the core. **Requires the
`../semstreams-ui` repo checked out** (`UI_CONTEXT`).

| Service | Port | Description |
|---------|------|-------------|
| Caddy | `localhost:3000` | Reverse proxy for UI, GraphQL, status APIs, and legacy `/graph` |
| SemStreams UI | internal `:5173` | Monitoring dashboard |

Useful ui-profile endpoints (via Caddy on `:3000`):

| URL | Description |
|-----|-------------|
| `http://localhost:3000` | SemStreams UI |
| `http://localhost:3000/graphql` | GraphQL gateway |
| `http://localhost:3000/source-manifest/status` | SemSource readiness/status |
| `ws://localhost:3000/graph` | Legacy raw GRAPH stream export |

### Configuration

Set these in `.env` or pass as environment variables:

```bash
SEMSOURCE_CONFIG=mvp.json         # Config file in configs/ (default: mvp.json — tier-1)
SEMSOURCE_TARGET=.                # Directory to mount as /workspace for ingestion
SEMSOURCE_HTTP_PORT=8080          # Host port for the SemSource HTTP API + MCP gateway
SEMEMBED_MODEL=Snowflake/snowflake-arctic-embed-s   # Embedding model
SEMEMBED_CPUS=2                   # CPU cap for semembed (raise on a server)
NATS_URL=nats://nats:4222         # NATS URL seen by the SemSource container
LOG_LEVEL=info                    # Log level: debug, info, warn, error
C360_PORT=3000                    # External port for Caddy (ui profile)
UI_CONTEXT=../semstreams-ui       # Path to semstreams-ui repo (ui profile)
```

Set `SEMSOURCE_CONFIG=tier0-statistical.json` to run without semembed (BM25 keyword search only); the
`semembed` container still starts but the embedder ignores it. NATS is required because SemSource runs on
the SemStreams ServiceManager and uses JetStream/KV for graph state, ownership, and query indices.
Outside Docker, SemSource defaults to `nats://localhost:4222`; override with `--nats-url` or `NATS_URL`.

### Port Map

| Port | Service | Notes |
|------|---------|-------|
| 8080 | ServiceManager HTTP API + MCP gateway | **Published to host** in the core profile |
| 8081 | semembed embeddings | Internal Docker network |
| 4222 | NATS | Internal Docker network by default |
| 3000 | Caddy entry point | ui profile: UI, GraphQL, source-manifest APIs, legacy `/graph` |
| 7890 | SemSource WebSocket | Internal legacy raw stream export |
| 8082 | GraphQL gateway | Internal, proxied at `/graphql` (ui profile) |
| 9091 | Prometheus metrics | Internal, proxied at `/metrics` (ui profile) |
| 5173 | SemStreams UI (Vite) | Internal, proxied via `/*` (ui profile) |

### Connect an agent (MCP)

Once the stack is up, point Claude Code (or any MCP client) at the gateway:

```bash
claude mcp add --transport http semsource http://localhost:8080/mcp-gateway/mcp
```

This is the product surface — the agent then queries the graph with `code_context`, `code_search`,
`code_impact`, `doc_context`, and `code_changes` (what changed between two versions) instead of
grepping. Full walkthrough (auth, readiness gating, tool
cheat-sheet): [docs/integration/mcp-quickstart.md](docs/integration/mcp-quickstart.md).

## Config File

SemSource uses a JSON config file (`semsource.json`). The wizard creates it for you, but it's fully hand-editable:

```json
{
  "namespace": "myorg",
  "sources": [
    { "type": "ast", "path": "./", "language": "go", "watch": true },
    { "type": "docs", "paths": ["docs/", "README.md"], "watch": true },
    { "type": "config", "paths": ["./"], "watch": true }
  ]
}
```

Optional top-level fields:

| Field | Default | Description |
|-------|---------|-------------|
| `http_port` | `8080` | ServiceManager HTTP API port |
| `mode` | `"standalone"` | Only `standalone` is supported (retained for back-compat); `headless` was removed in ADR-0006 and now fails validation |
| `entity_store.nats_url` | — | Optional NATS URL reused when no `NATS_URL` or `--nats-url` is set |
| `graph.gateway_bind` | `"0.0.0.0:8082"` | GraphQL gateway bind address (internal; host access is via the `ui` profile's Caddy on `:3000`, not `:8082` directly) |
| `graph.embedder_type` | `"bm25"` | Embedding backend: `bm25` (keyword, no dependencies) or `http` (semantic — **requires `model_registry` with an `embedding` capability**) |
| `graph.enable_clustering` | `false` | Enable graph-clustering (tier-2 community routes); LLM summaries also need `graph.clustering_llm` + a `model_registry` `community_summary` capability |
| `model_registry` | — | SemStreams model-endpoint registry, passed to the ServiceManager. **Required** when `graph.embedder_type: "http"` or clustering LLM is on. See [`configs/tiers/README.md`](configs/tiers/README.md) |
| `source_roots` | — | Allowlist of filesystem roots under which path-based HTTP/MCP source registration is permitted (ADR-0007) |
| `metrics.port` | `9091` | Prometheus metrics port |
| `websocket_bind` | `"0.0.0.0:7890"` | Legacy raw GRAPH stream WebSocket bind address |
| `websocket_path` | `"/graph"` | Legacy raw GRAPH stream WebSocket path |
| `workspace_dir` | `~/.semsource/repos` | Base directory where **remote git repos are cloned** (not used for local relative source paths) |
| `git_token` | — | Token for authenticated remote repo cloning |
| `media_store_dir` | — | Local directory for media binary storage |
| `streams` | — | Optional JetStream stream overrides for standalone mode |

Validate without starting the engine:

```bash
semsource validate
```

## Graph Query & Status API

SemSource exposes graph query and status endpoints via NATS request/reply, GraphQL, and HTTP.

### NATS Subjects

| Subject | Description |
|---------|-------------|
| `graph.query.entity` | Query a single entity by ID |
| `graph.query.entityByAlias` | Resolve an entity by alias |
| `graph.query.byName` | Resolve a name/title to ranked entity IDs (deterministic symbol lookup) |
| `graph.query.batch` | Fetch multiple entities |
| `graph.query.relationships` | Query entity relationships |
| `graph.query.pathSearch` | Traverse paths between entities |
| `graph.query.hierarchyStats` | Summarize hierarchy shape |
| `graph.query.prefix` | Page entities by ID prefix |
| `graph.query.spatial` | Spatial query surface |
| `graph.query.temporal` | Temporal query surface |
| `graph.query.semantic` | Semantic query surface |
| `graph.query.similar` | Similarity query surface |
| `graph.query.globalSearch` | Global graph search |
| `graph.query.summary` | Graph summary counts |
| `graph.query.searchGraph` | Search graph result expansion |
| `graph.query.status` | Current SemSource ingestion status |
| `graph.query.sources` | Configured source manifest |
| `graph.query.predicates` | Predicate schema by source type |
| `graph.query.versionDiff` | Changeset between two versions of a source (added/removed/changed symbols + before/after bodies) |

Compatibility note: SemStreams beta.114 routes `graph.query.capabilities` from the GraphQL gateway,
but graph-query does not currently register a responder for it. SemSource does not advertise that
subject until the upstream responder contract is restored.

### HTTP Endpoints (ServiceManager, default :8080)

| Endpoint | Description |
|----------|-------------|
| `GET /source-manifest/sources` | Configured sources |
| `GET /source-manifest/status` | Ingestion status with per-instance phases |
| `GET /source-manifest/predicates` | Predicate schema grouped by source type |
| `POST /supersession/versionDiff` | Changeset between two versions of a source (JSON `{project, from, to}`) |

### GraphQL Gateway (internal :8082)

Port `8082` is **internal to the Docker network** and is not published to the host by the
core profile. Reach GraphQL from the host via the `ui` profile, where Caddy proxies it at
`http://localhost:3000/graphql` (see [UI profile](#ui-profile--add-the-monitoring-dashboard)).

| Endpoint | Description |
|----------|-------------|
| `GET /graphql` | GraphQL playground when enabled |
| `POST /graphql` | GraphQL query endpoint |

### Status Phases

The status endpoint reports aggregate and per-source lifecycle:

| Aggregate Phase | Meaning |
|----------------|---------|
| `seeding` | Initial ingest in progress |
| `ready` | All sources completed initial ingest |
| `degraded` | Seed timeout fired before all sources reported |

| Source Phase | Meaning |
|-------------|---------|
| `ingesting` | Performing initial ingest |
| `watching` | Watching for changes |
| `idle` | No watch configured |
| `errored` | Error encountered |

Downstream consumers should gate on `phase: "ready"` before querying the graph.

## Fused Code & Doc Context (Agent API)

For agents (e.g. Claude Code) that want a source-first answer instead of raw triples,
SemSource runs a **deterministic fusion gateway** (ADR-0004, built on semstreams
`pkg/fusion`): it resolves a query, expands the structure around it, hydrates verbatim
bodies, and returns one ranked answer with a readiness/provenance envelope. Two instances:

| Instance | Domain | NATS subjects | HTTP |
|----------|--------|---------------|------|
| `code-context` | code symbols | `code.v1.<verb>` | `POST /code-context/<verb>` (proxied via Caddy) |
| `doc-context` | documents | `docs.v1.<verb>` | `POST /doc-context/<verb>` (ServiceManager :8080; not Caddy-proxied yet) |

**Verbs:** `context` (the primary fused answer), `callers`, `callees`, `impact`
(transitive reverse-relation closure), `file`, `search`.

**Request** (JSON):

```json
{ "query": "Dispatch", "want": ["body", "relations", "impact"], "budget": { "max_nodes": 20, "max_bytes": 60000 } }
```

`query` is what the agent already knows — a symbol, a path, or a natural-language question
(the lens routes it to exact, prefix, or semantic resolution). `want` and `budget` are
optional; verbs preset sensible defaults.

**Response:** the readiness/provenance envelope plus ranked `nodes` (each with verbatim
`body`, `relations`, and location), optional `paths`/`impact` facets, and `misses` carrying
`did_you_mean`. A not-ready graph returns an empty envelope (the caller should fall back to
grep) — never a false "not found". Verbatim bodies are dereferenced from an ObjectStore
handle, so a standalone or remote gateway serves source without access to the ingesting
host's worktree.

## Building from Source

Requires **Go 1.26.3+** (see `go.mod`). The binary still needs a running NATS server at
runtime (see [Prerequisites](#prerequisites)); `video`/`audio` sources additionally need
`ffmpeg`/`ffprobe` on `PATH`.

```bash
git clone https://github.com/C360Studio/semsource.git
cd semsource
go build -o semsource ./cmd/semsource
```

### Running Tests

```bash
go test ./...                              # Unit tests
go test -tags=integration ./...            # Integration tests (self-ingest)
go test -tags=e2e ./test/e2e/              # Black-box binary tests
go test -race ./...                        # Race detection
```

## License

MIT
