## Context

See `proposal.md` — Why. This document records what verification against the pinned semstreams
`v1.0.0-beta.159` found, because it contradicts an assumption the proposal was built on.

The proposal treats `graph.query.localSearch`, `globalSearch`, `summary`, and `searchGraph` as one
family behind one tier-2 gate. **They are not.** Read at the pinned version:

| Subject | Community-dependent? | Behavior with clustering off |
| --- | --- | --- |
| `graph.query.localSearch` | **Hard** — the handler subscribes only once the community cache exists (`processor/graph-query/graphrag.go:219`, `setupGraphRAGHandlers`) | **No responder.** The request fails at the transport, which reads to an agent as an infrastructure hiccup, not a missing capability |
| `graph.query.globalSearch` | **Partial** — `graphrag` is one of five strategies (`resolveStrategy`, `graphrag.go:556`); even that strategy searches semantically first and community lookup "may be empty without cache" | **Succeeds**, answered by another strategy, with `community_summaries` empty |
| `graph.query.searchGraph` | **Partial** — wraps `globalSearch`, then falls back to semantic search when the result is empty (`searchgraph.go:53`) | **Succeeds** |
| `graph.query.summary` | **None** — a composite discovery resolver fanning out to `graph.ingest.query.prefix` + `graph.index.query.predicateList` (`summary.go:58`); no clustering involvement whatsoever | **Succeeds**, fully functional |

Two consequences. First, `summary` is not GraphRAG; putting it behind a GraphRAG capability gate
would withdraw a working capability for no reason. Second — and this is the sharper problem — for
`globalSearch` and `searchGraph` the honesty hole is the *inverse* of the one the proposal
describes: they do not go silent below tier 2, they **succeed by another route**, and the caller
cannot tell whether it received community-backed thematic reasoning or a semantic-similarity list.

Compounding it: `GlobalSearchResponse.Strategy` exists on the wire, and the handler computes the
strategy and records it as a Prometheus counter — but the field is populated in exactly one place
in non-test code, the `searchGraph` semantic fallback (`searchgraph.go:219`). All twelve
`GlobalSearchResponse{}` constructions in `graphrag.go` leave it empty. The signal an agent needs
is computed and then dropped before it reaches the wire.

Two further corrections to the proposal's Impact section:

- `graph-clustering` publishes **no** `GRAPH_STATUS` readiness envelope. Only `graph-index`,
  `graph-embedding`, and `graph-ingest` keys exist (`internal/graphstatus/reader.go`), so the
  readiness signal the specs require has no substrate source today.
- `configs/tiers/tier2-semantic-instruct.json` ships with `enable_clustering: false` (commit
  `01c94d6`). The only config that turns clustering and LLM summaries on is
  `tier2-compose-dev.json`. Acceptance must target that one.

### The capability ladder

Verification also settles what "the benefit" actually is at each tier. `SynthesisOutcome`
(`answer.go:30`) escalates in three rungs:

| Stack | A thematic search answer contains |
| --- | --- |
| No clustering | Entity hits / digests. No community summaries, and **no synthesized answer** — synthesis requires summaries |
| Clustering, no LLM | \+ community summaries \+ a **template** answer |
| Clustering \+ LLM | \+ an **LLM-synthesized** answer |

With one trap that shapes the design: **`degraded` is not the "was this an LLM answer" flag.** A
deployment with no LLM configured returns a template answer with `degraded: false` deliberately —
"the template IS the canonical answer for that operator's deployment" (`answer.go:44-49`). `degraded`
means only that an LLM-*configured* deployment fell back. The sole discriminator for LLM synthesis
is a non-empty `answer_model`.

### Answering never requires an LLM

A natural-language query does not need LLM classification to be answered. `ClassifierChain`
(`graph/query/classifier_chain.go:44`) is tiered with a deterministic floor: **T0 KeywordClassifier**
is always present and tried first, and a keyword match returns immediately, bypassing the LLM tier
entirely; T1/T2 embedding runs only if no keyword matched; the LLM tier runs only if nothing above
did. The LLM tier is enabled solely by a `model_registry` entry with the `query_classification`
capability — absent it the substrate logs "keyword-only is fine" and proceeds. Even a fully nil
chain is supported: `classifyQuery` returns nil, `resolveStrategy(nil)` yields the default `graphrag`
strategy, and `extractSearchRefinements(nil, …)` passes the raw query through unrefined.

So there are **three independent LLM knobs, each with its own deterministic floor** — and they are
not the same switch:

| Knob | Enabled by | Floor without it |
| --- | --- | --- |
| Query classification | `model_registry` `query_classification` capability | KeywordClassifier (T0) |
| Answer synthesis | `clustering_llm` \+ `community_summary` capability | `TemplateAnswerSynthesizer` |
| Community summaries | `clustering_llm` | statistical summary |

