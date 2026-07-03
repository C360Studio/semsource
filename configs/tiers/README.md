# SemSource query tiers

SemSource's retrieval quality — especially `code_search` (natural-language) — depends on which
enrichment layer is running. The layers stack; each adds capability on top of the one below.

| Tier | Adds | `code_search` (NL) | External service | Runnable in semsource today |
|------|------|--------------------|------------------|------------------------------|
| **Structural** | graph-ingest / index / query / gateway | — (byName / prefix / relations / impact only) | none | ✅ |
| **0 — Statistical** | graph-embedding (**BM25**) | keyword-statistical: good for term overlap, weak for paraphrase | none | ✅ (`tier0-statistical.json`) |
| **1 — Semantic** | graph-embedding (**HTTP → semembed**) | semantic similarity (paraphrase-robust *in principle* — see the query-prefix caveat) | **semembed** (:8081, OpenAI-compatible embeddings) | ✅ (`tier1-semantic.json`) |
| **2 — Semantic + Instruct** | graph-clustering (LPA + **LLM → seminstruct**), GraphRAG/community summaries | + community/summary reasoning (`local`/`global`/`summary` search) | **seminstruct** (:8083, inference proxy) | ✅ (`tier2-semantic-instruct.json`) |

The other query verbs — `code_context`, `code_impact`, `doc_context`, and byName/prefix resolution —
are **structural** and work at every tier (they don't depend on the embedder). The tier only changes
`code_search` NL quality and (Tier 2) community/summary features.

## Tier 0 — Statistical (BM25) — works today

`tier0-statistical.json` is a complete, runnable config. BM25 is a pure-Go statistical embedder: it
ranks by term overlap, so it's solid when the query shares words with the code (identifiers, comments)
and weak on paraphrase ("where do we retry with backoff" won't match a `withRetry` helper unless the
words line up). No external service.

```bash
semsource run --config configs/tiers/tier0-statistical.json
```

> Observed while indexing a large dependency: on a big one-shot ingest, embeddings (even BM25) lag
> readiness — see semstreams#431. Poll `source_status` until `index.ready` and `total_entities` is
> stable before trusting `code_search`.

## Tier 1 — Semantic (semembed) — wired (`tier1-semantic.json`)

Tier 1 swaps the embedder to `http`, which resolves the `model_registry` **`embedding`** capability
(graph-embedding). semstreams centralizes endpoints in a `model_registry` (ADR-0002 / alpha.29 — it
removed per-component `embedder_url` in favour of the registry); semsource passes `config.ModelRegistry`
straight through to `ssCfg.ModelRegistry`, and the ComponentManager injects it into
`deps.ModelRegistry`. Config validation fails fast: `embedder_type: "http"` without a resolvable
`embedding` capability is rejected at load, not degraded silently at runtime.

```jsonc
{
  "namespace": "acme",
  "sources": [ /* ... */ ],
  "graph": { "embedder_type": "http" },
  "model_registry": {
    "endpoints": {
      "semembed": { "provider": "openai", "url": "http://localhost:8081/v1", "model": "Snowflake/snowflake-arctic-embed-s" }
    },
    "capabilities": { "embedding": { "preferred": ["semembed"] } },
    "defaults": { "model": "semembed" }
  }
}
```

```bash
docker run -d -p 8081:8081 ghcr.io/c360studio/semembed:latest   # OpenAI-compatible embeddings
semsource run --config configs/tiers/tier1-semantic.json
```

> **Query-prefix caveat (semstreams#-ask 13).** arctic-embed / BGE / E5 retrieval models are
> *asymmetric*: the query must carry an instruction prefix (arctic: `"Represent this sentence for
> searching relevant passages: "`) while documents are embedded raw. graph-embedding's HTTP embedder
> has a single `Generate` path (no `EmbedQuery`), so the query prefix is **not** applied. Measured on
> the dogfood corpus, the prefix ~doubles the relevant-vs-noise cosine margin (+0.090 → +0.158); without
> it, tier-1 `code_search` can rank *below* tier-0 BM25 (short generic entities crowd the top). Wiring is
> correct and complete; retrieval quality is capped until semstreams supports an asymmetric query
> instruction (tracked in `docs/upstream/semstreams-asks.md` #13). A code-specialized or larger embedder
> is the complementary product-side lever.

## Tier 2 — Semantic + Instruct (seminstruct) — wired (`tier2-semantic-instruct.json`)

Tier 2 adds **graph-clustering** (`graph.enable_clustering: true`): pure-Go LPA community detection over
ENTITY_STATES → COMMUNITY_INDEX, lighting up the already-declared `local`/`global`/`summary` query
routes. `graph.clustering_llm: true` additionally enables GraphRAG community summaries via the
`model_registry` **`community_summary`** capability (→ seminstruct). LPA runs with no external service;
only the LLM summaries need seminstruct.

```bash
docker run -d -p 8083:8083 ghcr.io/c360studio/seminstruct:qwen3-0.6b   # OpenAI-compatible inference
semsource run --config configs/tiers/tier2-semantic-instruct.json
```

Requires **seminstruct** running (Rust proxy, :8083, OpenAI-compatible inference) only when
`clustering_llm` is set.

## Current state / enablement

- **Tier 0 works now** (BM25, no external service).
- **Tier 1/2 are wired** (`config.ModelRegistry` → `ssCfg.ModelRegistry`; `graph.enable_clustering`
  adds graph-clustering). Validated end-to-end against local semembed: http embedder active,
  embeddings generated with 0 errors, `provenance: embedding` on `code_search`.
- **Tier-1 quality is gated on the query-prefix gap above** — the wiring is proven, but on the dogfood
  corpus tier-1 (arctic-embed-s, no query prefix) underperformed tier-0 BM25. Fixing the prefix
  (framework) and/or choosing a code-specialized embedder (product) is the next lever.
