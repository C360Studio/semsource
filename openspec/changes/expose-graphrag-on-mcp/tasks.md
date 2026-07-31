# Tasks

## 1. Route the two tools

- [ ] 1.1 Add `GraphSummaryInput` and `GraphSearchInput` typed argument structs in
      `processor/mcp-gateway/query_tools.go`. `graph_search` starts query-only (design — Open
      Questions); `graph_summary` takes no required argument. Validate a non-empty query the way
      `fusionQuery` already does.
- [ ] 1.2 Add `graphSummary` and `graphSearch` handlers routing through the existing `c.request`
      (`RequestClassified`, so ADR-060 errors keep surfacing as `isError`) to
      `graph.query.summary` and `graph.query.searchGraph`.
- [ ] 1.3 Register both in `buildServer()` (`component.go`). Descriptions must: name the tool as a
      substrate graph query rather than a fusion answer; draw the line against `code_search` in the
      first clause; state that the result discloses the rung it reached; promise no natural-language
      query understanding.
- [ ] 1.4 Add both subjects to the gateway's declared ports if the component advertises its
      request subjects, keeping discovery metadata truthful.

## 2. Derive the disclosure

- [ ] 2.1 Add a disclosure deriver reading only fields the substrate returns:
      `community_summaries`/`community_id` → community-backed; `answer` → an answer is present;
      `answer_model` → LLM-synthesized; `degraded`+`degraded_reason` → carried through verbatim.
- [ ] 2.2 **`answer_model` is the only LLM discriminator.** `degraded` is false by design on a
      template-only deployment (`answer.go:44-49`); it MUST NOT be used to infer template vs LLM.
- [ ] 2.3 Attach the disclosure **alongside** the substrate payload, which passes through verbatim —
      never merged into or rewriting it, so a future substrate `strategy` can be preferred over the
      derived rung.
- [ ] 2.4 Leave `graph_summary` results as pure passthrough: it has no retrieval rung to disclose.

## 3. Prove it hermetically

- [ ] 3.1 Table test over the three rungs from recorded substrate responses: no clustering
      (entity hits only), clustering without LLM (community summaries + template answer,
      `degraded:false`, empty `answer_model`), clustering with LLM (`answer_model` set).
- [ ] 3.2 Regression test pinning task 2.2: a response with `degraded:false` and empty
      `answer_model` MUST NOT be reported as LLM-synthesized. This is the trap the whole disclosure
      exists to avoid — it must fail loudly if the derivation is ever simplified.
- [ ] 3.3 Test that the substrate payload survives verbatim through the gateway.
- [ ] 3.4 Test argument validation and that a downstream ADR-060 error still maps to `isError`, and
      that a graph-query success is NOT rejected for lacking `contract_version`.
- [ ] 3.5 Extend the default-component/roster pinning test so dropping either registration fails CI.

## 4. Prove it on the default stack

- [ ] 4.1 Extend the existing tier-0 compose smoke with one real `tools/call` per new tool,
      asserting a non-empty answer — name-listing does not count as coverage.
- [ ] 4.2 Assert the not-community-backed disclosure is present on that tier-0 answer, and that
      neither tool refuses.
- [ ] 4.3 Add a query that only `graph_search` should win and only `code_search` should win, so a
      description regression that makes them confusable is visible.

## 5. Correct the record

- [ ] 5.1 New ADR revising the ADR-0004 MCP boundary: fusion tools stay deterministic and
      fusion-backed; graph-query tools are a second family, ungated, disclosing their rung. Add a
      pointer in ADR-0004 — do not rewrite it.
- [ ] 5.2 `configs/tiers/README.md` — replace the claim that MCP tools never reach GraphRAG with the
      capability ladder, and state that clustering is off in every shipped tier config except
      `tier2-compose-dev.json`.
- [ ] 5.3 `scripts/scorecard/README.md` — describe the new family and state why the graded scorecard
      still does not measure it (a drifting judge cannot support an A/B).
- [ ] 5.4 README advertised-surface matrix — add both tools with their named test evidence.

## 6. File the upstream asks

- [ ] 6.1 Ask: populate `GlobalSearchResponse.Strategy` on all `globalSearch` paths. The strategy is
      already computed and metered to Prometheus (`graphrag.go:629-632`) but reaches the wire in
      exactly one place, the `searchGraph` semantic fallback (`searchgraph.go:219`); all 12 response
      constructions in `graphrag.go` leave it empty. Record in `docs/upstream/semstreams-asks.md`
      and open a semstreams issue.
- [ ] 6.2 Ask: publish a `graph-clustering` `GRAPH_STATUS` readiness envelope, so the deferred
      `community_context` can use the same readiness contract as every other producer instead of
      probing `COMMUNITY_INDEX`. Not blocking for this change.

## 7. Gates

- [ ] 7.1 `gofmt`, `go vet`, `revive` (warnings fail, pinned v1.15.0), `go test ./...`, and
      `go test -tags=integration ./...` green.
- [ ] 7.2 `openspec validate expose-graphrag-on-mcp --strict` green.
- [ ] 7.3 Boot a real stack and call both tools over MCP before claiming done — a compile-clean
      build proves nothing about wiring.

## 8. Follow-up (not this change)

- [ ] 8.1 Open the follow-up change for `community_context` / `graph.query.localSearch`: the
      capability gate, three-state readiness, `source_status` extension, and live tier-2 acceptance
      on `tier2-compose-dev.json`. Start it after ask 6.2 lands, so readiness has a substrate source
      rather than a bucket probe.
