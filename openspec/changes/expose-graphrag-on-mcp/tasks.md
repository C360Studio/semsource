# Tasks

## 1. Route the tool

- [x] 1.1 Add the `GraphSearchInput` typed argument struct in
      `processor/mcp-gateway/query_tools.go`, query-only (design — Open Questions), validating a
      non-empty query the way `fusionQuery` already does. **`GraphSummaryInput` was added and then
      removed** — see 1.5.
- [x] 1.2 Add the `graphSearch` handler routing through the existing `c.request`
      (`RequestClassified`, so ADR-060 errors keep surfacing as `isError`) to
      `graph.query.searchGraph`.
- [x] 1.3 Register it in `buildServer()` (`component.go`). The description names the tool as a
      substrate graph query rather than a fusion answer, draws the line against `code_search`, states
      that the result discloses the rung it reached, and promises no natural-language query
      understanding. Pinned by `TestGraphToolDescriptionsStayHonest`.
- [x] 1.4 No port declaration needed: the gateway declares no request subjects at all
      (`InputPorts`/`OutputPorts` are empty) and already requests `graph.query.status`,
      `graph.query.versionDiff`, and the fusion verbs undeclared. Adding one subject here would make
      the metadata *less* consistent, not more.
- [x] 1.5 **Withdrew `graph_summary`.** Runtime acceptance returned SemSource's source-manifest
      payload: `graph.query.summary` is answered by BOTH `source-manifest` (`component.go:264`) and
      the substrate's `graph-query` (`query.go:45`) in the same process, so the reply is a race
      between two payload shapes. A tool on a contested subject is nondeterministic. See design D6
      and task 8.2.

## 2. Derive the disclosure

- [x] 2.1 Add a disclosure deriver reading only fields the substrate returns:
      `community_summaries`/`community_id` → community-backed; `answer` → an answer is present;
      `answer_model` → LLM-synthesized; `degraded`+`degraded_reason` → carried through verbatim.
- [x] 2.2 **`answer_model` is the only LLM discriminator.** `degraded` is false by design on a
      template-only deployment (`answer.go:44-49`); it MUST NOT be used to infer template vs LLM.
- [x] 2.3 Attach the disclosure **alongside** the substrate payload, which passes through verbatim —
      never merged into or rewriting it, so a future substrate `strategy` can be preferred over the
      derived rung.
- [x] 2.4 ~~Leave `graph_summary` results as pure passthrough~~ — withdrawn with the tool (1.5).

## 3. Prove it hermetically

- [x] 3.1 Table test over the three rungs from recorded substrate responses: no clustering
      (entity hits only), clustering without LLM (community summaries + template answer,
      `degraded:false`, empty `answer_model`), clustering with LLM (`answer_model` set).
- [x] 3.2 Regression test pinning task 2.2: a response with `degraded:false` and empty
      `answer_model` MUST NOT be reported as LLM-synthesized. This is the trap the whole disclosure
      exists to avoid — it must fail loudly if the derivation is ever simplified.
- [x] 3.3 Test that the substrate payload survives verbatim through the gateway.
- [x] 3.4 Test argument validation and that a downstream ADR-060 error still maps to `isError`, and
      that a graph-query success is NOT rejected for lacking `contract_version`.
- [x] 3.5 Extend the default-component/roster pinning test so dropping either registration fails CI.

## 4. Prove it on the default stack

- [x] 4.1 Extend the existing tier-0 compose smoke with a real `tools/call`, asserting a non-empty
      answer — name-listing does not count as coverage. **This is what caught 1.5.**
- [x] 4.2 Assert the not-community-backed disclosure is present on that tier-0 answer, that the tool
      does not refuse, and that it does not claim an LLM answer on a stack with no LLM.
- [x] 4.3 **Not added deliberately.** The smoke fixture is six entities (one Go file, one README, one
      YAML), which cannot separate "thematic across all types" from "semantic over code symbols" —
      any pair of queries would pass or fail on corpus accident, and a flaky gate is worse than none.
      The confusability risk is instead pinned statically by `TestGraphToolDescriptionsStayHonest`,
      which fails if `graph_search`'s description stops distinguishing itself from `code_search`. A
      real discrimination query belongs with the scorecard corpus, not this fixture.

## 5. Correct the record

- [x] 5.1 New ADR revising the ADR-0004 MCP boundary: fusion tools stay deterministic and
      fusion-backed; graph-query tools are a second family, ungated, disclosing their rung. Add a
      pointer in ADR-0004 — do not rewrite it.
- [x] 5.2 `configs/tiers/README.md` — replace the claim that MCP tools never reach GraphRAG with the
      capability ladder, and state that clustering is off in every shipped tier config except
      `tier2-compose-dev.json`.
- [x] 5.3 `scripts/scorecard/README.md` — describe the new family and state why the graded scorecard
      still does not measure it (a drifting judge cannot support an A/B).
- [x] 5.4 README advertised-surface matrix — add the tool with its named test evidence, including
      the mutation-verified disclosure-honesty row.

## 6. File the upstream asks

- [x] 6.1 Ask: populate `GlobalSearchResponse.Strategy` on all `globalSearch` paths. The strategy is
      already computed and metered to Prometheus (`graphrag.go:629-632`) but reaches the wire in
      exactly one place, the `searchGraph` semantic fallback (`searchgraph.go:219`); all 12 response
      constructions in `graphrag.go` leave it empty. Record in `docs/upstream/semstreams-asks.md`
      and open a semstreams issue.
- [x] 6.2 Ask: publish a `graph-clustering` `GRAPH_STATUS` readiness envelope, so the deferred
      `community_context` can use the same readiness contract as every other producer instead of
      probing `COMMUNITY_INDEX`. Not blocking for this change.

## 7. Gates

- [x] 7.1 `gofmt`, `go vet`, `revive` (warnings fail, pinned v1.15.0), `go test ./...`, and
      `go test -tags=integration ./...` green.
- [x] 7.2 `openspec validate expose-graphrag-on-mcp --strict` green.
- [x] 7.3 Boot a real stack and call the tool over MCP before claiming done — a compile-clean build
      proves nothing about wiring. `./scripts/core-profile-smoke.sh` exits 0 with `graph_search`
      answering and disclosing a non-community, non-LLM rung. It is also what surfaced 1.5.

## 8. Follow-up (not this change)

- [ ] 8.1 Open the follow-up change for `community_context` / `graph.query.localSearch`: the
      capability gate, three-state readiness, `source_status` extension, and live tier-2 acceptance
      on `tier2-compose-dev.json`. Start it after ask 6.2 lands, so readiness has a substrate source
      rather than a bucket probe.
- [ ] 8.2 Open a change resolving the `graph.query.summary` collision: move source-manifest to a
      SemSource-owned subject, with a deprecation path for consumers using the current one, and audit
      the other three subjects source-manifest claims inside the substrate's `graph.query.*`
      namespace (`sources`, `predicates`, `status`). Restore an MCP graph-overview tool once the
      subject is unambiguous.
