# Qualify SemSource on SemEngine Tier 0

Approved-by: Coby Leuschke (2026-11-12) sha256:8f597c039b48

## Why

The SETUP 03A SemStreams baseline merged in #213 as `3604a9ce8d5aec717253d7a905389f7910efd25d`.
SemSource's substrate migration and Tier 0 graph-foundation consumer qualification are tracked by
#221 and SemEngine #9. SemEngine 03B contracts and port admission merged in SemEngine #8 / PR #21
(`9286055e2c9a18426a73da38a60766845ab9e48e`), and the Slice 04A prerequisites (`pkg/buffer` #566,
`gateway/graph-gateway` #567, `output/websocket` #34, `processor/graph-clustering` #9, #15 broker
generation marker, #16 reserved-RPC startup refusal, and #579 no-embedder verb refusal) are closed
on SemEngine main at `7bf638643b98b4346101fabb39a1d016a7a15e7b`
(`v0.1.0-alpha.4.0.20261112145006-7bf638643b98`).

## What Changes

- Port SemSource exclusively to SemEngine (`v0.1.0-alpha.4.0.20261112145006-7bf638643b98`) and
  `semvocab v0.2.2` with zero `c360studio/semstreams` imports (#221, #231, #232, #253, #256, #257).
- Add the true no-embedder Tier 0 composition (`configs/tiers/tier0-foundation.json`,
  `embedder_type: "none"`) with truthful readiness and `not_enabled` (HTTP 501) capability refusal.
- Qualify `test/setup03a` `foundation` (`TestKnownAnswerCorpus` 64/64 and
  `TestDesiredSourceLifecycleWithoutRecovery` 33/33 + 37/37) alongside the BM25 statistical smoke
  suites (`e2e-local`, `UI smoke (dev)`, `Quickstart`), and cut `main` over to SemEngine so
  `semengine#9` can record the consumer baseline and tag `v0.1.0-alpha.5`.
- Reject retired SemStreams `graph.*` configuration keys (`gateway_bind`, `enable_playground`,
  `embedding_batch_size`, `clustering_llm`, `entity_id_edges`) with explicit migration errors, and
  record the forced `output/websocket` `ENTITY_STATES` KV-watch wire contract change in ADR-0001.

## Capabilities

### New Capabilities

- `tier0-consumer-qualification`: contract-gated, exclusive SemEngine consumer composition, exact
  graph-foundation and BM25 statistical corpus qualification, and SemEngine mainline cutover (with
  04C HTTP embedder `semengine#489` and SemConnect/dogfood tracked separately).

## Impact

SemSource owns its composition, source adapters, public consumer interfaces and known-answer workload.
SemEngine owns the substrate, public contracts and extraction admission. Mainline cuts over to
SemEngine with `semsource.json` (`embedder_type: "bm25"`) and `configs/tiers/tier0-foundation.json`
(`embedder_type: "none"`); the HTTP neural embedder (`semengine#489`, 04C) and LLM community
summaries (`semengine#485`) remain tracked as follow-up slices.

## Non-goals

- Implementing or editing SemEngine packages or importing both substrates in one binary.
- Claiming 04C neural (`semembed`) or Tier 2 LLM community-summary qualification before
  `semengine#489` and `semengine#485` land upstream.
- Treating SemSource code/docs corpus qualification as a substitute for SemEngine's separate
  SemConnect reference and engine dogfood obligations on `semengine#9`.
