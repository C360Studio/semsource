# SemSource query tiers

SemSource's retrieval quality — especially `code_search` (natural-language) — depends on which
enrichment layer is running. The layers stack; each adds capability on top of the one below.

**Tier 0 is the tier with no external provider**: nothing outside the SemSource process and NATS runs, and
no embedder is composed. It has two compositions — Structural (`graph.lexical_search: false`) and Lexical
(the default: `graph-lexical`, a BM25 word index).

| Tier | Adds | `code_search` (NL) | External service | Runnable in semsource today |
|------|------|--------------------|------------------|------------------------------|
| **0 — Structural** | graph-ingest / index / query / gateway | refused: `501 not_enabled` (byName / prefix / relations / impact only) | none | ✅ (`tier0-foundation.json`, `graph.lexical_search: false`) |
| **0 — Lexical** (default) | graph-lexical (**BM25** word index) | keyword-statistical: good for term overlap, weak for paraphrase | none | ✅ (`mvp.json`, `tier0-statistical.json`) |
| **1 — Semantic** | graph-embedding (**HTTP → semembed**), merged with the word index | word and embedding hits merged by rank, paraphrase-robust | **semembed** (:8081, OpenAI-compatible embeddings) | ✅ opt-in (`tier1-semantic.json`, `graph.semantic_search: true`); acceptance is [#234](https://github.com/C360Studio/semsource/issues/234) |
| **2 — Generative** | graph-clustering + LLM community summaries (**seminstruct**) | + community/summary reasoning | **seminstruct** (:8083, inference proxy) | ❌ LLM summaries are not shipped ([semengine#485](https://github.com/C360Studio/semengine/issues/485)); no config ships |

The structural verbs — `code_context`, `code_impact`, and byName/prefix resolution — don't depend on the word
index or an embedder and work at every tier. `doc_context` needs a source that produces documents (see
[Tier 0 — Structural](#tier-0--structural-tier0-foundationjson--works-today)). The tier only changes
`code_search` NL quality and (Tier 2) community/summary features.

> **On `semengine v0.1.0-alpha.7` Tier 0 is the default and Tier 1 is opt-in.** Tier 0 word search is
> `graph-lexical`, on by default; `graph.embedder_type` is retired and refused at load. Tier 1 is
> `graph.semantic_search: true` with a `model_registry` embedding endpoint (`tier1-semantic.json`); its
> qualification against the measured floors is [#234](https://github.com/C360Studio/semsource/issues/234).
> LLM community summaries wait on [semengine#485](https://github.com/C360Studio/semengine/issues/485); the
> Tier 2 section below describes the generative wiring as it stood on the SemStreams pin and is not
> runnable here.

> **Paths in these files are Docker-container paths** (`/workspace`) — exactly the mount the shipped
> compose stack creates (`SEMSOURCE_TARGET` on the host → `/workspace` in the container, defaulting to
> the repo checkout). Under compose, select a tier with
> `SEMSOURCE_CONFIG=tiers/<file>.json` (the config mount is `configs/` → `/etc/semsource/`, so the
> `tiers/` prefix is part of the value) and point `SEMSOURCE_TARGET` at the directory to index. To run
> a tier config natively, edit the `sources` paths (and `source_roots`) to real host directories.

## Tier 0 — Structural (`tier0-foundation.json`) — works today

`tier0-foundation.json` sets `graph.lexical_search: false`: SemSource composes graph-ingest, graph-index,
graph-query, and the gateway, and no word index. It is a complete, runnable config with no external service.

```bash
semsource run --config configs/tiers/tier0-foundation.json
```

- `POST /code-context/search` and `POST /doc-context/search` answer HTTP `501` with
  `error.code: "not_enabled"` and `retryable: false`, not a transient outage.
- The `embedding` block on `/source-manifest/status` reports `state: "disabled"` with
  `reason.code: "not_enabled"` instead of a readiness figure.
- The structural verbs are unaffected; gate them on `index.ready`.

> **`doc_context` needs a docs source.** Both shipped Tier 0 configs (`tier0-foundation.json` and
> `tier0-statistical.json`) configure only an `ast` source, so `doc_context` answers
> "no boot sources configured for docs retrieval" on them: the docs lens is scoped to the systems of
> `docs`, `config`, `url`, and `s3` sources (and the ones a `repo` source expands to), and none exist.
> Add one to your copy of the config, or use `mvp.json`, which indexes docs and config files too. The
> shipped JSON is unchanged.

## Tier 0 — Lexical (BM25) — works today

`tier0-statistical.json` is a complete, runnable config; `mvp.json` (the compose default) adds `docs` and
`config` sources. `graph-lexical` keeps an in-memory BM25 word index of each entity's text, rebuilt from
`ENTITY_STATES` at every start, and sends nothing to a model or service. It ranks by term overlap, so it's
solid when the query shares words with the code (identifiers, comments) and weak on paraphrase ("where do
we retry with backoff" won't match a `withRetry` helper unless the words line up).
`tier0-statistical.json`, like `tier0-foundation.json`, indexes only an `ast` source (see the
`doc_context` note above).

```bash
semsource run --config configs/tiers/tier0-statistical.json
```

> Observed while indexing a large dependency: on a big one-shot ingest, the word index lags structural
> readiness. Poll `source_status` until `embedding.ready` is `true` before trusting `code_search` (the
> `embedding` block reports the `graph-lexical` index; `index.ready` gates the structural tools; the old
> "total_entities is stable" heuristic is retired — counts are no longer a readiness proxy).

## Tier 1 — Semantic (semembed) — opt-in

`tier1-semantic.json` sets `graph.semantic_search: true` and binds the `model_registry` **`embedding`**
capability to semembed. SemSource then composes **graph-embedding** (registered with an HTTP embedder
built from that endpoint) beside graph-lexical, and sets graph-query's `embedding_enabled`, so
`searchGraph` and fusion's natural-language resolve merge the word list and the embedding list by rank.
Validation refuses `graph.semantic_search: true` unless `embedding` resolves to an embedding endpoint
and `graph.lexical_search` is on, so a Tier 1 config never boots silently as Tier 0. With the compose
stack, start semembed with the `semantic` profile:

```bash
SEMSOURCE_CONFIG=tiers/tier1-semantic.json docker compose --profile semantic up
```

**Set no `query_prefix` for semembed.** The embedder sends `input_type` (`query` or `document`) with
every request, and semembed applies the model's own query instruction (arctic: `"Represent this sentence
for searching relevant passages: "`). A `query_prefix` is only for a provider that does not apply the
instruction itself.

```jsonc
{
  "namespace": "acme",
  "sources": [ /* ... */ ],
  "graph": { "semantic_search": true },
  "model_registry": {
    "endpoints": {
      "semembed": { "provider": "openai", "url": "http://semembed:8081/v1", "model": "Snowflake/snowflake-arctic-embed-s" }
    },
    "capabilities": { "embedding": { "preferred": ["semembed"] } }
  }
}
```

**Switching a running deployment to Tier 1:** a booted deployment keeps its retained composition until the
file's `config_revision` exceeds the retained one, so raise `config_revision` with the switch or the
retained Tier 0 wiring stays. Whether graph-embedding backfills a retained store is still being measured
(#234); until then, reseed.

The URL must end in `/v1` (the client adds `/embeddings`), and a reply naming another model is refused.
To run semembed outside compose, point the URL at `http://localhost:8081/v1`:

```bash
docker run -d -p 8081:8081 ghcr.io/c360studio/semembed:latest   # OpenAI-compatible embeddings
```

> **Resource footprint — semembed is CPU-heavy on the first index (local-dev caveat).** semembed runs
> real embedding inference (fastembed-rs / ONNX, arctic-embed-s). Embedding a full initial index is a
> burst of inference and, by default, ONNX fans out across every core: **measured ~561% CPU (≈5.6 cores)
> embedding a 21.5k-entity graph on a laptop** — fine on a server, rough on a dev machine. It's a
> **one-time first-index cost** (vectors land in `EMBEDDING_INDEX`, content hashes in `EMBEDDING_DEDUP`;
> steady-state watch is cheap), not continuous. Levers if it's hammering your machine:
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
called, and semstreams' own guidance was to "bind the capability explicitly rather than reshaping
`defaults.model`".

Leaving an LLM capability unbound is **not** an omission to paper over. The consuming components
document and implement a non-LLM path for exactly this state:

| capability | unbound behaviour |
| --- | --- |
| `query_classification` | keyword-only classifier |
| `answer_synthesis` | template synthesis, no model |

So a tier 1 config binds `embedding` and nothing else (tier 0 binds nothing), and that is correct, not
incomplete.

Config load enforces this: a capability resolving to an endpoint that cannot serve it fails
`semsource validate` **and** `semsource run` before any component starts, naming the capability,
the endpoint, and both remedies. Every shipped config is checked by a test that discovers them,
so a config added later is covered without anyone remembering to register it.

**A tier 2 config would bind `query_classification` and `answer_synthesis` to `seminstruct`** — it
would have a generative endpoint, so the honest binding is the real one. It would **not** bind
`anomaly_review`: anomaly detection is not ported (graph-clustering refuses `enable_anomaly_detection`),
and binding a capability nothing runs would invent coverage.

> **What this does and does not claim.** Binding these capabilities would make a tier 2 configuration
> *honest*, not *proven*. They are consumed on graph-query's GraphRAG path, which `code_context` /
> `doc_context` do not use — so that path remains unexercised here, and this table should not be
> read as evidence it works end to end.

> **The query instruction still matters; semembed applies it.** arctic/BGE/E5 are *asymmetric*: the
> query must carry an instruction while documents are embedded raw. On semembed the instruction comes from
> `input_type`, so no `query_prefix` is set. **Measured on the SemStreams pin (dogfood, 21k entities),
> when the prefix was set by the product:** the arctic relevant-vs-noise cosine margin ~doubled
> (+0.090 → +0.158), and `code_search` beat tier-0 BM25 on paraphrase queries whose words don't appear in
> the code (e.g. *"prevent processing the same message twice"* → `publish_msgid_integration_test.go`).
> Without the instruction tier 1 ranked *below* BM25. Re-measured for this pin in #234.

**Scores at Tier 1 are on a different scale.** A merged score is the sum of 1/(60 + place) over the two
lists, at most 2/61. `graph_search` reports it as `relevance` with each match's `word_rank` and
`vector_rank`, and a `merge` block; `merge.honoured: false` means the embedding search failed and the
matches are the word list alone. The fusion tools report `provenance: embedding` when a returned seed came
from the embedding list, and `lexical` otherwise. A client that thresholds `relevance` on the word scale
breaks at Tier 1.

**An unreachable provider leaves SemSource up, degraded.** Search answers from words with the merge not
honoured, and the readiness `embedding` block (on `/source-manifest/status` and `source_status`) and the
`semantic_index` capability stay not ready until graph-embedding reports ready.

## Tier 2 — Generative (seminstruct) — not shipped

> **Nothing LLM-backed runs on this pin.** LLM community summaries are not ported
> ([semengine#485](https://github.com/C360Studio/semengine/issues/485)): `graph.clustering_llm` and
> `graph.entity_id_edges` are refused at load, no Tier 2 config ships, and the Tier 2 compose overlay
> (`docker-compose.tier2-dev.yml`) was removed. What does run is `graph.enable_clustering: true`, which
> composes `graph-clustering` with no external provider.

`graph.enable_clustering: true` adds **graph-clustering**: communities are entity-ID prefix groups over
`ENTITY_STATES`, kept in `COMMUNITY_INDEX` with a statistical summary (ADR-099), which graph-query's
`localSearch`, `globalSearch`, and `searchGraph` read. The generative part — LLM community summaries via
the `model_registry` **`community_summary`** capability (→ seminstruct) — is what semengine#485 tracks.

`graph.entity_id_edges` went with label propagation: graph-clustering now synthesizes no edges, so the
sibling and system-peer tuning measured on the SemStreams pin
([`docs/testing/tier-baselines.md`](../../docs/testing/tier-baselines.md)) no longer applies.

The seminstruct container the earlier wiring targeted still runs on its own; nothing in SemSource calls it
on this pin:

```bash
docker run -d -p 8083:8083 ghcr.io/c360studio/seminstruct:qwen3-0.6b   # OpenAI-compatible inference
```

**The MCP surface is two families.** The *fusion* tools (`code_context`, `code_impact`,
`code_search`, `doc_context`, `code_changes`) resolve through the fusion gateway and never reach
GraphRAG, `query_classification`, or `answer_synthesis`. An agent gets ranked, citable evidence and
does its own reasoning (ADR-0004) — which is why the MVP needs no generative model at all.

The *graph-query* tools (`graph_summary`, `graph_search`) reach the substrate's query surface
directly. Neither is gated on clustering: `graph.query.summary` is a discovery resolver, and
`graph.query.searchGraph` always starts with a word search over `graph-lexical`, then reads the
community groups that hold the matches when clustering has built them. So `graph_search` needs the
word index: under the structural composition (`graph.lexical_search: false`) there is no
`graph-lexical` to search and the call fails rather than returning entity hits. Where it runs,
`graph_search` discloses the rung it reached in a `retrieval` block:

| Composition | `graph_search` returns | Disclosed rung |
| ----------- | ---------------------- | -------------- |
| Tier 0 structural (`lexical_search: false`) | An error: no word index to search | — |
| Tier 0 lexical, no community records for the matches | Word-search matches. No community summaries | `entities_only`, `community_backed: false` |
| Lexical + clustering, community records read for the matches | \+ community summaries | `community_summaries`, `community_backed: true` |

Enabling `graph.enable_clustering` does not by itself make an answer community-backed: the rung
reflects whether the reply actually carried community records for the matched groups.

Read the disclosure before treating results as community evidence: a non-community answer is a
word-search hit list, not thematic reasoning. The answer text, when one comes back, is a deterministic
template built from the results (`answer_source: template`, else `none`) — never LLM-synthesized, because
the pinned substrate synthesizes none ([semengine#485](https://github.com/C360Studio/semengine/issues/485)).

**Answering never requires an LLM.** Query classification is a tiered chain whose first tier is a
deterministic keyword classifier; a keyword match bypasses the LLM tier entirely, and the LLM tier
engages only when a `model_registry` `query_classification` capability exists — which SemSource does
not currently expose, so it is off in every deployment.

## Current state / enablement

- **Tier 0 (Lexical) is the MVP default** (`configs/mvp.json`, BM25 with no external service) and is what
  the shipped compose runs until Slice 04C (`semengine#489`,
  [#234](https://github.com/C360Studio/semsource/issues/234)) lands. Structural (`tier0-foundation.json`)
  is the other Tier 0 composition.

> **Measured baselines:** [`docs/testing/tier-baselines.md`](../../docs/testing/tier-baselines.md)
> records boot-to-ready timings and answer-quality observations from a real 6,685-entity Java corpus
> — including the finding that LPA clustering put 82% of entities in one community, which made
> tier-2 "community-backed" answers weak on code. That was measured on the SemStreams LPA
> implementation; SemEngine groups communities by entity-ID prefix, so it does not carry over. Read it
> before recommending a tier.

- **Tier 2 is not shipped** — see the note above: no config or compose overlay, and LLM community
  summaries are [semengine#485](https://github.com/C360Studio/semengine/issues/485). `graph.enable_clustering`
  composes graph-clustering, but the community-backed rung of `graph_search`'s disclosure is covered by
  unit tests over substrate-shaped replies (`processor/mcp-gateway/disclosure_test.go`), not by a live
  clustered stack.
- **Tier 1 is opt-in** (`tier1-semantic.json`, `graph.semantic_search: true`). Its acceptance against
  the measured floors (setup03a's neural lane and the scorecard Tier 1 arm) is
  [#234](https://github.com/C360Studio/semsource/issues/234); until it lands, treat Tier 1 answer quality
  on this pin as unmeasured.
