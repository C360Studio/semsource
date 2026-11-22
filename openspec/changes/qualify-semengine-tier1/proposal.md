# Qualify SemSource on SemEngine Tier 1 (neural search)

Approved-by: Coby Leuschke (2026-11-22) sha256:2f717b5b2e6e

Track: design (adds a capability, touches the composition boot path and the composition
generation, and changes the MCP `graph_search` answer at Tier 1).

## Why

`v1.0.0-beta.10` ships Tier 0 only: structural queries plus `graph-lexical` word search, with
embeddings off (#237, ADR-0013). SemEngine `v0.1.0-alpha.6` now supplies the Tier 1 pieces:
`graph/embedding/httpembedder`, `graph-embedding.RegisterWithEmbedder`, and a `searchGraph` that
merges the word and vector lists by reciprocal rank when `embedding_enabled` is true, falling back
to the word answer when the provider fails (semengine#489, #666, #668 closed). `v0.1.0-alpha.7` routes
fusion's `nl` resolve through the same merge (`graph.query.mergedSearch`, semengine#706) and has the
embedder send `input_type`, so semembed applies the model's query instruction itself (semengine#686). #234 tracks
qualifying it. The #230 baseline gives the Tier 1 floor: dogfood 32/36, OSH 11/13.

## What Changes

- An opt-in Tier 1 composition. A configuration whose `model_registry` resolves the `embedding`
  capability to a real embedder, plus `graph.semantic_search: true`, composes graph-embedding
  through `RegisterWithEmbedder` and `httpembedder`. It also sets graph-query
  `embedding_enabled: true` and bumps `compositionGeneration` to 6. The default composition stays
  Tier 0.
- Restore `configs/tiers/tier1-semantic.json`. Use the `semantic` compose profile's pinned
  semembed, which is still in `docker-compose.yml`. Make the "no graph-embedding" tests
  tier-conditional.
- Move the SemEngine pin to `v0.1.0-alpha.7`.
- Fusion tools at Tier 1 (`code_search`, `code_context`, `doc_context`) seed from the merged list
  with no SemSource routing; replies carry `provenance`, and descriptions state the merged
  `similarity` scale.
- MCP `graph_search` at Tier 1: `relevance` is the merged score, and each match carries
  `word_rank` / `vector_rank`. A failed merge is disclosed instead of being passed off as a
  merged answer. All of this stays within the 8 KB tools-list budget.
- Provider identity is pinned and recorded. A configured-but-missing provider counts as
  unavailable evidence, never a pass. Search keeps answering on the word list.
- Acceptance: the `test/setup03a` neural lane, the provider-fault cases, and the full scorecard
  Tier 1 arm (dogfood, OSH, loose band), graded against #230 and the alpha.6 Tier 0 run (#301), fusion tools included.
  The verdict is posted on semengine#11.

## Capabilities

### New Capabilities

- `tier1-consumer-qualification`: the opt-in neural composition, its provider-identity and fault
  obligations, the merged `graph_search` disclosure, and the Tier 1 acceptance floor.

## Impact

- `cmd/semsource/run.go` (composition, generation 6), `config/config.go` (`graph.semantic_search`
  and its registry check), `processor/mcp-gateway/graph_matches.go`, `configs/tiers/`,
  `test/setup03a`, `scripts/scorecard/`.
- No change to the default composition or entity identity. Whether a retained Tier 0 store can turn
  Tier 1 on without a reseed is an open question in `design.md`.
- The #178 OSH tail against the GRAPH stream ceiling is remeasured as part of acceptance.
- Out of scope: LLM community summaries (semengine#485), and the upstream gaps semengine#667 and #683,
  which stay tracked upstream.
