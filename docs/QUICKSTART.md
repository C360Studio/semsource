# SemSource Quickstart

This is the zero-to-first-query path: point SemSource at a repository, watch
it reach `phase: ready`, and ask it a real question — then connect an agent.
It covers one repository first, then several repositories with explicit
identity (including the git-submodule case).

This document is executable: CI extracts its marked command blocks (the
`quickstart:` marker in a fence's info string) and runs them verbatim, in
order, against the same public fixture repositories they name (`test/e2e`,
quickstart tracks) — if a command here stops producing the documented
outcome, a build fails before you do. Unmarked blocks are variants or
examples.

**Deployment posture:** SemSource targets same-LAN deployment. Nothing below
sets up TLS or a reverse proxy; don't expose these ports to untrusted
networks.

## Prerequisites

- **Docker** — the recommended path. Docker Compose brings up NATS,
  embeddings, and SemSource together; nothing else is installed on the host.
- **Without Docker** you need:
  - the `semsource` binary — `go install github.com/c360studio/semsource/cmd/semsource@latest`
    (Go 1.26.3+), and
  - a reachable **NATS server with JetStream**. The bundled compose file can
    provide just that piece: `docker compose up -d nats` from a
    [semsource checkout](https://github.com/C360Studio/semsource) publishes
    NATS on `localhost:4222`. SemSource finds it there by default; point
    elsewhere with `NATS_URL` or `--nats-url`.
- A repository to index. The steps below use
  [`C360Studio/semdev-test`](https://github.com/C360Studio/semdev-test), a
  tiny public Go fixture — substitute your own repository anywhere it
  appears.

The HTTP surface (status, queries, MCP) serves on port `8080`; override the
host port with `SEMSOURCE_HTTP_PORT`. The command blocks below use
`${SEMSOURCE_HTTP_PORT:-8080}` so they work either way.

## One repository: zero to first query

### Step 1 — get a repository

```bash quickstart:single
git clone https://github.com/C360Studio/semdev-test.git
```

### Step 2 — start SemSource

Two ways to start; everything after this step is identical for both.

> **Upgrade note (fresh storage):** SemSource releases that adopt a breaking
> SemStreams pin require fresh NATS storage: `docker compose down -v`, then
> start and reseed (natively: clear the NATS server's JetStream data
> directory). The graph re-derives entirely from your sources — nothing is
> lost but the time to reseed. Release notes say when this applies.

**Option A — Docker Compose (recommended).** From a semsource checkout,
mount your repository as the ingest target:

```bash
git clone https://github.com/C360Studio/semsource.git
cd semsource
SEMSOURCE_TARGET=../semdev-test docker compose up -d --build --wait
```

No config file is needed: the default [`configs/mvp.json`](../configs/mvp.json)
indexes whatever `SEMSOURCE_TARGET` mounts (code, docs, and config files,
namespace `c360`) and wires `code_search` to the bundled semembed service, so
search is semantic from the first index.

**Option B — native binary ("without Docker").** From inside the repository,
generate a config and run the engine:

```bash quickstart:single
cd semdev-test
semsource init --quick
```

`init --quick` inspects the directory and writes `semsource.json` with zero
prompts: it detects the language, registers code + docs + config sources plus
the git history (from the `origin` remote), and derives the namespace from
the remote's owner — here `c360studio` (a repository with no remote falls
back to its directory name). The namespace becomes the `org` segment of
every entity ID; run `semsource init` instead for full control.

```bash quickstart:single
semsource run
```

`semsource run` stays in the foreground — leave it running and continue in a
second terminal. Natively the search index defaults to BM25 keyword ranking
(no extra services); the compose stack upgrades it to semantic embeddings.

### Step 3 — watch it become ready

```bash quickstart:single
curl -sf --max-time 5 "http://localhost:${SEMSOURCE_HTTP_PORT:-8080}/source-manifest/status"
```

Re-run until the response reports `"phase": "ready"` — typically under a
minute for a small repository; large repositories take longer on the first
seed. Three signals matter, and they become ready in this order:

| Signal                | Meaning when `ready`/`true`                                             |
| --------------------- | ----------------------------------------------------------------------- |
| `"phase": "ready"`    | Every configured source finished its initial seed                       |
| `"index": {...}`      | The structural index caught up — `code_context`/`callers`/`impact` answer reliably |
| `"embedding": {...}`  | The semantic pipeline caught up — `code_search` ranking is trustworthy  |

While seeding, the per-source `files_parsed` and `bodies_offloaded` counters
advance even when the published-entity count is flat — that's a working seed,
not a hang. `GET /source-manifest/sources` shows what was registered.

### Step 4 — first query

Ask for a fused answer about a symbol you know exists (`Classify` lives in
the fixture's `health.go`):

```bash quickstart:single
curl -sf --max-time 10 -X POST "http://localhost:${SEMSOURCE_HTTP_PORT:-8080}/code-context/context" \
  -H 'Content-Type: application/json' \
  -d '{"query":"Classify"}'
```

The response is a ranked answer with the symbol's verbatim body, its
relations, and provenance — plus a readiness envelope, so a not-ready graph
returns an honest empty answer instead of a false "not found".

Search by meaning instead of by name:

```bash quickstart:single
curl -sf --max-time 10 -X POST "http://localhost:${SEMSOURCE_HTTP_PORT:-8080}/code-context/search" \
  -H 'Content-Type: application/json' \
  -d '{"query":"classify service health"}'
```

### Step 5 — connect an agent

The MCP gateway serves the same graph as tools
(`code_context`, `code_search`, `code_impact`, `doc_context`, `code_changes`,
`graph_search`, `source_status`, `add_source`, `remove_source`):

```bash
claude mcp add --transport http semsource "http://localhost:${SEMSOURCE_HTTP_PORT:-8080}/mcp-gateway/mcp"
```

Auth, readiness gating, and the tool cheat-sheet:
[docs/integration/mcp-quickstart.md](integration/mcp-quickstart.md).

## Several repositories, explicit identity

Registering multiple repositories is where identity starts to matter. Every
entity ID is six parts — `{org}.{platform}.{domain}.{system}.{type}.{instance}`
— and two levers decide how repositories relate in one graph:

- **Namespace (org).** Your configured `namespace` is sovereign: entities in
  `acme.*` never merge with another org's. The reserved `public.*` namespace
  is the opposite — deterministic IDs for open-source entities that merge
  unconditionally across every instance that ingests them.
- **`project` + `version` per source.** A source's project identity is
  derived automatically (from a repository URL, or from the path for local
  sources). Declare `project` explicitly when the automatic value would be
  wrong or unstable — e.g. a local clone directory whose name isn't the
  project — and pair it with `version` to scope entities to a specific
  revision. Registering the *same* `project` at two `version`s is what
  lights up version diffs (`code_changes`).

The same rules make git submodules Just Work: a submodule's code is scoped to
its **own** project (from its resolved URL) at a version derived from the
pinned gitlink SHA — never blended into the parent's identity. The same pin
ingested via any number of parents (or registered standalone, as below)
yields byte-identical entity IDs and merges into one entity
([ADR-0012](adr/0012-submodule-identity.md)).

The multi-repository steps register a **remote + local mix**: the remote
`semdev-test` (which declares `semdev-test-sub` as a submodule) and a local
clone of `semdev-test-sub` itself, pinned to the same commit the parent pins
— so the merge is observable at the end.

### Step 1 — a workspace and a local repository

```bash quickstart:multi
mkdir semsource-multi && cd semsource-multi
git clone https://github.com/C360Studio/semdev-test-sub.git
git -C semdev-test-sub checkout b1256521ee39
```

(The checkout pins the local clone to the same commit `semdev-test`'s root
submodule declares, so both sides of the dedup demonstration line up.)

### Step 2 — one config, several sources

```bash quickstart:multi
cat > semsource.json <<'EOF'
{
  "namespace": "quickstart",
  "sources": [
    {
      "type": "repo",
      "url": "https://github.com/C360Studio/semdev-test",
      "languages": ["go"],
      "watch": true,
      "poll_interval": "30s"
    },
    {
      "type": "ast",
      "path": "./semdev-test-sub",
      "language": "go",
      "project": "github-com-c360studio-semdev-test-sub",
      "version": "b1256521ee39",
      "watch": true
    }
  ]
}
EOF
```

The `repo` source clones and analyzes the remote itself (code + docs +
config + git history) — its project identity derives from the URL. The local
`ast` source declares `project` and `version` explicitly: the slug matches
what the submodule machinery derives from the same repository's URL, and the
version is the pinned commit — that exact pair is what makes its entities
land on the same IDs as the parent's submodule pin.

`semsource add repo --url <url>` appends a remote source from the command
line; local sources with explicit `project`/`version` are declared in the
config file as above.

```bash quickstart:multi
semsource validate
```

### Step 3 — run, and read per-source readiness

```bash quickstart:multi
semsource run
```

(Or on compose: place your config in `configs/` and select it with
`SEMSOURCE_CONFIG=<name>.json`, with paths under `/workspace`.)

```bash quickstart:multi
curl -sf --max-time 5 "http://localhost:${SEMSOURCE_HTTP_PORT:-8080}/source-manifest/status"
```

Re-run until `"phase": "ready"`. With several sources the per-source entries
are the interesting part: the `repo` source expands into git + code + docs +
config instances, each reporting its own `phase` and `entity_count`, and the
declared submodules appear on the git source with a per-path state:

| Submodule `state`    | Meaning                                                                |
| -------------------- | ---------------------------------------------------------------------- |
| `materialized`       | Present at its pinned commit and ingested                              |
| `unmaterialized`     | Declared but its tree is empty — SemSource will clone it, or clone the parent with `--recurse-submodules` |
| `excluded_by_config` | Skipped because the source sets `"submodules": false`                  |
| `declared_but_absent`| Stale `.gitmodules` entry — the path no longer exists                  |
| `beyond_cap`         | Deeper than the nesting cap (10); reported, not silently dropped       |

Here you'll see both of `semdev-test`'s pins `materialized`: the root
`semdev-test-sub` at `b1256521ee39` and `nested/semdev-test-sub` at
`b191a7bf4013` — two different pins of the same project, each scoped to its
own version.

### Step 4 — query across scopes

Content from the parent repository:

```bash quickstart:multi
curl -sf --max-time 10 -X POST "http://localhost:${SEMSOURCE_HTTP_PORT:-8080}/code-context/context" \
  -H 'Content-Type: application/json' \
  -d '{"query":"Classify"}'
```

And from the submodule project — this is the dedup demonstration:

```bash quickstart:multi
curl -sf --max-time 10 -X POST "http://localhost:${SEMSOURCE_HTTP_PORT:-8080}/code-context/context" \
  -H 'Content-Type: application/json' \
  -d '{"query":"Farewell"}'
```

`Farewell` exists at the `b1256521ee39` pin. Look at the returned node's
`handle` — the entity's identifier:

```
quickstart.semsource.golang.github-com-c360studio-semdev-test-sub-b1256521ee39.function.farewell-go-Farewell
```

It carries the submodule's own project scope
(`github-com-c360studio-semdev-test-sub`) and that pin — and there is exactly
**one** such entity, even though two registrations supplied it (your
standalone `ast` source and `semdev-test`'s root submodule pin). Same
project, same version, byte-identical IDs, one merged entity — dedup by
construction, not by reconciliation. The nested `b191a7bf4013` pin (which
predates `Farewell`) stays a separate version scope of the same project —
that's the pair `code_changes` diffs.

## Troubleshooting

Every entry keys on a signal you can observe on
`GET /source-manifest/status` (or the query routes' HTTP status codes) —
what it means, and what to do.

| Signal | Meaning | Action |
| ------ | ------- | ------ |
| `phase: "seeding"` and per-source `files_parsed` / `bodies_offloaded` advancing between polls | The seed is working through files; publishing may lag parsing | Wait; re-poll. Large repos seed for a while on first run |
| `phase: "seeding"`, counters **not** advancing, `publish_total` flat | A source is stalled, not slow | Check that source's `last_error` (`code` + `message`); verify NATS is up and reachable at your `NATS_URL` |
| A source's `backpressure: true` | Its publisher is retrying against a refusing or saturated transport — no drops, no errors, but functionally stalled | Check NATS health and capacity (server up? stream limits hit?); the flag clears on its own once the transport drains |
| `phase: "degraded"` | The seed timeout fired before every source reported | Find the source whose `phase` is not `watching`/`idle`; its `last_error` says why. Fix and restart |
| `phase: "ready"` but `code_context` misses a symbol you can see, `index.ready: false` | Sources finished, structural index still catching up | Wait for `index.ready: true`, then re-query — structural queries gate on it |
| `code_search` results weak or empty, `embedding.ready: false` (or `available: false`) | Semantic ranking not caught up (or not configured — native default is BM25) | Wait for `embedding.ready`; for semantic search natively, configure `graph.embedder_type: "http"` + a `model_registry` (the compose stack does this for you) |
| A source's `phase: "errored"` or `error_count` > 0 | That source hit parse/publish failures; entities may be missing | Read `last_error.code` — bad paths and unreadable files are config-side; publish errors are transport-side |
| A submodule path's `state` ≠ `materialized` | Declared submodule code is not in the graph | See the state table above — each state names its fix |
| `total_entities: 0` and never leaves `seeding` | Sources registered but nothing ingests (wrong paths, empty target) | `GET /source-manifest/sources` shows exactly what was registered; `semsource validate` checks the config without starting |
| Query route answers `503` (`component_not_ready` / `dependency_unavailable`) | The stack is still starting, or NATS dropped | Retryable — poll status; if persistent, check NATS |
| `curl: connection refused` on port 8080 | Engine not up yet, or a port collision | Compose: another stack on 8080/4222 — set `SEMSOURCE_HTTP_PORT` / `NATS_HOST_PORT`. Native: is `semsource run` still in the foreground? |

## Where to next

- [README](../README.md) — full config reference (every source type and
  top-level field), compose profiles, and the API surfaces.
- [docs/integration/mcp-quickstart.md](integration/mcp-quickstart.md) — the
  agent-side walkthrough of the MCP tools.
- `configs/tiers/` — search-quality tiers, from BM25-only to
  semantic-instruct.
