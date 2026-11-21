# Qualify SemSource on SemEngine Tier 1 — design

## Context

Main runs SemEngine `v0.1.0-alpha.6` at Tier 0 (`compositionGeneration = 5`,
`cmd/semsource/run.go:527`). `graphSubsystemComponents` (`run.go:771`) composes graph-query with
ports only. `embedding_enabled` is left unset and reads false (`run.go:787-791`). graph-lexical is
added when `cfg.Graph.LexicalSearchEnabled()` (`run.go:865-878`). No graph-embedding is
registered (factory map `run.go:299-300`). `ModelRegistry` reaches `semconfig.Config` (`run.go:614`),
but nothing resolves it at Tier 0. `GraphConfig` has no embedding key (`config/config.go:27-54`), and
`embedder_type` is refused on presence (`config.go:274-276`).

Upstream at alpha.6 (`$SE` = the module dir):

- `graph/embedding/httpembedder`: `Config{BaseURL, Model, APIKey, QueryPrefix}` (`httpembedder.go:26-43`).
  `BaseURL` must end in `/v1`, and a reply naming another model is refused. It has no timeout of its
  own, and `Dimensions()` is learned from the first reply (`:106`).
- `processor/graph-embedding.RegisterWithEmbedder(registry, embedderType, newEmbedder)`
  (`component.go:230`) calls `newEmbedder` on each Start. Plain `Register` refuses construction
  (`:219`). The recipe in `processor/graph-embedding/MIGRATION.md:17-35` is
  `model.ResolveEndpointWithConfig(reg, model.CapabilityEmbedding)` mapped to `httpembedder.Config`.
- graph-query `embedding_enabled` (`config.go:29-33`) turns on `mergedText` (`textsearch.go:76-124`).
  It runs word and vector searches concurrently and merges them by RRF, 1/(60+place)
  (`merge.go:15,32`). Matches carry `word_rank` and `vector_rank`
  (`graph/query_global_search_types.go:109-110`), and the reply carries `merge` and `semantic`.
- If the embedding side fails, the reply is the word list with `merge.honoured: false`, one Warn,
  and `semengine_graph_query_search_merge_failed_total` (`textsearch.go:65-71,116-120`). If word
  search is not ready, `semanticAlone` answers (`:241-250`).

The tests that pin "no graph-embedding" today are
`TestGraphSubsystemComponents_GraphLexicalDefersToDefaultPorts` (`run_test.go:364`),
`TestGraphSubsystemComponents_LexicalSearchFalseIsStructuralOnly` (`run_test.go:405`) and
`TestDefaultComposeConfigDeclaresComponents` (`migration_contract_test.go:76`). The semembed service
survives in `docker-compose.yml:86-105` under profile `semantic`, digest-pinned. `test/setup03a`
already parses `SETUP03A_PROFILE=neural` (`qualification_test.go:101-122`). It fails at `configure`
for lack of a Tier 1 config (`:384`), and it carries `TestProviderFailureAndRecovery`
(`provider_test.go:27`).

## Goals / Non-Goals

**Goals:** an opt-in Tier 1 composition built only from admitted alpha.6 APIs; an honest
`graph_search` answer when merged; provider identity recorded well enough to reproduce; a fault
story where a missing provider can never read as a pass; acceptance graded against #230 and the
alpha.6 Tier 0 run.

**Non-Goals:** changing the default composition (it stays Tier 0); LLM community summaries
(semengine#485); working around upstream gaps semengine#667 (unmeasured truncation), #686 (query
prefix carried by SemSource) and #683 (sibling disambiguation); any entity-identity change.

## Decisions

1. **Opt-in switch: `graph.semantic_search` (bool, nil = off).** Turning it on requires
   `model_registry` to resolve the `embedding` capability to an endpoint that `isEmbeddingEndpoint`
   accepts (`config.go:~437`). Config validation refuses it otherwise, so a Tier 1 config cannot boot
   silently into Tier 0. It also requires `lexical_search` on, because the merge has two lists.
   *Alternative:* infer Tier 1 from the registry's presence. Rejected: Tier 0 configs carry registries
   for other capabilities, and inference would turn embeddings on by accident.
