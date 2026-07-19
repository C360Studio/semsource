# SemSource

SemSource scrapes the pile of code, docs, configs, URLs, and media in your projects and turns them
into a live semantic knowledge graph (SKG) in [SemStreams](https://github.com/C360Studio/semstreams).
Humans and agents can ask what exists, what changed, where something is used, and
whether the graph is ready without each tool inventing its own parser, cache, and rules.
SemSource will not beat `find` for a filename or `grep` for a known string.
It earns its keep when the answer depends on relationships, history, provenance, or live state.

Run it beside one project or many. Each instance emits stable IDs, provenance, and indexing intent, so
downstream tools can inspect impact, compare versions, or build UI views without reverse-engineering
the source files.

> **Public beta (`v1.0.0-beta.5`).** See [ROADMAP.md](ROADMAP.md) for what's in the beta, the known
> limitations, and what's coming next.

## Prerequisites

SemSource runs on SemStreams and uses NATS JetStream/KV for graph
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

> **Port already allocated?** Another NATS is using 4222 — set `NATS_HOST_PORT=24222` (and
> `NATS_MONITOR_HOST_PORT=28222`) and re-run.
> **Picked up a stale image?** `docker compose up` reuses previously built/pulled images — use
> `docker compose up --build` (local changes) or `--pull always` (released images) to refresh.

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
     [Sem*]                [MCP]      [Your App]
```

Every entity gets a deterministic 6-part ID (`org.platform.domain.system.type.instance`), semantic
triples, provenance, and an indexing profile (`content`, `control`, `signal`, or `trace`). Query
consumers wait for `phase: "ready"` and then use NATS request/reply or GraphQL. The raw WebSocket
export is available for stream-oriented consumers such as federation, fan-out,
or live UI updates; it is not the primary governed query contract.

## Source Types

| Type     | What it indexes                                             | Watch                |
| -------- | ----------------------------------------------------------- | -------------------- |
| `ast`    | Go, TypeScript, Java, Python, Svelte code symbols           | fsnotify             |
| `git`    | Commits, authors, branches                                  | polling              |
| `repo`   | Full remote repo (clones and analyzes code + docs + config) | polling              |
| `docs`   | Markdown, plain text                                        | fsnotify             |
| `config` | go.mod, package.json, pom.xml, Dockerfile, etc.             | fsnotify             |
| `url`    | HTTP/S pages                                                | configurable polling |
| `image`  | PNG, JPG, etc. (metadata + optional thumbnails)             | fsnotify             |
| `video`  | Keyframe extraction via ffmpeg                              | fsnotify             |
| `audio`  | Audio metadata via ffprobe                                  | fsnotify             |

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

The compose file ships a UI-free **MVP core** by default and an optional `ui` profile. The core is
everything needed for a working, semantically-searchable graph in one command, with no browser,
proxy, UI image, sibling repo, or Node.js requirement.

Here, **UI-free** (or **headless deployment**) means that no workbench profile is enabled. It does
not select a SemSource runtime mode. Remove the old `mode` key from `semsource.json`; strict loading
rejects it as an unknown field, and `SEMSOURCE_MODE` is ignored. SemSource has one external-service
runtime.

### MVP core (default) — tier-1 semantic search out of the box

```bash
docker compose up
```

Brings up three services and indexes the current directory (`SEMSOURCE_TARGET`, default `.`):

| Service   | Port             | Description                                                            |
| --------- | ---------------- | ---------------------------------------------------------------------- |
| SemSource | `localhost:8080` | Ingest + ServiceManager HTTP API **+ MCP gateway** (published to host) |
| semembed  | internal `:8081` | OpenAI-compatible embeddings (tier-1 semantic search)                  |
| NATS      | internal `:4222` | JetStream and KV substrate                                             |

SemSource runs on [`configs/mvp.json`](configs/mvp.json) — `embedder_type: http` wired to semembed, so
`code_search` is true semantic (paraphrase-robust) from the first index, not just BM25. Point an agent at
the MCP gateway:

| URL                                             | Description                                        |
| ----------------------------------------------- | -------------------------------------------------- |
| `http://localhost:8080/mcp-gateway/mcp`         | MCP gateway — add this to Claude Code / your agent |
| `http://localhost:8080/source-manifest/status`  | Readiness/status (poll until `phase: ready`)       |
| `http://localhost:8080/source-manifest/sources` | Configured source manifest                         |

> **First-index note:** semembed embedding is CPU-heavy on the initial pass (arctic-embed-s / ONNX). The
> compose caps it at `SEMEMBED_CPUS` (default `2`) for local dev — raise it on a server. Structural
> queries (`code_context`/`callers`/`impact`/`byName`) are ready before embeddings finish; poll
> `source-manifest/status` for `index.ready` / `embedding.ready`. See
> [`configs/tiers/README.md`](configs/tiers/README.md).

### UI profile — add the SemSource workbench

```bash
SEMSOURCE_UI_IMAGE=ghcr.io/c360studio/semsource-ui:<version>@sha256:<digest> \
  docker compose --profile ui up
```

Layers the SemSource-owned project workbench and a Caddy reverse proxy on top of the unchanged core.
The image reference must be an immutable tag-plus-digest release reference. The released path does
not build UI source and needs neither a sibling checkout nor Node.js on the host.

> **Breaking profile migration:** the `ui` flag previously built a sibling SemTeams checkout. It now
> means the SemSource workbench. SemTeams owns its application packaging and connects to SemSource as
> a consumer through the same HTTP, MCP, NATS, GraphQL, and governed graph contracts used by UI-free
> deployments. The first immutable workbench registry artifact is published and verified for merged
> `main` revision `25b2816d14a147c1d6eb7b54e40668b51ba3574a`; use the exact pin below.

CI publishes and verifies trusted workbench artifacts while keeping pull requests publication-free.
Pull requests run locked UI quality/browser gates, a clean production-image build, and release
verifier contract tests without publishing. Trusted pushes publish multi-platform `linux/amd64` and
`linux/arm64` images to `ghcr.io/c360studio/semsource-ui`:

- `main`: `latest` and `sha-<full-commit>`; verification uses the SHA tag and OCI version
  `sha-<full-commit>`;
- release tag: exact `v<semver>` and plain `<semver>`; verification uses the `v` tag and OCI version
  `<semver>`; and
- every platform image: full `org.opencontainers.image.revision` plus the derived
  `org.opencontainers.image.version`.

The publish job passes its repository, manifest digest, verification tag, version, and revision to a
separate release-smoke job. That job proves the tag still resolves to the exact multi-platform
manifest, checks platform labels, pulls the exact `<tag>@sha256:<manifest-digest>`, confirms the local
`RepoDigest`, and runs `task ui:smoke`. Released smoke also proves the Compose-rendered and running
container image references equal that pin. `latest` is never acceptable release evidence, even with
a digest. A successful run records the GitHub Actions run URL/attempt in its summary and evidence
artifact; failures upload profile diagnostics.

First trusted evidence:

- exact image:
  `ghcr.io/c360studio/semsource-ui:sha-25b2816d14a147c1d6eb7b54e40668b51ba3574a@sha256:43edacf62e7908681e7bedd193d1b18f3ebe8f3de438d417c6c091517020ea20`;
- OCI version and full revision: `sha-25b2816d14a147c1d6eb7b54e40668b51ba3574a` and
  `25b2816d14a147c1d6eb7b54e40668b51ba3574a`;
- platforms: `linux/amd64` and `linux/arm64`;
- observed local `RepoDigest`:
  `ghcr.io/c360studio/semsource-ui@sha256:43edacf62e7908681e7bedd193d1b18f3ebe8f3de438d417c6c091517020ea20`;
- Compose-rendered image and running container `Config.Image`: both the exact image above; and
- [Actions run 29693062800, attempt 1](https://github.com/C360Studio/semsource/actions/runs/29693062800),
  all six workflow jobs green, including `build-and-push` and `ui-release-smoke`, released-profile
  browser 6/6, and
  [evidence artifact 8444245976](https://github.com/C360Studio/semsource/actions/runs/29693062800/artifacts/8444245976).

This is the completed successful trusted publication and release-smoke workflow evidence.

| Service             | Port             | Description                                                  |
| ------------------- | ---------------- | ------------------------------------------------------------ |
| Caddy               | `localhost:3000` | Reverse proxy for UI, GraphQL, status APIs, and raw `/graph` |
| SemSource workbench | internal `:3000` | Source/readiness/search workbench                            |

Useful ui-profile endpoints (via Caddy on `:3000`):

| URL                                            | Description                         |
| ---------------------------------------------- | ----------------------------------- |
| `http://localhost:3000`                        | SemSource workbench                 |
| `http://localhost:3000/health`                 | UI-compatible SemSource health JSON |
| `http://localhost:3000/graphql`                | GraphQL gateway                     |
| `http://localhost:3000/source-manifest/status` | SemSource readiness/status          |
| `ws://localhost:3000/graph`                    | Raw GRAPH stream export             |

The release candidate adopts the governed projection shipped for
[SemStreams #533](https://github.com/C360Studio/semstreams/issues/533) in
[`v1.0.0-beta.153`](https://github.com/C360Studio/semstreams/releases/tag/v1.0.0-beta.153). The
workbench calls the existing `POST /code-context/context` route with `want: ["graph"]`; there is no
separate projection endpoint and GraphQL is not part of this slice. Returned handles remain opaque,
and missing node details are shown as unresolved only when an explicit edge names that endpoint.
Truncated, incoherent, or zero-revision projections retain prior graph items instead of interpreting
bounded omissions as deletion. Unit/component, desktop/narrow accessibility, and non-intercepted
Caddy-to-SemSource smoke coverage prove the ready graph and valid no-graph states.

Build and smoke the local `./ui` source explicitly:

```bash
task ui:smoke:dev
```

The equivalent long-form development start is:

```bash
docker compose -f docker-compose.yml -f docker-compose.ui-dev.yml \
  --profile ui up --build
```

Workbench validation commands:

```bash
SEMSOURCE_UI_IMAGE=<tag>@sha256:<digest> task ui:smoke  # released-image start/test/teardown
task ui:smoke:dev                                      # local ./ui build/test/teardown
task ui:e2e                                            # test an already-running profile
task ui:image:release:test                             # CI/release verifier contracts; no publish
```

Both smoke paths run SemSource-owned, lockfile-matched Playwright in a container and assert the
same-origin shell, advertised backend routes, source/readiness state, real search, keyboard
result/detail navigation, and the capability-advertised graph state at desktop and narrow widths. The
released smoke neither checks out sibling source nor uses host Node.js. `task ui:e2e` joins the Compose network
of an already-running released or development profile.

To roll back the optional workbench, omit `--profile ui`; the core contracts and graph state are
unchanged. For a workbench-specific rollback, pin an earlier published tag and immutable digest.

### Configuration

Set these in `.env` or pass as environment variables:

```bash
SEMSOURCE_CONFIG=mvp.json         # Config file in configs/ (default: mvp.json — tier-1)
SEMSOURCE_TARGET=.                # Directory to mount as /workspace for ingestion
SEMSOURCE_HTTP_PORT=8080          # Host port for the SemSource HTTP API + MCP gateway
SEMEMBED_MODEL=Snowflake/snowflake-arctic-embed-s   # Embedding model
SEMEMBED_CPUS=2                   # CPU cap for semembed (raise on a server)
NATS_URL=nats://nats:4222         # NATS URL seen by the SemSource container
NATS_HOST_PORT=4222               # Host NATS client port for compose
NATS_MONITOR_HOST_PORT=8222       # Host NATS monitor port for compose
LOG_LEVEL=info                    # Log level: debug, info, warn, error
C360_PORT=3000                    # External port for Caddy (ui profile)
SEMSOURCE_UI_IMAGE=<tag>@sha256:<digest>  # Required immutable workbench release image
```

Set `SEMSOURCE_CONFIG=tiers/tier0-statistical.json` to run without semembed (BM25 keyword search only); the
`semembed` container still starts but the embedder ignores it. NATS is required because SemSource runs on
the SemStreams ServiceManager and uses JetStream/KV for graph state, ownership, and query indices.
Outside Docker, SemSource defaults to `nats://localhost:4222`; override with `--nats-url` or `NATS_URL`.

### Port Map

| Port | Service                               | Notes                                                                                                    |
| ---- | ------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| 8080 | ServiceManager HTTP API + MCP gateway | **Published to host** in the core profile                                                                |
| 8081 | semembed embeddings                   | Internal Docker network                                                                                  |
| 4222 | NATS                                  | Internal Docker network by default                                                                       |
| 3000 | Caddy entry point                     | ui profile: UI, GraphQL, source-manifest APIs, raw `/graph` stream                                       |
| 7890 | SemSource WebSocket                   | Internal raw GRAPH stream export                                                                         |
| 8082 | GraphQL gateway bind subject          | Component registration setting; UI profile proxies ServiceManager `/graph-gateway/graphql` at `/graphql` |
| 9091 | Prometheus metrics                    | Internal, proxied at `/metrics` (ui profile)                                                             |
| 3000 | SemSource workbench                   | Internal UI service, proxied only at `/` and `/_app/*` (ui profile)                                      |

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
    { "type": "ast", "path": "./", "languages": ["go", "typescript"], "watch": true },
    { "type": "docs", "paths": ["docs/", "README.md"], "watch": true },
    { "type": "config", "paths": ["./"], "watch": true }
  ]
}
```

Optional per-source fields on `ast`/`repo` entries: `languages` (list; singular `language` still
accepted), and the version-intelligence pair — `project` + `version`. Registering the **same
`project` at two paths with two `version`s** is what lights up supersession lineage and
`code_changes` ("what changed between 1.9.0 and 1.10.0"); versions correspond by project, so
declare it explicitly when the paths differ per version:

```json
{ "type": "ast", "path": "deps/depA-1.9.0", "project": "depA", "version": "1.9.0" },
{ "type": "ast", "path": "deps/depA-1.10.0", "project": "depA", "version": "1.10.0" }
```

Omitting both keeps version-independent ingestion (entity IDs unchanged).

Optional top-level fields:

| Field                     | Default              | Description                                                                                                                                                                                         |
| ------------------------- | -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `http_port`               | `8080`               | ServiceManager HTTP API port                                                                                                                                                                        |
| `entity_store.nats_url`   | —                    | Optional NATS URL reused when no `NATS_URL` or `--nats-url` is set                                                                                                                                  |
| `graph.gateway_bind`      | `"0.0.0.0:8082"`     | GraphQL gateway host:port subject used by SemStreams registration; in ServiceManager mode the live HTTP route is `/graph-gateway/graphql` on `:8080` and the `ui` profile exposes it as `/graphql`  |
| `graph.embedder_type`     | `"bm25"`             | Embedding backend: `bm25` (keyword, no dependencies) or `http` (semantic — **requires `model_registry` with an `embedding` capability**)                                                            |
| `graph.enable_clustering` | `false`              | Enable graph-clustering (tier-2 community routes); LLM summaries also need `graph.clustering_llm` + a `model_registry` `community_summary` capability                                               |
| `model_registry`          | —                    | SemStreams model-endpoint registry, passed to the ServiceManager. **Required** when `graph.embedder_type: "http"` or clustering LLM is on. See [`configs/tiers/README.md`](configs/tiers/README.md) |
| `source_roots`            | —                    | Allowlist of filesystem roots under which path-based HTTP/MCP source registration is permitted (ADR-0007)                                                                                           |
| `metrics.port`            | `9091`               | Prometheus metrics port                                                                                                                                                                             |
| `websocket_bind`          | `"0.0.0.0:7890"`     | Raw GRAPH stream WebSocket bind address                                                                                                                                                             |
| `websocket_path`          | `"/graph"`           | Raw GRAPH stream WebSocket path                                                                                                                                                                     |
| `workspace_dir`           | `~/.semsource/repos` | Base directory where **remote git repos are cloned** (not used for local relative source paths)                                                                                                     |
| `git_token`               | —                    | Token for authenticated remote repo cloning                                                                                                                                                         |
| `media_store_dir`         | —                    | Local directory for media binary storage                                                                                                                                                            |
| `streams`                 | —                    | Optional JetStream stream overrides for the external service                                                                                                                                        |

Validate without starting the engine:

```bash
semsource validate
```

## Graph Query & Status API

SemSource exposes status and query surfaces through MCP, HTTP, GraphQL, and NATS request/reply. Most
users should start with MCP, HTTP status, or GraphQL. The full NATS subject catalog, low-level schema
routes, and compatibility notes live in the
[M5 consumer integration guide](docs/integration/m5-consumer-integration.md).

### HTTP Endpoints (ServiceManager, default :8080)

| Endpoint                            | Description                                                             |
| ----------------------------------- | ----------------------------------------------------------------------- |
| `GET /source-manifest/sources`      | Configured sources                                                      |
| `GET /source-manifest/status`       | Ingestion status with per-instance phases                               |
| `GET /source-manifest/health`       | UI-compatible health envelope derived from source-manifest status       |
| `GET /source-manifest/capabilities` | Versioned SemSource workbench discovery and readiness                   |
| `POST /supersession/versionDiff`    | Changeset between two versions of a source (JSON `{project, from, to}`) |

The capability document is available from the UI-free core; it does not require the optional `ui`
profile.
It identifies its project scope as the configured deployment namespace, reports source, structural,
and semantic readiness, and enumerates exact supported query/action routes. Implemented routes whose
dependencies are still building are `not_ready`. Planned OKF, project-view, and governed graph
projection capabilities are `unsupported` with machine-readable reasons rather than speculative
links. Contract version 1 permits additive fields and map entries while preserving existing meanings.

### GraphQL Gateway

SemSource runs the graph gateway through the SemStreams ServiceManager. The live internal route is
`/graph-gateway/graphql` on the ServiceManager HTTP API (`:8080`). Reach GraphQL from the host via the
`ui` profile, where Caddy exposes the operator-facing route at `http://localhost:3000/graphql` (see
[UI profile](#ui-profile--add-the-semsource-workbench)).

| Endpoint                      | Description                                                 |
| ----------------------------- | ----------------------------------------------------------- |
| `GET /graph-gateway/graphql`  | GraphQL playground when enabled on the ServiceManager route |
| `POST /graph-gateway/graphql` | GraphQL query endpoint on the ServiceManager route          |
| `GET/POST /graphql`           | Same GraphQL route through the `ui` profile Caddy proxy     |

### Status Phases

The status endpoint reports aggregate and per-source lifecycle:

| Aggregate Phase | Meaning                                        |
| --------------- | ---------------------------------------------- |
| `seeding`       | Initial ingest in progress                     |
| `ready`         | All sources completed initial ingest           |
| `degraded`      | Seed timeout fired before all sources reported |

| Source Phase | Meaning                   |
| ------------ | ------------------------- |
| `ingesting`  | Performing initial ingest |
| `watching`   | Watching for changes      |
| `idle`       | No watch configured       |
| `errored`    | Error encountered         |

Downstream consumers should gate on `phase: "ready"` before querying the graph.

## Fused Code & Doc Context (Agent API)

For agents (e.g. Claude Code) that want a source-first answer instead of raw triples,
SemSource runs a **deterministic fusion gateway** (ADR-0004, built on semstreams
`pkg/fusion`): it resolves a query, expands the structure around it, hydrates verbatim
bodies, and returns one ranked answer with a readiness/provenance envelope. Two instances:

| Instance       | Domain       | NATS subjects    | HTTP                                            |
| -------------- | ------------ | ---------------- | ----------------------------------------------- |
| `code-context` | code symbols | `code.v1.<verb>` | `POST /code-context/<verb>` (proxied via Caddy) |
| `doc-context`  | documents    | `docs.v1.<verb>` | `POST /doc-context/<verb>` (proxied via Caddy)  |

**Verbs:** `context` (the primary fused answer), `callers`, `callees`, `impact`
(transitive reverse-relation closure), `file`, `search`.

**Request** (JSON):

```json
{
  "query": "Dispatch",
  "want": ["body", "relations", "impact"],
  "budget": { "max_nodes": 20, "max_bytes": 60000 }
}
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

### Fusion HTTP errors

Fusion HTTP failures use a versioned JSON envelope. This is a breaking change from the former
plain-text response that reported nearly every failure as `400 bad request`; clients should branch
on HTTP status plus `error.code`.

```json
{
  "error": {
    "contract_version": "1",
    "code": "dependency_unavailable",
    "class": "transient",
    "message": "graph query service is temporarily unavailable",
    "retryable": true
  }
}
```

| Status | Code                                            | Meaning                                     |
| ------ | ----------------------------------------------- | ------------------------------------------- |
| 400    | `invalid_json`, `invalid_request`               | Correct the request; do not retry unchanged |
| 405    | `method_not_allowed`                            | Use POST                                    |
| 413    | `request_too_large`                             | Reduce the request body                     |
| 500    | `internal_error`                                | SemSource failed locally                    |
| 502    | `upstream_contract_error`, `upstream_failure`   | The graph dependency failed non-transiently |
| 503    | `component_not_ready`, `dependency_unavailable` | Retry after service/dependency recovery     |
| 504    | `upstream_timeout`                              | The graph query exceeded its deadline       |

Messages are sanitized and never carry raw NATS, storage, or entity details. `index.ready: false`
and ready responses containing `misses` remain successful HTTP 200 fusion responses.

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
task core:smoke                            # UI-free Compose contract smoke
task ui:image:verify                       # Clean local production-image verification
SEMSOURCE_UI_IMAGE=<tag>@sha256:<digest> task ui:smoke  # Released profile smoke
task ui:smoke:dev                          # Local ./ui profile smoke
task ui:e2e                                # Playwright against a running UI profile
go test -race ./...                        # Race detection
```

## License

MIT