SemSource exposes no configuration for `query_classification` today, so that tier is off in every
current deployment — and the substrate carries hardening showing a small classifier model can make
results *worse* (template placeholders emitted verbatim; single-token proper nouns replaced with
generic examples — `graphrag.go:423-445`). Consequence for this design: no new dependency and no
gating, but tool descriptions must not promise natural-language query understanding, since it is
registry-dependent and not observable from the response.

## Goals / Non-Goals

**Goals:**

- Give an agent the graph-query surface **on every tier**, with the answer escalating as the stack
  provides more, rather than the tool disappearing when it provides less.
- Require no LLM for any tool to answer.
- Make the reached rung legible from the tool result alone.
- Keep the deterministic fusion family and the new family distinguishable in the roster.

**Non-Goals** (beyond `proposal.md` — Non-goals):

- **`community_context` / `graph.query.localSearch` — deferred to a follow-up change.** It is the
  only hard-gated subject, and it carries essentially all of this change's cost: config plumbing, a
  `COMMUNITY_INDEX` probe, a three-state readiness signal, a `source_status` extension, and a live
  tier-2 harness with a seminstruct model dependency. It is also the least reachable capability —
  of four shipped tier configs, only `tier2-compose-dev.json` enables clustering. Deferring it
  leaves the no-responder trap unfixed but *unreachable*, which is the status quo, not a regression.
  Upstream ask 2 below is what gives the follow-up a real readiness source instead of a bucket probe.
- Normalizing the substrate response shapes into one envelope. They differ because they answer
  different questions; flattening them would hide the differences this change exists to expose.
- Caching, pre-warming, or otherwise influencing when clustering runs.
- A GraphQL-side change; the substrate signal gaps identified here are filed upstream, not patched
  behind SemSource's own surfaces.

## Decisions

### D1 — One ungated tool

The proposal left tool count and arity open. Decision: **one new tool**, `graph_search`, routed to
`graph.query.searchGraph`, with its own typed argument struct and no capability gate.

A second tool, `graph_summary` over `graph.query.summary`, was designed, implemented, and then
**withdrawn during runtime acceptance** — see D6. `graph.query.summary` is contested, and a tool on
a contested subject returns a nondeterministic payload.

*Why not one tool with a `mode` argument.* Moot at one tool, and it was already the wrong shape:
conditionally-required fields are the schema models most reliably get wrong.

*Why `globalSearch` gets no tool of its own.* `searchGraph` calls `handleGlobalSearch` first and
returns its response **unchanged** when non-empty; it only adds a labelled fallback on empty. A
separate `global_search` tool would offer the agent a strictly weaker sibling, distinguishable only
by expertise the agent does not have. `globalSearch` remains reachable — every non-empty answer from
`graph_search` *is* a `globalSearch` answer.

*Why it is not gated.* `graph.query.searchGraph` does not require clustering. Gating it would be a
fabricated restriction that withholds the tool exactly when the agent has least other recourse. The
one subject that genuinely needs a gate — `localSearch` — is deferred (see Non-Goals).

### D2 — Disclosure is derived from response fields, and the gaps are filed upstream

`GlobalSearchResponse.Strategy` would say which path answered, but it is unpopulated on every normal
path (see Context), so SemSource cannot report the strategy. What it *can* read from the substrate's
own response, without recomputing anything:

| Signal read | What it establishes |
| --- | --- |
| `community_summaries` / `community_id` non-empty | The answer is community-backed |
| `answer` non-empty | A synthesized answer is present |
| `answer_model` non-empty | That answer was LLM-synthesized — **the only reliable discriminator** |
| `degraded` \+ `degraded_reason` | An LLM-*configured* deployment fell back, including `global_search_empty_semantic_fallback` |

`degraded` deliberately does **not** discriminate LLM from template (see the capability ladder), so
the disclosure derives the rung from `community_summaries` and `answer_model`, and carries `degraded`
through as the separate signal it actually is.

A `graph_search` result therefore carries a small SemSource-added disclosure block **alongside** the
substrate payload, which passes through verbatim. The disclosure never rewrites the payload, so a
future substrate `strategy` value can be preferred over the SemSource-derived rung without conflict.

Two upstream asks (`docs/upstream/semstreams-asks.md` + semstreams issues), neither blocking:

1. Populate `GlobalSearchResponse.Strategy` on all `globalSearch` paths — the value is already
   computed and metered to Prometheus, it just never reaches the wire.
2. Publish a `graph-clustering` `GRAPH_STATUS` readiness envelope. Not needed by this change, but it
   is what lets the deferred `community_context` use the same readiness contract as every other
   producer instead of probing a bucket name.

### D3 — The new family is marked in the roster, not blended into it

The existing tools' honesty story rests on `contract_version` and the fusion envelope; these tools
have neither. Rather than manufacture a fake `contract_version`, each new tool's description opens by
naming what it is — a substrate graph query, not a fusion answer — and states that its result
discloses the rung it reached. No `source_status` change is needed in this scope: nothing here is
gated, so there is no availability signal to publish.

Descriptions must also draw a hard line against `code_search` and must not promise natural-language
query understanding, which is registry-dependent (see the classification note above).

### D4 — Acceptance is hermetic; no live tier-2 stack required

