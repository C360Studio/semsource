# SemSource query tiers

SemSource's retrieval quality — especially `code_search` (natural-language) — depends on which
enrichment layer is running. The layers stack; each adds capability on top of the one below.

| Tier | Adds | `code_search` (NL) | External service | Runnable in semsource today |
|------|------|--------------------|------------------|------------------------------|
| **Structural** | graph-ingest / index / query / gateway | — (byName / prefix / relations / impact only) | none | ✅ |
| **0 — Statistical** | graph-embedding (**BM25**) | keyword-statistical: good for term overlap, weak for paraphrase | none | ✅ (`tier0-statistical.json`) |
| **1 — Semantic** | graph-embedding (**HTTP → semembed**) | true semantic similarity, paraphrase-robust (needs `query_prefix` — beats BM25 once set) | **semembed** (:8081, OpenAI-compatible embeddings) | ✅ (`tier1-semantic.json`) |
| **2 — Semantic + Instruct** | graph-clustering (LPA + **LLM → seminstruct**), GraphRAG/community summaries | + community/summary reasoning (`local`/`global`/`summary` search) | **seminstruct** (:8083, inference proxy) | ✅ (`tier2-semantic-instruct.json`) |

The other query verbs — `code_context`, `code_impact`, `doc_context`, and byName/prefix resolution —
are **structural** and work at every tier (they don't depend on the embedder). The tier only changes
`code_search` NL quality and (Tier 2) community/summary features.

> **Paths in these files are Docker-container paths** (`/workspace`, `/mnt/workspace/myrepo`) — they
> match what `docker compose` mounts, not your host. To run a tier config natively, edit the `sources`
> paths (and `source_roots`) to real host directories; under compose, point `SEMSOURCE_TARGET` at the
> directory to index instead of editing the file.

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

**Set `query_prefix` for arctic/BGE/E5 models** (see the caveat below): it applies to the query side
only (documents are embedded raw). Without it, retrieval quality collapses toward short generic entities.

```jsonc
{
  "namespace": "acme",
  "sources": [ /* ... */ ],
  "graph": { "embedder_type": "http" },
  "model_registry": {
    "endpoints": {
      "semembed": { "provider": "openai", "url": "http://localhost:8081/v1", "model": "Snowflake/snowflake-arctic-embed-s",
        "query_prefix": "Represent this sentence for searching relevant passages: " }
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

> **Resource footprint — semembed is CPU-heavy on the first index (local-dev caveat).** semembed runs
> real embedding inference (fastembed-rs / ONNX, arctic-embed-s). Embedding a full initial index is a
> burst of inference and, by default, ONNX fans out across every core: **measured ~561% CPU (≈5.6 cores)
> embedding a 21.5k-entity graph on a laptop** — fine on a server, rough on a dev machine. It's a
> **one-time first-index cost** (results land in `EMBEDDINGS_CACHE`; steady-state watch is cheap), not
> continuous. Levers if it's hammering your machine:
> - **Cap the container CPU** — `docker run --cpus=2 ...` up front, or **live with no restart / no lost
>   progress**: `docker update --cpus=2 semembed` (drops ~561%→~200%; the initial index just takes
>   longer). In compose, `deploy.resources.limits.cpus: "2"`.
> - **Index a smaller scope** first (fewer sources / a subdir) — structural queries
>   (`code_context`/`callers`/`impact`/`byName`) work at tier 0 with no embedder, so you only pay this
>   for `code_search` NL quality.
>
> semembed exposes no thread-cap env today (only `SEMEMBED_PORT`/`SEMEMBED_MODEL`/`RUST_LOG`); the
> container `--cpus` limit fully covers the local-dev need, so this is a documented lever, **not** an
> upstream ask. (A default intra-op thread cap would be a marginal semembed DX nicety if it ever comes up.)

> **Query prefix — REQUIRED for arctic/BGE/E5 (resolved semstreams#438, beta.129).** These retrieval
> models are *asymmetric*: the query must carry an instruction prefix (arctic:
> `"Represent this sentence for searching relevant passages: "`) while documents are embedded raw.
> semstreams beta.129 (PR #440) added `EndpointConfig.query_prefix` + `GenerateQuery` — set it on the
> endpoint (as above) and the semantic-search path applies it to the query only. **Measured impact
> (dogfood, 21k entities):** with the prefix the arctic relevant-vs-noise cosine margin ~doubles
> (+0.090 → +0.158), and end-to-end `code_search` beats tier-0 BM25 on paraphrase queries where the
> query words don't appear in the code (e.g. *"prevent processing the same message twice"* →
> `publish_msgid_integration_test.go`; *"compute a sha256 hash"* → `graph/embedding/cache.go`) — BM25
> whiffs those (no lexical overlap). Omit the prefix and tier-1 ranks *below* BM25. A code-specialized
> or larger embedder is a complementary product-side lever.

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
- **Tier-1 beats BM25 with the query prefix set** (semstreams#438 resolved, beta.129). On the dogfood
  corpus, semembed + `query_prefix` nails paraphrase queries that BM25 misses; omit the prefix and it
  regresses below BM25. A code-specialized or larger embedder is the complementary product-side lever.
