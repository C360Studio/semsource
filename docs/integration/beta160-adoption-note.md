# SemSource on SemStreams beta.160 — consumer adoption note

Audience: SemSpec (canary), SemTeams, Open Sensor Hub, and any deployment
running SemSource against a persistent NATS account.

## What you must do

**Provision fresh NATS storage when you take this SemSource version.** The
SemStreams `v1.0.0-beta.160` foundation is an intentionally clean break: there
is no upstream state migration, no compatibility shims, and no mixed-version
operation. For Compose deployments that is:

```bash
docker compose down -v   # removes NATS volumes
docker compose up -d     # fresh provision + reseed from source
```

SemSource's graph is a projection of your sources; reseeding rebuilds it
completely. Seeding wall-clock on the reference dogfood corpus is ~8 minutes.
If you have retained NATS state you cannot drop, stop the upgrade and raise it
— do not run the new version against old storage.

## What does not change for you

The read contracts you consume are stable across this migration:

- `graph.query.*` request/reply operations keep their wire shapes, including
  `graph.query.prefix` (typed, opaque-cursor pagination), `graph.query.entity`,
  `graph.query.searchGraph`, and SemSource's own `graph.query.versionDiff`.
- The MCP gateway tool surface (code_context / code_impact / doc_context /
  code_search and the source-registration tools) is unchanged.
- HTTP status (`/source-manifest/status`) and its readiness signals
  (`phase` + `index.ready` + `embedding.ready`) are unchanged — keep gating on
  all three before querying, exactly as before.
- The GraphQL surface under the UI profile and the raw WebSocket export remain.

## Behavioral notes

- Request/reply consumers may now receive a `response_too_large` classified
  error: treat it as a result-size failure (narrow the request), never as an
  availability timeout to retry.
- Mutations to missing entities fail with `entity_not_found` and never create
  stub entities. If you write to the graph (per ADR-040 curator flows), ensure
  entities are born through their envelope-carrying lane first.
- **Exact entity reads are wrapped**: `graph.ingest.query.entity` (and the
  substrate's exact-read operations generally) return `graph.ExactEntity` —
  `{"entity": {...}, "kvRevision": N}` — not a bare `EntityState`. Decoders
  reading the old bare shape will silently see zero triples.
- Metrics servers bind synchronously and fail startup loudly on a port
  collision (previously swallowed). If you run multiple SemSource processes on
  one host, give each a distinct `metrics.port`.

Questions: file a SemSource issue and tag it `beta160-adoption`.
