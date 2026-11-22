# Qualify SemSource on SemEngine Tier 1 — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` and `design.md` (2026-11-21, sha256:fc1a4c524e4c; line entered by claude at the owner's instruction).
- [x] 1.2 Open questions ruled by the owner on 2026-11-21: the switch is the `graph.semantic_search` boolean (design decision 1), and a Tier 1 boot with an unreachable provider stays up degraded (decision 6). Both match the design as written.

## 2. Verify before code

- [x] 2.1 Fusion NL at alpha.6 is **word-only**. `fusionnats.Client.Resolve` sends `ResolveModeNL` to `resolveWords`, which asks only `graph.lexical.query.search` (`pkg/fusion/fusionnats/client.go:100-101,148-168`), and the verb table has no semantic row (`:23-27`). Design decision 3's word-only branch applies: Tier 1 changes `graph_search` only, and `code_search` / `code_context` / `doc_context` are unchanged. Filed upstream as semengine#706.
- [x] 2.2 graph-embedding catches up a retained store from source. Its `graph/follower` replays the stored `ENTITY_STATES` entities and then the live writes (`graph/follower/follower.go:1-3`; `processor/graph-embedding/README.md:35`), so switching a retained Tier 0 store to Tier 1 should need no reseed. To be confirmed by measurement in 5.1 before any doc promises it.
- [x] 2.3 tools/list is 7,603 B of 8,192 today (`graph_search` 1,030 B, of which 728 B is description). The new fields are reply fields, not input schema, so they cost only the description text about the merged score scale. That fits in the 589 B of headroom.

## 3. Composition and config

- [ ] 3.0 Owner ruling 2026-11-21: implementation waits for an answer on semengine#706 (fusion nl stays word-only at alpha.6). Hold: OPEN-Q semengine#706 must be answered before implementation
- [ ] 3.1 `graph.semantic_search` with its validation (embedding capability resolves; lexical on); tests for both refusals.
- [ ] 3.2 Compose graph-embedding via `RegisterWithEmbedder` + `httpembedder`, set `embedding_enabled`, generation 6 and its fingerprint.
- [ ] 3.3 Make the three "no graph-embedding" tests tier-conditional.
- [ ] 3.4 Restore `configs/tiers/tier1-semantic.json`; document `docker compose --profile semantic up`.
- [ ] 3.5 Readiness: the semantic index reports unavailable until graph-embedding is ready.

## 4. graph_search

- [ ] 4.1 Merged `relevance`, `word_rank`, `vector_rank` and the `merge` disclosure; description states the score scale.
- [ ] 4.2 `TestToolsListSchemaBudget` passes.

## 5. Acceptance

- [ ] 5.1 `test/setup03a` neural lane on the restored config, and `TestProviderFailureAndRecovery`.
- [ ] 5.2 Pin and record the provider identity.
- [ ] 5.3 Scorecard Tier 1 arm (dogfood, OSH, loose band) vs #230 and the alpha.6 Tier 0 run; #178 OSH tail remeasured.
- [ ] 5.4 Independent go-component and graph-event review; post the verdict on semengine#11.

## 6. Close

- [ ] 6.1 Archive this change before the implementing PR is marked ready.
