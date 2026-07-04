# Point Claude Code (or any agent) at SemSource over MCP

SemSource's MCP gateway **is the product surface**: once the graph is indexed, an agent queries it
with natural-language and symbol-level tools instead of grepping the filesystem. This guide gets an
agent talking to a running SemSource in a couple of minutes.

The gateway speaks **Streamable-HTTP MCP** at `/mcp-gateway/mcp`, served by SemSource's HTTP API on
port `8080`.

## 1. Bring up SemSource

The bundled compose stack gives you a working, semantically-searchable graph in one command (tier-1
semantic search out of the box — see the [Docker Compose](../../README.md#docker-compose) section):

```bash
# Index the current directory; publishes the MCP gateway on :8080
SEMSOURCE_TARGET=/path/to/your/repo docker compose up
```

Confirm the endpoint is live:

```bash
curl -s http://localhost:8080/source-manifest/status | jq '{phase, total_entities}'
# → { "phase": "ready", "total_entities": 3665 }
```

## 2. Add the server to Claude Code

Name first, URL second. Default scope is `local` (this project, not version-controlled):

```bash
claude mcp add --transport http semsource http://localhost:8080/mcp-gateway/mcp
```

From another machine on the LAN, swap `localhost` for the SemSource host's IP (MVP is same-LAN, no
TLS). To share the server with a team via a checked-in `.mcp.json`, add `--scope project`.

Verify:

```bash
claude mcp list          # semsource should be listed
claude mcp get semsource # shows the URL + transport
# inside Claude Code:  /mcp   (shows connection health + the tool list)
```

Remove it later with `claude mcp remove semsource`.

## 3. (Optional) Enable the bearer token

Auth is **off by default** (permissive) — fine for `localhost`. For a shared or LAN-exposed instance,
set `SEMSOURCE_API_TOKEN` on the SemSource container and every request must then carry a matching
`Authorization: Bearer` header:

```bash
# server side — add to the semsource service environment (or your .env)
SEMSOURCE_API_TOKEN=$(openssl rand -hex 32) docker compose up
```

```bash
# client side — pass the same token as a header
claude mcp add --transport http semsource http://<host>:8080/mcp-gateway/mcp \
  --header "Authorization: Bearer YOUR_TOKEN_HERE"
```

The token compare is constant-time; an unset/empty token disables the check entirely.

## 4. Tool cheat-sheet

Seven tools. The four **query** tools are why an agent beats grep; the two **registration** tools let
an agent point SemSource at new sources at runtime; `source_status` is the readiness gate.

| Tool | Use it to… | Notes |
|------|-----------|-------|
| `code_context` | Understand a symbol — resolved definition, verbatim body, callers **and** callees. | Structural. Gate on `index.ready`. |
| `code_impact` | See the reverse-dependency closure — what breaks if you change this symbol. | Structural. Answers what grep can't. |
| `code_search` | Semantic / natural-language search over code (*"where is the retry-with-backoff logic"*). | Semantic. Reliable once `embedding.ready`. |
| `doc_context` | Get the intended design from prose — READMEs, ADRs, docs — not just the code. | Structural over the doc graph. |
| `source_status` | Check readiness: ingest phase, per-source counts, `index` (structural) + `embedding` (semantic). | Poll this before trusting a miss. |
| `add_source` | Register a new source (repo/git/docs/config/url) to index at runtime. | Path sources must be under an allowlisted root. |
| `remove_source` | Deregister a source by handle. | Stops ingestion; does not retract existing graph data. |

### Readiness is honest — gate on it

SemSource reports **honest readiness** (semstreams ADR-066): `ready` means caught up to the latest
committed write, so a miss once the relevant signal is `ready` is a genuine absence, not build-window
lag. In practice:

- **Structural queries** (`code_context`, `code_impact`, byName) are reliable once
  `source_status.index.ready` is `true` — this happens fast, before embeddings finish.
- **`code_search`** quality tracks `source_status.embedding.ready` — the semantic pipeline catches up
  after the structural index (first-index embedding is CPU-heavy; see
  [`configs/tiers/README.md`](../../configs/tiers/README.md)).

So a good agent flow is: `add_source` (if needed) → poll `source_status` → `code_context` /
`code_search` / `code_impact` / `doc_context`.

## Verify with a raw handshake (optional)

No agent required — a raw JSON-RPC `initialize` proves the endpoint speaks MCP and lists the tools:

```bash
URL=http://localhost:8080/mcp-gateway/mcp
SID=$(curl -sD - -o /dev/null -X POST "$URL" \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"0.0.1"}}}' \
  | grep -i 'Mcp-Session-Id' | tr -d '\r' | awk '{print $2}')

curl -s -X POST "$URL" \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -H "Mcp-Session-Id: $SID" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  | sed -n 's/^data: //p' | jq '.result.tools[].name'
```

Expect the seven tool names above.

## Related

- [Docker Compose bringup](../../README.md#docker-compose) — the one-command tier-1 MVP stack.
- [Query tiers](../../configs/tiers/README.md) — what `code_search` quality depends on (BM25 vs semembed).
- [M5 consumer integration](m5-consumer-integration.md) — status gating and the NATS/GraphQL query surfaces.