- **Unit / integration** (`processor/mcp-gateway/`, always in CI): disclosure derivation across all
  three rungs from recorded substrate responses — including the template-vs-LLM discrimination that
  `degraded` alone would get wrong — plus roster shape, argument validation, and error mapping.
- **Existing tier-0 compose smoke**, extended with one real `tools/call` per new tool asserting a
  non-empty answer and the not-community-backed disclosure. This is the default stack, so it needs no
  new infrastructure, no clustering, and no model registry.

The community-backed and LLM rungs are proven from recorded responses rather than a live clustered
stack: the logic under test is SemSource's derivation, not the substrate's clustering, and a
seminstruct model dependency in the PR gate buys flakiness rather than confidence. When
`community_context` ships, it brings the live tier-2 harness with it.

### D5 — ADR revising the ADR-0004 boundary

One page: fusion tools remain deterministic and fusion-backed; graph-query tools are a second family
that answers at every tier and discloses the rung it reached. The ADR records the boundary decision;
the mechanics stay in the specs. ADR-0004 gets a pointer, not a rewrite.

### D6 — `graph.query.summary` is contested, so no tool routes to it

Runtime acceptance on the real stack returned SemSource's **source-manifest** payload from what was
supposed to be the substrate's graph summary. The cause is a subject collision inside a single
SemSource process:

| Subject | SemSource handler | SemStreams handler |
| --- | --- | --- |
| `graph.query.summary` | `source-manifest` (`processor/source-manifest/component.go:264`) | `graph-query` (`processor/graph-query/query.go:45`) |

Both subscribe with plain request/reply and no queue group, so a requester receives whichever handler
replies first and discards the other. The two payloads are entirely different shapes. This is a live
defect independent of this change: any consumer calling `graph.query.summary` on a SemSource
deployment today gets a race.

It is SemSource's defect, not the substrate's. `graph.query.*` is a substrate-owned namespace, and
source-manifest claims four subjects inside it (`sources`, `predicates`, `status`, `summary`). Three
name SemSource concepts the substrate does not define; `summary` collides.

**Decision: withdraw `graph_summary` from this change.** Shipping a tool whose payload is decided by
a race would be worse than not shipping it. Resolving the collision means moving source-manifest to
a SemSource-owned subject, which changes a documented consumer contract and therefore needs its own
change with a deprecation path — not a silent fix folded into this one.

The capability is not lost: the same content is served at `GET /source-manifest/summary`. What is
lost is MCP reachability for it, which the follow-up restores once the subject is unambiguous.

## Risks / Trade-offs

- **`graph_search` and `code_search` are confusable** → Descriptions draw the line in the first
  clause: `code_search` finds code symbols by meaning; `graph_search` answers corpus-wide thematic
  questions across all entity types. The compose smoke asserts a query only one of them should win,
  so a description regression is visible.
- **Shipping `graph_search` on a non-clustered stack could read as "GraphRAG on MCP" when the answer
  is a semantic hit list** → This is exactly what the disclosure requirement exists to prevent, and
  it is the one thing in this scope that must not be cut: without it, phase one would create an
  honesty hole rather than close one.
- **Derived disclosure could disagree with a future substrate `strategy` field** → The disclosure is
  additive and namespaced; the substrate payload passes through verbatim, so both can be present and
  a consumer can prefer the substrate's own value.
- **Deferring `community_context` leaves the no-responder trap unfixed** → It also leaves it
  unreachable: no MCP agent can hit `localSearch` today, so this is the status quo rather than a
  regression. The follow-up change owns it.
- **Other SemSource subjects sit inside the substrate's `graph.query.*` namespace** → `sources`,
  `predicates`, and `status` do not collide today because the substrate defines no such subjects, but
  nothing prevents a future semstreams release from adding one. The `summary` collision is the
  warning; a subject-ownership audit belongs with the fix.
- **Proving the community and LLM rungs from recorded responses, not a live clustered stack** → The
  logic under test is SemSource's derivation, which recorded responses exercise fully. What they do
  not prove is that the substrate still emits those fields; that is pinned by the semstreams version
  bump process, and the follow-up change's live harness covers it.

## Migration Plan

Purely additive: no existing tool's name, arguments, or response changes, so current MCP consumers
are unaffected and no version bump is forced on them. Rollback is removing the two registrations.

Sequencing: register the two tools with verbatim passthrough → add the disclosure derivation →
unit/integration tests → extend the tier-0 compose smoke → ADR → docs
(`configs/tiers/README.md`, `scripts/scorecard/README.md`, README surface matrix) → file the two
upstream asks. No config plumbing, no readiness work, and no new stack are on this path.

## Open Questions

- Whether `graph_search` should expose `searchGraph`'s optional shaping flags
  (`max_communities`, `include_relationships`, `include_sources`, `summarize_threshold`) or start
  with query-only and add them on demand. Defaults are server-side and sane, so starting query-only
  changes no spec requirement and no task boundary — it is a description-and-schema detail settled
  during implementation.
