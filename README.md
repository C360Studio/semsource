# SemSource

Graph-first knowledge ingestion for the [SemStreams](https://github.com/C360Studio/semstreams) ecosystem. Point it at code, docs, configs, URLs, or media — it builds a normalized knowledge graph and streams it to downstream consumers via WebSocket.

Drop a SemSource instance next to any project you want to index. Run one or many — each produces a deterministic graph stream that any SemStreams app (SemSpec, SemDragon, or your own) can consume. Multiple SemSource instances feeding multiple consumers is the intended topology. The federation model ensures `public.*` entities merge cleanly across instances while `{org}.*` entities stay sovereign.

## Quick Start

```bash
# Install
go install github.com/c360studio/semsource/cmd/semsource@latest

# Auto-detect your project and create semsource.json
semsource init --quick

# Start ingesting
semsource run
```

Or run the interactive wizard for full control:

```bash
semsource init
```

## What It Does

SemSource ingests heterogeneous sources and emits a continuously updated knowledge graph stream:

```
[Your Code] ─┐                                              ┌─→ [SemSpec]
[Your Docs] ─┤                                              │
[Config]    ─┼─→ [SemSource] ─→ WebSocket :7890/graph ──────┼─→ [SemDragon]
[URLs]      ─┤                                              │
[Media]     ─┘                                              └─→ [Your App]
```

Every entity gets a deterministic 6-part ID (`org.platform.domain.system.type.instance`) and a set of semantic triples. Consumers receive SEED, DELTA, RETRACT, and HEARTBEAT events. Multiple SemSource instances can feed the same consumer — federation merges the graphs automatically.

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

Run the full stack with the monitoring dashboard:

```bash
docker compose up
```

This starts:

| Service | Port | Description |
|---------|------|-------------|
| Caddy | `localhost:3000` | Reverse proxy — single entry point |
| SemSource | internal :7890 | Graph WebSocket (proxied at `/graph`) |
| SemStreams UI | internal :5173 | Monitoring dashboard |

Access the UI at **http://localhost:3000** and the graph WebSocket at **ws://localhost:3000/graph**.

### Configuration

Set these in `.env` or pass as environment variables:

```bash
SEMSOURCE_CONFIG=semsource.json   # Config file in configs/ (default: semsource.json)
SEMSOURCE_TARGET=.                # Directory to mount as /workspace for ingestion
LOG_LEVEL=info                    # Log level: debug, info, warn, error
C360_PORT=3000                    # External port for Caddy
UI_CONTEXT=../semstreams-ui       # Path to semstreams-ui repo
```

NATS is required — SemSource runs on the semstreams ServiceManager, which requires NATS JetStream. The default
`docker compose up` includes NATS. The `--profile nats` flag is no longer needed.

### Port Map

Ports are chosen to avoid clashes when running alongside other SemStreams services:

| Port | Service | Notes |
|------|---------|-------|
| 3000 | Caddy entry point | UI + graph WebSocket |
| 4222 | NATS | JetStream, included by default |
| 7890 | SemSource WebSocket | Internal, proxied via `/graph` |
| 5173 | SemStreams UI (Vite) | Internal, proxied via `/*` |
| 8080 | ServiceManager HTTP API | Status, sources, predicates, metrics |
| 9090 | Prometheus metrics | Scrape endpoint (semstreams alpha.61+) |

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
| `websocket_bind` | `"0.0.0.0:7890"` | WebSocket server bind address |
| `websocket_path` | `"/graph"` | WebSocket path |
| `workspace_dir` | — | Root directory for relative source paths |
| `git_token` | — | Token for authenticated remote repo cloning |
| `media_store_dir` | — | Local directory for media binary storage |

Validate without starting the engine:

```bash
semsource validate
```

## Graph Query & Status API

SemSource exposes graph query and status endpoints via NATS request/reply and HTTP.

### NATS Subjects

| Subject | Type | Description |
|---------|------|-------------|
| `graph.query.entity` | Request/Reply | Query a single entity by ID |
| `graph.query.relationships` | Request/Reply | Query entity relationships |
| `graph.query.pathSearch` | Request/Reply | Traverse paths between entities |
| `graph.query.status` | Request/Reply | Current ingestion status |
| `graph.query.sources` | Request/Reply | Configured source manifest |
| `graph.query.predicates` | Request/Reply | Predicate schema by source type |

### HTTP Endpoints (ServiceManager, default :8080)

| Endpoint | Description |
|----------|-------------|
| `GET /source-manifest/sources` | Configured sources |
| `GET /source-manifest/status` | Ingestion status with per-instance phases |
| `GET /source-manifest/predicates` | Predicate schema grouped by source type |
| `GET /graphql` | GraphQL gateway (port 8082) |

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

## Building from Source

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
