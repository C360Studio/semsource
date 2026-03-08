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

Include NATS (required for image/video/audio object storage):

```bash
docker compose --profile nats up
```

### Port Map

Ports are chosen to avoid clashes when running alongside other SemStreams services:

| Port | Service | Notes |
|------|---------|-------|
| 3000 | Caddy entry point | UI + graph WebSocket |
| 4222 | NATS | Internal, opt-in via `--profile nats` |
| 7890 | SemSource WebSocket | Internal, proxied via `/graph` |
| 5173 | SemStreams UI (Vite) | Internal, proxied via `/*` |
| 8080 | Reserved | Future: semspec/semdragon backend |

## Config File

SemSource uses a JSON config file (`semsource.json`). The wizard creates it for you, but it's fully hand-editable:

```json
{
  "namespace": "myorg",
  "flow": {
    "outputs": [{
      "name": "graph_stream",
      "type": "network",
      "subject": "http://0.0.0.0:7890/graph"
    }],
    "delivery_mode": "at-least-once",
    "ack_timeout": "5s"
  },
  "sources": [
    { "type": "ast", "path": "./", "language": "go", "watch": true },
    { "type": "docs", "paths": ["docs/", "README.md"], "watch": true },
    { "type": "config", "paths": ["go.mod", "Dockerfile"], "watch": true }
  ]
}
```

Validate without starting the engine:

```bash
semsource validate
```

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
