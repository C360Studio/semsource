# Qualify SemSource on SemEngine Tier 1 — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` and `design.md` (2026-11-21, sha256:fc1a4c524e4c; line entered by claude at the owner's instruction).
- [x] 1.2 Open questions ruled by the owner on 2026-11-21: the switch is the `graph.semantic_search` boolean (design decision 1), and a Tier 1 boot with an unreachable provider stays up degraded (decision 6). Both match the design as written.

## 2. Verify before code

- [x] 2.1 Fusion NL at alpha.6 is **word-only**. `fusionnats.Client.Resolve` sends `ResolveModeNL` to `resolveWords`, which asks only `graph.lexical.query.search` (`pkg/fusion/fusionnats/client.go:100-101,148-168`), and the verb table has no semantic row (`:23-27`). Design decision 3's word-only branch applies: Tier 1 changes `graph_search` only, and `code_search` / `code_context` / `doc_context` are unchanged. Filed upstream as semengine#706. **Superseded at alpha.7** (2026-11-22): semengine#706 closed; fusion `nl` asks `graph.query.mergedSearch`, so decision 3 now covers the fusion tools.
- [x] 2.2 graph-embedding catches up a retained store from source. Its `graph/follower` replays the stored `ENTITY_STATES` entities and then the live writes (`graph/follower/follower.go:1-3`; `processor/graph-embedding/README.md:35`), so switching a retained Tier 0 store to Tier 1 should need no reseed. To be confirmed by measurement in 5.1 before any doc promises it.
- [x] 2.3 tools/list is 7,603 B of 8,192 today (`graph_search` 1,030 B, of which 728 B is description). The new fields are reply fields, not input schema, so they cost only the description text about the merged score scale. That fits in the 589 B of headroom.
- [x] 2.4 Pin moved to `v0.1.0-alpha.7`: test doubles return `fusion.Names` / `fusion.Neighbors`; `query-pattern` fork reconciled; `nlDisabledClient` comment names `mergedSearch`.

## 3. Composition and config

