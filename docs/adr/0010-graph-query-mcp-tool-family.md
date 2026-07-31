# ADR-0010: Graph-Query Tools Are a Second MCP Family

> **Status:** Accepted — owner ruling 2026-07-31 that GraphRAG being unreachable over MCP is a gap,
> not intended design. | **Date:** 2026-07-31
> **Owners:** SemSource owns the gateway routes, tool contracts, and the retrieval disclosure.
> SemStreams owns clustering, community summaries, and the `graph.query.*` handlers.
> **Supersedes:** the MCP scoping note in [ADR-0004](0004-deterministic-fusion-gateway.md).

## Context

ADR-0004 scoped the MCP surface to "a thin wrapper over the same fusion contract". Under that
scoping, the gateway's eight tools all resolved through the fusion gateway, and the substrate's
graph-query surface — `graph.query.summary`, `globalSearch`, `searchGraph`, `localSearch` — was
reachable only over NATS and GraphQL.

The consequence was not a missing convenience. An agent connected over MCP, which is the door
external clients actually use, could not reach community/thematic reasoning at all. It also distorted
measurement: the graded scorecard drives the MCP surface, so it had never measured that surface — not
because the instrument skipped it, but because there was no door.

Verification against semstreams `v1.0.0-beta.159` established that these subjects do not share one
dependency, which is what makes a second family workable without a capability gate:

- `graph.query.summary` is a discovery resolver over prefix + predicate-list queries. It never
  touches clustering.
- `graph.query.searchGraph` delegates to `globalSearch`, whose `graphrag` path is one of five
  strategies, and adds a labelled semantic fallback when the result is empty. It answers at every
  tier.
- `graph.query.localSearch` is the only hard-gated subject: its handler subscribes solely when the
  community cache exists, so below a clustered deployment it has no responder at all.

## Decision

**The MCP surface has two families, and they stay distinguishable.**

1. **Fusion tools** (`code_context`, `code_impact`, `code_search`, `doc_context`, `code_changes`)
   remain exactly as ADR-0004 scoped them: deterministic, fusion-backed, carrying
   `contract_version`, returning ranked citable evidence for the agent to reason over.

2. **Graph-query tools** (`graph_summary`, `graph_search`) route to the substrate's query surface.
   They carry the substrate's payload verbatim and no `contract_version` — that field belongs to the
   fusion contract, and its absence on a graph-query result is not a failure.

**Neither graph-query tool is gated.** Gating a tool on a capability its routed subject does not
require would withdraw a working capability. What varies across deployments is how far an answer
escalates, and that is disclosed rather than hidden:

| Deployment | Answer contains | Disclosed rung |
| ---------- | --------------- | -------------- |
| No clustering | Entity hits | `entities_only` |
| Clustering, no LLM | \+ community summaries \+ template answer | `community_summaries` |
| Clustering \+ LLM | \+ LLM-synthesized answer | `llm_answer` |

**The disclosure is the price of not gating.** Without it, `graph_search` on a non-clustered stack
would present semantic-similarity hits as thematic community reasoning — an honesty hole worse than
the gap this change closes. The disclosure is derived only from fields the substrate returns and sits
*alongside* a verbatim payload, never merged into it, so a substrate-reported value can always be
preferred over ours.

**`answer_model`, not `degraded`, discriminates an LLM answer from the template floor.** The
substrate sets `degraded` only when an LLM-*configured* deployment falls back; a deployment with no
LLM returns a template answer with `degraded: false` deliberately, because the template is the
canonical answer there. Any derivation that reads `degraded` as the LLM flag reports every template
answer on a no-LLM stack as LLM-synthesized.

**No tool requires an LLM to answer.** Query classification is a tiered chain whose first tier is a
deterministic keyword classifier, tried first and bypassing the LLM tier on a match. Tool descriptions
therefore do not promise natural-language query understanding.

## Consequences

- The scorecard's scope is now an explicit choice rather than an accident of reachability: it grades
  the fusion family, and the graph-query family is excluded because grading synthesized prose needs a
  judge, and a drifting judge cannot support an A/B. A GraphRAG quality instrument is separate.
- SemSource takes a dependency on substrate response *fields* (`community_summaries`, `answer_model`,
  `degraded`) for the disclosure. This is reading substrate state, not reimplementing substrate
  behavior — SemSource computes no membership, summary, or ranking.
- Two upstream asks follow, neither blocking: populate `GlobalSearchResponse.Strategy` on all paths
  (it is computed and metered but never reaches the wire), and publish a `graph-clustering`
  `GRAPH_STATUS` readiness envelope.
- `graph.query.localSearch` stays unexposed pending a follow-up change. It needs a genuine capability
  gate and a three-state readiness signal, and the readiness ask above is what gives that signal a
  substrate source instead of a bucket probe.

## References

- ADR-0004 — deterministic fusion gateway; this ADR revises its MCP scoping note.
- ADR-0002 — tier support and model registry; the capability ladder is its tiers seen from the agent
  side.
- `openspec/specs/graphrag-access/spec.md` — the behavior contract these decisions imply.
