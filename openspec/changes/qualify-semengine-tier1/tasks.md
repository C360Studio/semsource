# Qualify SemSource on SemEngine Tier 1 — tasks

## 1. Approval

- [ ] 1.1 Owner approves `proposal.md` and `design.md`, including the open questions. Hold: OPEN-Q proposal awaits owner approval

## 2. Verify before code

- [ ] 2.1 Determine whether `pkg/fusion/fusionnats` at alpha.6 resolves nl seeds through the merged `searchGraph` or only `graph.lexical.query.search`; record the answer in `design.md` decision 3.
- [ ] 2.2 Determine whether graph-embedding backfills vectors from an existing `ENTITY_STATES`; record it under Risks.
- [ ] 2.3 Measure the tools-list size with the new `graph_search` fields against the 8 KB budget.

## 3. Composition and config

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
