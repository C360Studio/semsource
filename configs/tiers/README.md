# SemSource query tiers

SemSource's retrieval quality — especially `code_search` (natural-language) — depends on which
enrichment layer is running. The layers stack; each adds capability on top of the one below.

| Tier | Adds | `code_search` (NL) | External service | Runnable in semsource today |
|------|------|--------------------|------------------|------------------------------|
| **Structural** | graph-ingest / index / query / gateway | — (byName / prefix / relations / impact only) | none | ✅ |
| **0 — Statistical** | graph-embedding (**BM25**), graph-clustering (LPA) | keyword-statistical: good for term overlap, weak for paraphrase | none | ✅ (`tier0-statistical.json`) |
| **1 — Semantic** | graph-embedding (**HTTP → semembed**) | true semantic similarity (paraphrase-robust) | **semembed** (:8081, OpenAI-compatible embeddings) | ⛔ needs `model_registry` wiring (see below) |
| **2 — Semantic + Instruct** | graph-clustering (**LLM → seminstruct**), GraphRAG/community summaries | + community/summary reasoning over the graph | **seminstruct** (:8083, inference proxy) | ⛔ needs `model_registry` + graph-clustering wiring |

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

## Tier 1 — Semantic (semembed) — NOT yet wired in semsource

Tier 1 swaps the embedder to `http`, which "requires a model registry with embedding capability"
(graph-embedding). semstreams centralizes this in a `model_registry` (ADR-0002 / alpha.29 — it removed
per-component `embedder_url` in favour of the registry). **semsource does not yet wire `model_registry`
or the HTTP embedder** (tracked — see the tiered-config enablement task). The intended shape once wired:

```jsonc
{
  "namespace": "acme",
  "sources": [ /* ... */ ],
  "graph": { "embedder_type": "http" },
  "model_registry": {
    "endpoints": {
      "semembed": { "provider": "openai", "url": "http://localhost:8081/v1", "model": "<embedding-model>" }
    },
    "capabilities": { "embedding": { "endpoint": "semembed" } },
    "defaults": { }
  }
}
```

Requires **semembed** running (Rust, :8081, OpenAI-compatible embeddings, fastembed-rs).

## Tier 2 — Semantic + Instruct (seminstruct) — NOT yet wired

Tier 2 additionally wires **graph-clustering** to an LLM (via `model_registry` with an inference
capability) for community detection + summaries (GraphRAG). Requires **seminstruct** (Rust proxy,
:8083, OpenAI-compatible inference). semsource wires neither graph-clustering nor the inference
capability today.

## Current state / enablement

- **Tier 0 works now.**
- **Tier 1/2 are gated on a semsource enablement:** pass a `model_registry` through
  `config.GraphConfig` → `ssCfg.ModelRegistry`, wire `embedder_type: "http"` (already flows) and, for
  Tier 2, add graph-clustering. This needs semembed/seminstruct running to validate end-to-end, so it
  is deferred until those services are available in the dev/demo environment.
- When enabled, benchmark `code_search` across tiers on the same corpus to *feel* the difference —
  BM25's keyword ceiling vs semembed's paraphrase robustness. (This pairs with the graph-index
  throughput fix, semstreams#430 — Tier-0 embeddings were starved during that benchmark.)
