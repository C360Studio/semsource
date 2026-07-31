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

> **Paths in these files are Docker-container paths** (`/workspace`) — exactly the mount the shipped
> compose stack creates (`SEMSOURCE_TARGET` on the host → `/workspace` in the container, defaulting to
> the repo checkout). Under compose, select a tier with
> `SEMSOURCE_CONFIG=tiers/<file>.json` (the config mount is `configs/` → `/etc/semsource/`, so the
> `tiers/` prefix is part of the value) and point `SEMSOURCE_TARGET` at the directory to index. To run
> a tier config natively, edit the `sources` paths (and `source_roots`) to real host directories.

## Tier 0 — Statistical (BM25) — works today

`tier0-statistical.json` is a complete, runnable config. BM25 is a pure-Go statistical embedder: it
ranks by term overlap, so it's solid when the query shares words with the code (identifiers, comments)
and weak on paraphrase ("where do we retry with backoff" won't match a `withRetry` helper unless the
words line up). No external service.

```bash
semsource run --config configs/tiers/tier0-statistical.json
```

> Observed while indexing a large dependency: on a big one-shot ingest, embeddings (even BM25) lag
> structural readiness. Poll `source_status` until `embedding.ready` is `true` before trusting
> `code_search` (`index.ready` gates the structural tools; the old "total_entities is stable"
> heuristic is retired — counts are no longer a readiness proxy).

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

### Capability binding — an unbound LLM capability is a supported state

The model registry maps *capabilities* (`embedding`, `query_classification`, `answer_synthesis`,
`community_summary`, …) to *endpoints*. Only bind a capability when there is an endpoint that can
actually serve it.

**These configs deliberately set no `defaults.model`.** It is a legitimate framework feature — a
catch-all for capabilities that resolve nowhere else — but a catch-all pointed at an embeddings
endpoint routes *every* unbound LLM capability there. That starts cleanly and fails only when
called, and semstreams' own guidance is to "bind the capability explicitly rather than reshaping
`defaults.model`".

Leaving an LLM capability unbound is **not** an omission to paper over. The consuming components
document and implement a non-LLM path for exactly this state:

| capability | unbound behaviour |
| --- | --- |
| `query_classification` | keyword-only classifier |
| `answer_synthesis` | template synthesis, no model |

So tier 0 and tier 1 bind `embedding` and nothing else, and that is correct, not incomplete.

Config load enforces this: a capability resolving to an endpoint that cannot serve it fails
`semsource validate` **and** `semsource run` before any component starts, naming the capability,
the endpoint, and both remedies. Every shipped config is checked by a test that discovers them,
so a config added later is covered without anyone remembering to register it.

**Tier 2 binds `query_classification` and `answer_synthesis` to `seminstruct`** — it has a
generative endpoint, so the honest binding is the real one. It does **not** bind `anomaly_review`:
that consumer runs only under clustering, which ships off, and binding a capability nothing runs
would invent coverage.

> **What this does and does not claim.** Binding these capabilities makes tier 2's configuration
> *honest*, not *proven*. They are consumed on graph-query's GraphRAG path, which `code_context` /
> `doc_context` do not use — so that path remains unexercised here, and this table should not be
> read as evidence it works end to end.

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

> **Ships OFF, and is not part of the MVP.** `enable_clustering` and `clustering_llm` are both
> `false` in this config, and **no compose file defines a `seminstruct` service** — you bring your
> own. Treat this file as a **wiring example**, not a supported profile: the capability bindings show
> what to bind if you run seminstruct, and nothing here starts by default.
>
> It previously shipped with both flags `true`, which meant selecting this config would have driven
> community summaries at `localhost:8083` with nothing listening. Flipped off rather than deleted so
> the wiring stays documented.

Tier 2 *would* add **graph-clustering** (`graph.enable_clustering: true`): pure-Go LPA community
detection over ENTITY_STATES → COMMUNITY_INDEX, lighting up the already-declared
`local`/`global`/`summary` query routes. `graph.clustering_llm: true` additionally enables GraphRAG
community summaries via the `model_registry` **`community_summary`** capability (→ seminstruct). LPA
runs with no external service; only the LLM summaries need seminstruct.

To try it, stand up seminstruct yourself and set both flags to `true`:

```bash
docker run -d -p 8083:8083 ghcr.io/c360studio/seminstruct:qwen3-0.6b   # OpenAI-compatible inference
semsource run --config configs/tiers/tier2-semantic-instruct.json
```

Requires **seminstruct** running (Rust proxy, :8083, OpenAI-compatible inference) only when
`clustering_llm` is set — and you must provide it: it is not in any compose profile.

**None of this is on the agent path.** The MCP tools (`code_context`, `code_impact`, `code_search`,
`doc_context`, `code_changes`) resolve through the fusion gateway and never reach GraphRAG,
`query_classification`, or `answer_synthesis`. An agent gets ranked, citable evidence and does its
own reasoning (ADR-0004) — which is why the MVP needs no generative model at all.

## Current state / enablement

- **Tier 0 works now** (BM25, no external service).
- **Tier 1 is the MVP default** (`configs/mvp.json`) and is what the shipped compose runs.
- **Tier 2 is wired but ships off** — see the note above: no compose service, both clustering flags
  `false`, and its GraphRAG path has no in-repo consumer. It is unexercised; do not read its row in
  the table above as evidence it works end to end.
- Tier 1/2 wiring is real (`config.ModelRegistry` → `ssCfg.ModelRegistry`; `graph.enable_clustering`
  adds graph-clustering). Validated end-to-end against local semembed: http embedder active,
  embeddings generated with 0 errors, `provenance: embedding` on `code_search`.
- **Tier-1 beats BM25 with the query prefix set** (semstreams#438 resolved, beta.129). On the dogfood
  corpus, semembed + `query_prefix` nails paraphrase queries that BM25 misses; omit the prefix and it
  regresses below BM25. A code-specialized or larger embedder is the complementary product-side lever.