- [x] 3.0 semengine#706 answered by alpha.7 (2026-11-22); owner re-approved the alpha.7 revision the same day (sha256:2f717b5b2e6e; line entered by claude at the owner's instruction).
- [x] 3.1 `graph.semantic_search` with its validation (embedding capability resolves; lexical on); tests for both refusals. Evidence: `config/config.go:36,311` (`validateSemanticSearch`, run after the registry check); refusals for no registry, embedding unbound, embedding bound to a chat endpoint and lexical off in `config/semantic_search_test.go:16`; review fix: offline URL check (http/https, host, `/v1`), `checkEmbeddingURL` in `config/config.go`.
- [x] 3.2 Compose graph-embedding via `RegisterWithEmbedder` + `httpembedder`, set `embedding_enabled`, generation 6 and its fingerprint. Evidence: `cmd/semsource/embedder.go:22,33` (embedder from the retained registry, `desired_config.go`), `cmd/semsource/run.go:537,813,907`; tier 0 fingerprint unchanged at 6 and a tier 1 fixture fingerprint, `cmd/semsource/composition_fingerprint_test.go:24,32`; the composed config constructs through the factory, `cmd/semsource/embedder_test.go` (`TestTierOneGraphEmbeddingConfigIsAccepted`).
- [x] 3.3 Make the three "no graph-embedding" tests tier-conditional. Evidence: `cmd/semsource/run_test.go:364,426`, `cmd/semsource/migration_contract_test.go:77` (tier 0 asserts absence; tier 1 asserts presence, `embedding_enabled: true` and matching text suffixes).
- [x] 3.4 Restore `configs/tiers/tier1-semantic.json`; document `docker compose --profile semantic up`. Evidence: `configs/tiers/tier1-semantic.json` (semembed `http://semembed:8081/v1`, arctic-embed-s, no `query_prefix`); `configs/tiers/README.md:89`, and the semembed path no longer says to set `query_prefix`.
- [x] 3.5 Readiness: the semantic index reports unavailable until graph-embedding is ready. Evidence: `processor/source-manifest/readiness.go:56` (`foldSemanticReadiness`), `workbench_capabilities.go:333` (capabilities and `/status`), MCP `source_status` via `SemanticReadinessJSON`; tests `processor/source-manifest/semantic_readiness_test.go:30` and `processor/mcp-gateway/nats_integration_test.go:219`. Readiness is index coverage, not provider liveness (owner ruling 2026-11-22, #234; spec delta and design decision 6 narrowed).

## 4. graph_search

- [x] 4.1 Merged `relevance`, `word_rank`, `vector_rank` and the `merge` disclosure; description states the score scale. Evidence: `processor/mcp-gateway/graph_matches.go:72,261`, `query_tools.go:131`, retrieval note in `disclosure.go`; live graph-query + graph-embedding test `processor/mcp-gateway/graph_search_tier1_integration_test.go:85` (honoured and `honoured:false`); review fix: the embedding-only answer (`text: null`, merge `index_not_ready`) renders `data.semantic` in vector order with hints kept, `buildGraphSearchResult` in `graph_matches.go`, test `TestIntegration_GraphSearchRendersEmbeddingOnlyAnswer`.
- [x] 4.2 Fusion tools pass `provenance` through, and their descriptions state the Tier 1 `similarity` scale. Evidence: `processor/code-context/provenance_integration_test.go:28` (real fusionnats over `mergedSearch`: `embedding`, `lexical`, unhonoured merge `lexical`); descriptions in `processor/mcp-gateway/component.go`, pinned by `graph_tools_test.go:67`.
- [x] 4.3 `TestToolsListSchemaBudget` passes. Evidence: the gate now measures the marshalled tools/list result, 8,141 of 8,192 B (the earlier per-tool sum read 8,121 and missed the list framing; the scorecard's 8,175 B adds the JSON-RPC envelope). Was 7,603; `graph_search` 1,215 B.

## 5. Acceptance

- [x] 5.1 `test/setup03a` neural lane on the restored config, and `TestProviderFailureAndRecovery`, which asserts the actual recovery behaviour: answers report `merge.honoured: false` while the provider is down, failed entities are not re-embedded until a restart (semengine#724), and a restart re-embeds what hop 1 replays before the first reply (semengine#722). Evidence (`60a1565`): neural lane 72/72 live (`docs/testing/setup-03a/tier1-neural-report.json`); provider faults 15/15 live, stall bounded at 4,023 ms, no retry over 65 s, re-embed partial (`tier1-provider-faults-report.json`).
- [x] 5.2 Pin and record the provider identity. Evidence: semembed `0.2.1@sha256:22ea57c6…` pinned (`a1bf7f2`; owner ruling on #234, the earlier pin ignored `input_type`); identity in `docs/testing/setup-03a/tier1-provider-identity.json`.
- [x] 5.3 Scorecard Tier 1 arm (dogfood, OSH, loose band) vs #230 and the alpha.6 Tier 0 run, fusion tools included; #178 OSH tail remeasured. Evidence (`SUMMARY-tier1-alpha7.md`): dogfood 33/36, loose 6/7, OSH 13/13, no band regresses vs #230, alpha.6 T0 or fix329 T0. The fresh OSH seed fits at 82.6% of the GRAPH ceiling; a warm restart within `max_age` breaches it (#160 wedge, not specific to Tier 1). Owner ruling 2026-11-22: recorded on #160/#178, does not block acceptance.
- [x] 5.4 Independent go-component and graph-event review; post the verdict on semengine#11. Evidence: go-component-reviewer APPROVE at bacd2cc (budget-gate minor fixed in 891552b); graph-event-reviewer APPROVE at bacd2cc (F1 fixed in 3743a3f, doc nits in 963d6b7). semengine#11 verdict: see the PR.

## 6. Close

- [x] 6.1 Archive this change before the implementing PR is marked ready.
