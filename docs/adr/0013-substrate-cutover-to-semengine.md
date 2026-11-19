# ADR-0013: SemSource's substrate is SemEngine; SemStreams is retired

- **Status:** Accepted
- **Date:** 2026-11-19
- **Issue:** [#235](https://github.com/C360Studio/semsource/issues/235); cutover landed in #222 (`4ca2072`)

## Context

SemSource was built on `github.com/c360studio/semstreams`. The governed graph substrate (entity
state, the graph-ingest write seam, query verbs, components) now lives in SemEngine, and SemStreams
stops receiving graph-substrate work. Carrying both would mean two copies of every contract
SemSource depends on, so the choice is one substrate.

The move is irreversible in practice. SemEngine renames the substrate's outward identifiers
([semengine#69](https://github.com/C360Studio/semengine/issues/69)), including the config bucket
that retains deployment authority, so a deployment cannot move back without re-ingesting.

## Decision

1. **SemEngine only.** `go.mod` requires `github.com/c360studio/semengine` and no
   `c360studio/semstreams` module. There is no dual import and no shim. `TestNoSemStreamsImports`
   (`cmd/semsource/clustering_edges_test.go`) fails on a SemStreams import, `go.mod` entry or
   `go.sum` entry.
2. **Cut over at Tier 0, not 04C.** Main moved in #222 at the 04A-qualified pseudo-version
   (`v0.1.0-alpha.4.0.20261112145006-7bf638643b98`), then to `v0.1.0-alpha.5` (#290) and
   `v0.1.0-alpha.6` with embeddings off (#302). Owner rulings on #237 (2026-11-16, amended
   2026-11-17): the first SemEngine tag ships at Tier 0 on alpha.6; neural 04C qualification
   (#234) is post-tag. The acceptance floor is the Tier 0 graded run (#233, regraded on alpha.6
   in #301), not "tests pass".
3. **One clean storage break.** Upgrading means fresh NATS storage and a re-ingest from source
   (`docker compose down -v`, then reseed), as ADR-102/104 already require for identity changes.
   Existing IDs are never aliased or rewritten. At startup, `checkLegacySemStreamsConfig`
   (`cmd/semsource/legacy_lifecycle.go`) refuses to boot over a populated
   `semstreams_config_<org>_<stem>` bucket. Authority is retained in
   `semengine_config_<org>_<stem>` from then on.
4. **Substrate asks go to SemEngine issues.** `docs/upstream/semstreams-asks.md` is frozen history;
   the port's crosswalk and evidence are in `docs/upstream/semengine-port-ledger.md`.

## Consumer contracts

**Unchanged:** the `graph.query.*` verbs SemSource serves (`status`, `sources`, `sourceSummary`,
`versionDiff`); the MCP tool family (ADR-0010); `/source-manifest/status` and the other HTTP
status routes; entity identity (6-part IDs built through `entityid.*`).

**Changed at the cutover:**

| Surface | Before (SemStreams) | After (SemEngine) |
| --- | --- | --- |
| `/ws` export | raw `graph.ingest.*` envelopes (`SEED`/`DELTA`/`RETRACT`/`HEARTBEAT`), ack/nack | `ENTITY_STATES` KV-watch frames `put`/`delete`/`poisoned`/`synced`, convergent (ADR-0001 amended; notice #262) |
| Readiness | gate on `GRAPH_STATUS` | read readiness from query replies; `index_not_ready` is transient (SemEngine ADR-109) |
| Fusion envelope | contract v1, `index` block | contract v2, `deferred` / `defer_reason` (#253) |
| GraphQL | own `graph.gateway_bind` listener | `graph-gateway` on the shared HTTP port at `/graph-gateway/graphql` |
| Search composition | `graph.embedder_type` (`bm25`/`http`) | `graph.lexical_search` (default on, `graph-lexical`); `embedder_type` refused at every value |
| Disabled capability | varied per surface | `not_enabled`, HTTP 501, `retryable: false` |
| Retired config | accepted | `graph.gateway_bind`, `enable_playground`, `embedding_batch_size`, `clustering_llm`, `entity_id_edges` refused with a migration message |
| Outward names | `semstreams_config_*`, `semstreams_*` metrics, `SemStreamsBase` IRI base | `semengine_config_*`, `semengine_*` metrics, `SemEngineBase` IRI base |
| Stream overrides | any extra GRAPH subject | exactly the five `graph.ingest.*` defaults (#315) |

## Consequences

- Every existing deployment re-ingests once. There is no in-place migration.
- `/ws` consumers must change decoders (#262); query, MCP and HTTP consumers do not.
- SemSource's substrate cadence follows SemEngine pre-1.0 tags. Each pin bump reads SemEngine's
  `MIGRATION.md` SemSource section and reruns the Tier 0 graded scorecard.
- Neural search (04C, #234) and LLM community summaries
  ([semengine#485](https://github.com/C360Studio/semengine/issues/485)) are not part of this
  decision and qualify separately.
