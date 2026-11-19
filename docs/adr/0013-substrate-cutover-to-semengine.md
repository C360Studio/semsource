# ADR-0013: SemSource's substrate is SemEngine; SemStreams is retired

- **Status:** Accepted
- **Date:** 2026-11-19
- **Issue:** [#235](https://github.com/C360Studio/semsource/issues/235); cutover landed in #222 (`4ca2072`)
- **Amends:** [ADR-0001](0001-websocket-exports-raw-entities.md) (`/ws` wire),
  [ADR-0010](0010-graph-query-mcp-tool-family.md) (`graph_search` disclosure),
  [ADR-0003](0003-programmatic-source-add-api.md) and
  [ADR-0006](0006-external-service-source-registration.md) (a source added at runtime starts after
  the next restart, not live)

## Context

SemSource was built on `github.com/c360studio/semstreams`. The governed graph substrate (entity
state, the graph-ingest write seam, query verbs, components) now lives in SemEngine. Two defects
that blocked broker recovery and source removal on the frozen SemStreams pin, a stale applied
sequence after stream recreation ([semstreams#1442]) and deleted file components returning at
restart ([semstreams#1443]), were fixed only in SemEngine
([semengine#15](https://github.com/C360Studio/semengine/issues/15),
[semengine#17](https://github.com/C360Studio/semengine/issues/17)). Carrying both substrates would
mean two copies of every contract SemSource depends on.

The move cannot be undone without a re-ingest. SemEngine ADR-102/104 reorder the entity ID and
mint the platform authority, and SemEngine renames the bucket that retains that authority
([semengine#69](https://github.com/C360Studio/semengine/issues/69)).

**Alternatives considered.** Staying on the frozen SemStreams pin `8b99efe9` (SemSource `bbfb3c6`) keeps both defects
and receives no further substrate work. Holding the cutover until neural 04C qualified would have
delayed every Tier 0 fix behind an embedder SemSource's default composition does not run.

## Decision

1. **SemEngine only.** `go.mod` requires `github.com/c360studio/semengine` and no
   `c360studio/semstreams` module, with no dual import and no shim. `TestNoSemStreamsImports`
   (`cmd/semsource/clustering_edges_test.go`) fails on a SemStreams import, `go.mod` entry or
   `go.sum` entry.
2. **Cut over at Tier 0, not 04C.** Owner ruling 2026-11-12 (Option A, recorded in
   `docs/upstream/semengine-port-ledger.md`): #222 cut main over at the 04A-qualified
   pseudo-version `v0.1.0-alpha.4.0.20261112145006-7bf638643b98`. Main then moved to
   `v0.1.0-alpha.5` (#290) and to `v0.1.0-alpha.6` with embeddings off (#302). Owner rulings on
   #237 (2026-11-16, amended 2026-11-17): the first SemEngine tag ships at Tier 0 on alpha.6, and
   neural 04C (#234) qualifies after the tag. The acceptance floor is the Tier 0 graded scorecard
   (#233, regraded on alpha.6 for #301), not "tests pass".
3. **One clean storage break.** Upgrading means fresh NATS storage and a re-ingest from source.
   Existing IDs are never aliased or rewritten. `checkLegacySemStreamsConfig`
   (`cmd/semsource/legacy_lifecycle.go`) refuses to boot over a populated
   `semstreams_config_<org>_<stem>` bucket; authority is retained in
   `semengine_config_<org>_<stem>` from then on.
4. **Substrate asks go to SemEngine issues.** `docs/upstream/semstreams-asks.md` records the
   SemStreams-era asks and receives no new rows.

## Consumer contracts: v1.0.0-beta.9 → the first SemEngine tag

Unchanged: the tool names of both MCP families (ADR-0004, ADR-0010); the `graph.query.*` verbs
SemSource serves (`status`, `sources`, `sourceSummary`, `versionDiff`, `predicates`); the consumer
readiness gate `/source-manifest/status` → `phase: "ready"`; the GraphQL route
`/graph-gateway/graphql`. Verbs SemEngine itself serves follow SemEngine's `MIGRATION.md`.

| Surface | beta.9 | First SemEngine tag | PR / issue |
| --- | --- | --- | --- |
| Entity ID | `{org}.semsource.{domain}.{system}.{type}.{instance}` | `{org}.{platform}.{system}.{domain}.{type}.{instance}`, platform minted and retained (SemEngine ADR-102/104) | #213 |
| `/ws` export | raw `graph.ingest.>` payloads (`entity`, `manifest`, `status`, `predicates`), at-most-once | `ENTITY_STATES` KV-watch frames `put`/`delete`/`poisoned`/`synced`, convergent | #222; notice #262 |
| Status figures | `publish_total` | `offered_total`, `delivered_total`, `lost_total`, `seed_lost`; a lossy seed reports `degraded` | #195 |
| MCP `graph_search` | `llm_answer` rung; `answer_model`, `degraded` fields; substrate payload verbatim | rungs `entities_only` / `community_summaries` / `unknown`; typed `matches`, `community_summaries`, `entity`, `path`, `hints` | #292 |
| Fusion envelope | contract v1, `index` and `view_revision` | contract v2, `deferred` / `defer_reason` | #222 (issue #253) |
| Disabled capability | varied per surface | `not_enabled`, HTTP 501, `retryable: false` | #222, #312 |
| Search composition | `graph.embedder_type` (`bm25`/`http`) | `graph.lexical_search` (default on); `embedder_type` refused | #290 |
| GraphQL listener | extra listener on `graph.gateway_bind` (`:8082`) and a playground | shared HTTP port only; both keys refused | #222 |
| Source lifecycle | `add_source` spawns the source live (`ready_when`: phase `watching`/`idle`); `remove_source` stops it at once; no `config_revision` | `add_source` and `remove_source` persist desired state applied after the next restart (`ready_when` says so); a retained store applies `semsource.json` edits only when `config_revision` rises | #213, #223, #271 |
| Git credentials | `git_token` in `semsource.json` | `SEMSOURCE_GIT_TOKEN` only; the file key is refused | #309 |
| Outward names | `semstreams_config_*`, `semstreams_*` metrics, `semstreams.graph.mutation`, `SemStreamsBase` IRI base | `semengine_config_*`, `semengine_*` metrics, `semengine.graph.mutation`, `SemEngineBase` IRI base | #222 (issue #256) |

Direct callers of SemEngine's own `graph.query.*` verbs read readiness from the reply
(`index_not_ready` is transient; SemEngine ADR-109, #227). SemSource's consumer gate above does
not change.

## Consequences

- Every existing deployment re-ingests once; there is no in-place migration.
- `/ws`, status-figure and `graph_search` consumers change decoders. Callers of `add_source` /
  `remove_source` (ADR-0003, ADR-0006) restart SemSource to apply the change and must not wait for
  live readiness. Consumers that hard-code an
  entity-ID prefix change it. Fusion-family MCP and HTTP callers change only for contract v2.
- The boot refusal looks only for the old config bucket. A store whose `semstreams_config_*`
  bucket was deleted by hand while `ENTITY_STATES` kept old-order IDs would boot into a mixed
  graph; the documented fresh-storage upgrade is the guard.
- Substrate cadence follows SemEngine pre-1.0 tags. Each pin bump reads SemEngine's `MIGRATION.md`
  SemSource section and reruns the Tier 0 graded scorecard.
- Neural search (04C, #234) and LLM community summaries
  ([semengine#485](https://github.com/C360Studio/semengine/issues/485)) qualify separately.

[semstreams#1442]: https://github.com/C360Studio/semstreams/issues/1442
[semstreams#1443]: https://github.com/C360Studio/semstreams/issues/1443