2. **Composition.** When on: register graph-embedding with `RegisterWithEmbedder("http", …)`. The
   constructor resolves the endpoint per the MIGRATION recipe and builds `httpembedder.New`.
   Compose graph-embedding and set graph-query `embedding_enabled: true`. Bump `compositionGeneration`
   to 6 with a new entry in `compositionFingerprints` (`composition_fingerprint_test.go:87`). Tier 0
   fingerprints stay byte-identical, which the test asserts.
3. **Fusion NL stays on the word index until proven otherwise.** Before implementation, verify
   whether `pkg/fusion/fusionnats` at alpha.6 resolves nl seeds through the merged `searchGraph` or
   only through `graph.lexical.query.search`. If only lexical, Tier 1 changes `graph_search` and
   leaves the fusion tools unchanged, and the change says so. It does not route fusion through a
   bespoke path.
4. **`graph_search` disclosure.** At Tier 1, `graphMatch.Relevance` (`graph_matches.go:116`) is the
   merged RRF score, so the tool description states that it is at most 2/61 and is not on the word
   scale. Each match gains `word_rank` and `vector_rank` when present, and the reply gains a `merge`
   object (`honoured`, `detail`). `honoured: false` is surfaced verbatim, never dropped.
   `TestToolsListSchemaBudget` (8 KB, `schema_budget_test.go:18`) must still pass; if it can't, the
   ranks move into the description, not the schema.
5. **Provider identity is evidence, not config.** The qualification record pins the semembed image
   digest, build SHA, served model id and revision, ONNX hash, dimensions (384), query prefix and
   preprocessing, as SETUP 03A did. The same provider instance serves both comparison points. The
   runtime only checks `Model` (upstream refuses a mismatched reply); the rest is recorded and checked
   by the harness.
6. **Fault semantics.** A configured-but-unreachable provider leaves boot up (upstream constructs
   without a request). Search answers on the word list with `merge.honoured: false`. source-manifest
   readiness reports the semantic index `unavailable`, never `ready`, until graph-embedding reports
   ready. setup03a's neural lane and the scorecard refuse to grade a run whose provider
   `/metrics` was unreachable, so it is never a pass.
7. **Configs and compose.** Restore `configs/tiers/tier1-semantic.json`: `graph.semantic_search:
   true`, registry `semembed` at `http://semembed:8081/v1`, model `Snowflake/snowflake-arctic-embed-s`
   with its query prefix. The documented launch is `docker compose --profile semantic up`. The three
   "no graph-embedding" tests become tier-conditional: Tier 0 keeps asserting absence, and Tier 1
   asserts presence plus `embedding_enabled: true`.

## Risks / Trade-offs

- **Retained Tier 0 store switching on Tier 1.** Unknown: does graph-embedding backfill vectors
  from existing `ENTITY_STATES`, or embed only new writes? It must be measured before the docs
  promise either. Until then the procedure is a reseed.
- **Seed time and cost.** Embedding 8k to 29k entities on a CPU semembed is slow. The #178 OSH tail
  against the GRAPH stream ceiling is remeasured here.
- **Merged score scale.** Clients that threshold `relevance` on the word scale break at Tier 1. That
  is why the tool description and the adoption notes say so explicitly.
- **Upstream gaps.** semengine#667 means long texts are cut at 512 tokens without being counted,
  which could depress vector recall on doc passages. Recorded, not fixed here.

## Open Questions (owner)

- Is `graph.semantic_search` the right name and shape, or should Tier 1 be a named profile
  (`graph.tier`)?
- Should a Tier 1 boot with an unreachable provider stay up degraded (proposed), or refuse to start?
