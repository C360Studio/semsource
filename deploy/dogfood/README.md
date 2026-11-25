# Dogfood host runbook (#391)

Two SemSource instances on the 2018 Intel Mac mini (i7-8700B, 32 GB, Docker
Desktop amd64), booting the released `1.0.0-beta.11` image. No secrets live
here; every source is a public `C360Studio/*` repository.

| Instance | Compose project | Env file | Config | HTTP/MCP | NATS (loopback) | NATS monitor (loopback) |
| --- | --- | --- | --- | --- | --- | --- |
| 1 — sem\* | `dogfood-sem` | `sem.env` | `sem.json` (namespace `c360sem`) | `:18080` | `127.0.0.1:14222` | `127.0.0.1:18222` |
| 2 — bb\* + semengine | `dogfood-bb` | `bb.env` | `bb.json` | `:28080` | `127.0.0.1:24222` | `127.0.0.1:28222` |

Shared: one semembed (`dogfood-semembed`), reached as `http://semembed:8081` on the
external Docker network `c360-dogfood-embed`. Never shared: NATS — each instance has
its own server and volume (#351, semengine#740).

Instance 1 sources (owner ruling on #390): `semsource`, `semengine`, `semteams`,
`semvocab`, `semsparql`, `semconnect`, all on `main`, as remote `repo` sources that
poll every 5 minutes. Tier 1 (`graph.semantic_search: true`).

## Boot

All commands run from the repository root.

```bash
docker network create c360-dogfood-embed            # once per host
docker compose -p dogfood-semembed -f deploy/dogfood/semembed.compose.yml up -d

# Instance 1
docker compose --env-file deploy/dogfood/sem.env -f deploy/dogfood/compose.yml up -d
```

First seeds run one at a time: start instance 2 only after instance 1 reports
`phase: "ready"` **and** `index.ready: true`.

```bash
curl -s localhost:18080/source-manifest/status | jq '{phase, index: .index.ready, embedding: .embedding.ready}'
```

`phase: ready` means seeded, not that every entity has landed; wait for
`index.ready` before the first query. After a restart the name index replays
`ENTITY_STATES` and can lag `ready` by many minutes.

## First query (from another LAN machine)

```bash
curl -s http://<mini-lan-ip>:18080/source-manifest/status | jq .phase
# MCP gateway for an agent: http://<mini-lan-ip>:18080/mcp-gateway/mcp
```

## Measure

```bash
docker stats --no-stream --format '{{.Name}}\t{{.MemUsage}}\t{{.CPUPerc}}'
curl -s 'localhost:18222/jsz?streams=true' | jq '.account_details[].stream_detail[] | select(.name=="GRAPH") | .state'
```

The GRAPH backlog cap is 256 MiB of unapplied messages; refusals show as 10077 in status.

## Upgrade and reset

- Image upgrade: change the digest in `compose.yml`, then `docker compose ... up -d semsource`
  (recreates only semsource; safe since #394).
- Identity-breaking upgrade: `docker compose ... down -v`, then `up -d` (fresh NATS storage + reseed).

## Agent-shell note

On this host, pulls from a non-interactive shell fail on the Docker Desktop keychain
credential helper even for public images. Run the pulls from a GUI terminal, or put a
shim `docker-credential-desktop` on `PATH` that answers `get` with "credentials not
found" and exit 1.
