# ADR-0002: Tier Support and Model Registry Passthrough

> **Status:** Accepted (Statistical Tier Implemented) | **Date:** 2026-03-12

## Context

Semstreams alpha.29 centralizes LLM and embedding endpoint resolution through a `model_registry` config section, replacing per-component fields (`llm_endpoint`, `llm_model`, `embedder_url`). SemSource currently wires only structural-tier graph components and is unaffected by this breaking change.

The semstreams tier architecture provides three levels of graph processing:

| Tier | Components Added | External Services |
|------|-----------------|-------------------|
| Structural | graph-ingest, graph-index, graph-query, graph-gateway | None |
| Statistical | + graph-embedding (BM25), graph-clustering (LPA) | None (pure Go) |
| Semantic | + graph-embedding (HTTP), graph-clustering (LLM) | semembed, seminstruct |

SemSource may want to offer optional tier escalation so users can get embeddings and community detection on their code/doc knowledge graph without running a separate semstreams instance.

## Decision

**Implement statistical tier. Defer semantic tier until semembed/seminstruct infrastructure is needed.**

Statistical tier is now wired by default:

- `graph-embedding` (BM25) is registered in `graphSubsystemComponents()` with `coalesce_ms: 200`.
- `graph-index` also uses `coalesce_ms: 200`.
- No external services are required — BM25 embeddings and LPA clustering are pure Go.

When semantic tier is needed, the implementation path is:

1. Add `tier` and `model_registry` fields to `config.Config`.
2. Extend `graphSubsystemComponents()` in `run.go` to conditionally wire the HTTP-backed
   `graph-embedding` and LLM-backed `graph-clustering`.
3. Pass `model_registry` through to the semstreams config builder.

Model registry passthrough is available in the semstreams config builder but not yet
exposed in the semsource config surface.

## Consequences

### Enrichment Architecture Clarification

Enrichment components write to **separate KV buckets**, not to ENTITY_STATES:

| Component | Output KV |
|-----------|----------|
| graph-embedding | `EMBEDDINGS_CACHE` |
| graph-clustering | `COMMUNITY_INDEX` |

ENTITY_STATES holds raw entity triples regardless of tier. The WebSocket export (which consumes the raw GRAPH stream) is unaffected by tier — it always exports raw entities. Enrichment data is accessed via GraphQL/MCP, not the WebSocket stream.

### Service Choices

When semantic tier is implemented, the recommended services are:

- **semembed** (Rust, port 8081) — OpenAI-compatible embedding API via fastembed-rs
- **seminstruct** (Rust proxy, port 8083) — OpenAI-compatible inference proxy to shimmy

Both are swappable for any OpenAI-compatible service via the model_registry abstraction.

### Statistical Tier as Stepping Stone

Statistical tier requires no external services (BM25 embeddings and LPA clustering are pure Go). This is a natural first step before committing to semembed/seminstruct infrastructure.

## Related

- ADR-0001: WebSocket Output Exports Raw Entities Before ENTITY_STATES
- `cmd/semsource/run.go` — `graphSubsystemComponents()` wiring
- `config/config.go` — Config struct
- semstreams `configs/statistical.json`, `configs/semantic.json` — reference tier configs
- semstreams `docs/operations/migration-alpha29.md` — model registry migration guide
