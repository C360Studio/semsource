# SemSource on SemEngine — consumer adoption note

Audience: SemTeams, SemDragon, and any deployment that upgrades from a SemStreams-backed SemSource
(`v1.0.0-beta.9` or earlier) to the first SemEngine-backed tag. SemSpec is retired and out of scope (#251).

The full before/after contract table is in [ADR-0013](../adr/0013-substrate-cutover-to-semengine.md). This note
covers what a consumer has to change.

## What you must do

**Provision fresh NATS storage and re-ingest.** The SemEngine substrate changes the entity-ID order and mints the
platform authority (SemEngine ADR-102/104). There is no migration, alias, or ID rewrite. For Compose:

```bash
docker compose down -v   # removes NATS volumes
docker compose up -d     # fresh provision + reseed from source
```

SemSource refuses to boot over a populated `semstreams_config_<org>_<stem>` bucket. Do not delete that bucket by
hand to get past the refusal: old-order IDs left in `ENTITY_STATES` would then mix into the new graph.

**Change hard-coded entity-ID prefixes.** IDs are now `{org}.{platform}.{system}.{domain}.{type}.{instance}`
(previously `{org}.semsource.{domain}.{system}.…`). `platform` is minted by SemEngine, so read it from an entity
you received; never construct one yourself.

## `/ws` WebSocket export (#262)

`ws://<host>:7890/ws` (and `:3000/ws` in the `ui` profile) no longer relays raw `graph.ingest.*` payloads. It
watches the `ENTITY_STATES` KV bucket and sends one JSON object per text frame:

```json
{"type":"put","bucket":"ENTITY_STATES","key":"<entity id>","revision":42,"timestamp":1731330000100,"payload":{"...":"stored entity state"}}
{"type":"delete","bucket":"ENTITY_STATES","key":"<entity id>","revision":43,"timestamp":1731330000200}
{"type":"poisoned","bucket":"ENTITY_STATES","key":"<entity id>","revision":44,"error":"<classification code>"}
{"type":"synced","bucket":"ENTITY_STATES","revision":44}
```

- **Keep a map by `key`:** `put` sets it, `delete` and `poisoned` remove it (log the `poisoned` error; a later
  valid write heals the key).
- **On connect**, the server sends one `put` per current entity, one `poisoned` per unreadable key, then `synced`.
  Frames before `synced` are the snapshot; frames after it are live changes.
- **Delivery is convergent, not every-revision.** Intermediate revisions of a key may be folded, but its latest
  write is never omitted. Do not count frames or expect every revision.
- **Reconnect** is the recovery path. Upgrades answer `503` while the server's view is catching up or its watch is
  lost; a lost watch closes clients with code `1013`. Reconnect and rebuild the map from the fresh snapshot.
- **Remove:** `ack`/`nack` replies (frames you send are read and discarded), `delivery_mode`, and any decoding of
  `entity`/`manifest`/`status`/`predicates` envelopes. Manifest, status, and predicates never appear on `/ws`; read
  them over HTTP, MCP, or `graph.query.*`.

## Readiness and status

- **Gate on `GET /source-manifest/status` → `phase: "ready"`** (unchanged). `phase: "degraded"` means a source errored, a
  seed timed out, or the last seed lost entities (`seed_lost > 0`). The graph is queryable but incomplete: surface
  `sources[].last_error` / `seed_lost` instead of waiting indefinitely. A clean re-seed returns it to `ready`.
- `index.ready` gates structural queries. `embedding.ready` tracks the Tier 0 word (BM25) index behind
  `code_search`. With `graph.lexical_search: false` it reports `state: "disabled"` with reason `not_enabled`
  (`retryable: false`); never wait for it.
- Per-source `publish_total` is replaced by `offered_total`, `delivered_total`, `lost_total`, and `seed_lost`
  (offered = delivered + lost + in flight).
- If you call SemEngine's own `graph.query.*` verbs directly, read readiness from the reply: `index_not_ready` is
  transient; retry it (SemEngine ADR-109).
- A disabled capability answers `not_enabled`, HTTP `501`, `retryable: false` on every surface. Do not retry it.

## Other changes you may hit

- **MCP `graph_search`:** the `llm_answer` rung and the `answer_model` / `degraded` fields are gone. Rungs are
  `entities_only`, `community_summaries`, `unknown`; results are typed `matches`, `community_summaries`, `entity`,
  `path`, `hints` (#292). Other MCP tool names are unchanged.
- **Fusion envelope** (`code_context` family, HTTP and MCP) is contract v2, carrying `deferred` / `defer_reason`;
  decoders keyed on the v1 `index` / `view_revision` fields change.
- **`add_source` / `remove_source`** persist desired state that applies after the next SemSource restart. Do not
  wait for the new source to become live without restarting.
- **GraphQL** is served only on the shared HTTP port at `/graph-gateway/graphql`; the `:8082` listener and the
  playground are gone.
- **Refused config keys:** `graph.embedder_type` (use `graph.lexical_search`), `graph.gateway_bind`, and
  `git_token` (set `SEMSOURCE_GIT_TOKEN`). A deployment that ran `main` with `git_token` before #309 should rotate
  the token.
- **Outward names:** `semstreams_*` metrics, `semstreams.graph.mutation`, and the `SemStreamsBase` IRI base are now
  `semengine_*`, `semengine.graph.mutation`, and `SemEngineBase`.

Questions: file a SemSource issue that references #237.
