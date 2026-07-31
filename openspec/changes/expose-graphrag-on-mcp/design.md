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

## Goals / Non-Goals

**Goals:**

- Route MCP tools to the substrate's community query surface with per-subject gating that matches
  each subject's actual dependency, not a uniform tier check.
- Make "which retrieval path answered this" legible to the agent from the tool result alone.
- Keep the deterministic fusion family and the new family distinguishable in the roster.

**Non-Goals** (beyond `proposal.md` — Non-goals):

- Normalizing the four substrate response shapes into one envelope. They differ because they answer
  different questions; flattening them would hide the differences this change exists to expose.
- Caching, pre-warming, or otherwise influencing when clustering runs.
- A GraphQL-side change; the substrate signal gaps identified here are filed upstream, not patched
  behind SemSource's own surfaces.

## Decisions

### D1 — Three tools, gated per subject, not one mode-switched tool

The proposal left tool count and arity open. Decision: **three new tools**, each with its own typed
argument struct, following the existing `<domain>_<noun>` naming.

| Tool | Subject | Gate |
| --- | --- | --- |
| `graph_summary` | `graph.query.summary` | **None** — works at every tier |
| `graph_search` | `graph.query.searchGraph` | **None** — always answers; result discloses which path answered |
| `community_context` | `graph.query.localSearch` | **Capability-gated** — the only hard gate |

*Why not one tool with a `mode` argument.* The argument shapes are genuinely different —
`localSearch` requires an `entity_id` **and** a query, `summary` takes no query at all, `searchGraph`
takes a query plus optional shaping flags. A single tool would need conditionally-required fields,
which is the schema shape models most reliably get wrong, and it would force one gating story onto
three different dependencies.

*Why `globalSearch` gets no tool of its own.* `searchGraph` calls `handleGlobalSearch` first and
returns its response **unchanged** when non-empty; it only adds a labelled fallback on empty. A
separate `global_search` tool would offer the agent a strictly weaker sibling of `graph_search`
distinguishable only by expertise the agent does not have. `globalSearch` remains reachable — every
non-empty answer from `graph_search` *is* a `globalSearch` answer. This is a roster decision, not a
capability removal.

*Why `graph_summary` ships ungated.* It never touched clustering. Gating it would be a fabricated
restriction.

### D2 — Availability is a three-state signal from two sources

`community_context` needs to separate three states, and no single source gives all three:

| State | Source | Retryable |
| --- | --- | --- |
| Not enabled in this configuration | `config.Graph.EnableClustering`, passed into the gateway's component config from `cmd/semsource/run.go` (`mcpGatewayComponentConfig`, which already holds `*config.Config`) | No — reconfigure |
| Enabled, community index not yet populated | `COMMUNITY_INDEX` KV bucket: absent or empty | Yes — retry |
| Enabled and populated | Both above pass | — |

The no-responder transport error stays as a backstop, but it is no longer how the agent learns the
answer: the gate is checked before the request is issued, so the agent gets a stated capability
condition instead of a timeout.

*Alternative rejected — config flag alone.* Cheap and adds no substrate coupling, but it cannot
distinguish "still clustering" from "ready", so a mid-clustering empty answer would still read as a
genuine absence.

*Alternative rejected — probe only, no config flag.* An empty bucket cannot tell "clustering
disabled" from "clustering enabled and not finished", collapsing a reconfigure into a retry.

Reading `COMMUNITY_INDEX` is reading substrate state, not reimplementing substrate behavior; the
boundary rule prohibits computing membership or summaries locally, which this does not do. It is
still coupling to a bucket name, so it is paired with D3's upstream ask and retired once that lands.

### D3 — Disclosure is derived from response fields, and the gap is filed upstream

Because `strategy` is unpopulated on every normal path (see Context), SemSource cannot report which
strategy answered. What it *can* read from the substrate's own response, without recomputing
anything:

- `community_summaries` / `community_id` present → the answer is community-backed.
- `degraded` + `degraded_reason` → the substrate already labels its own degradation, including
  `global_search_empty_semantic_fallback`.
- `answer_model` → whether an LLM synthesized the prose, so a statistical summary is never
  presented as an LLM one.

`graph_search` and `community_context` results therefore carry a small SemSource-added disclosure
block alongside the substrate payload, stating community-backed yes/no and echoing the substrate's
degradation fields. The substrate payload itself passes through **verbatim** — the disclosure sits
beside it, never rewrites it, so a future substrate `strategy` value cannot be contradicted by a
SemSource-derived guess.

Two upstream asks (`docs/upstream/semstreams-asks.md` + semstreams issues), neither blocking:

1. Populate `GlobalSearchResponse.Strategy` on all `globalSearch` paths — the value is already
   computed and metered, it just never reaches the wire.
2. Publish a `graph-clustering` `GRAPH_STATUS` readiness envelope, so D2's bucket probe can be
   replaced by the same readiness contract every other producer uses.

### D4 — The new family is marked in the roster, not blended into it

The existing tools' honesty story rests on `contract_version` and the fusion envelope; these tools
have neither. Rather than manufacture a fake `contract_version`, each new tool's description opens
by naming what it is (substrate graph query, not fusion) and what it requires. `source_status`
grows a `graphrag` object using the same `available`/`ready`/`state`/`reason{retryable}` shape as
`index` and `embedding`, so the discovery path an agent already knows extends to this family
instead of being a second mechanism.

### D5 — Acceptance is layered; only the cheap layer gates CI

- **Unit / integration** (`processor/mcp-gateway/`, always in CI): gating decisions across the three
  states, the "no empty success" invariant, disclosure derivation from synthetic substrate
  responses, and the roster's shape. Fast, hermetic, and it covers every honesty requirement the
  specs state.
- **Live tier-2 acceptance** (script under `scripts/`, Task target, **not** default CI): boots
  `configs/tiers/tier2-compose-dev.json` — the only tier config with `enable_clustering: true` and
  `clustering_llm: true` — waits for a populated community index, and asserts non-empty
  community-backed results with attribution on a known corpus. Kept out of default CI because it
  needs a model registry with the `community_summary` capability (`config/config.go:282` →
  seminstruct); a heavyweight model dependency in the PR gate buys flakiness, not confidence.

Both are deterministic. No LLM judge in either, per `proposal.md` — Non-goals.

### D6 — ADR revising the ADR-0004 boundary

One page: fusion tools remain deterministic and fusion-backed; graph-query tools are a second,
separately-gated family that discloses its retrieval path. The ADR records the boundary decision;
the mechanics stay in the specs. ADR-0004 gets a pointer, not a rewrite.

## Risks / Trade-offs

- **`graph_search` and `code_search` are confusable** → Descriptions must draw the line at the first
  clause: `code_search` finds code symbols by meaning; `graph_search` answers corpus-wide thematic
  questions across all entity types. The tier-2 acceptance asserts a query that only one of them
  should win, so a description regression is visible.
- **Coupling to the `COMMUNITY_INDEX` bucket name** → Isolated behind one accessor with a single
  call site, and retired when upstream ask 2 lands.
- **Derived disclosure could disagree with a future substrate `strategy` field** → The disclosure is
  additive and namespaced; the substrate payload is verbatim, so both can be present and a consumer
  can prefer the substrate's own value.
- **Live acceptance outside default CI can rot** → The unit layer covers every spec requirement
  independently; the live layer proves the wiring. The Task target is named in the
  advertised-surface evidence matrix so it stays discoverable.
- **A three-state gate adds a pre-flight read to every `community_context` call** → One KV read
  against a local NATS, well inside the existing 30s tool timeout; cache with a short TTL if it ever
  shows up in latency.

## Migration Plan

Purely additive: no existing tool's name, arguments, or response changes, so current MCP consumers
are unaffected and no version bump is forced on them. Rollback is removing the three registrations.

Sequencing: pass the clustering flags into the gateway config → add the availability accessor →
register the three tools with gating and disclosure → extend `source_status` → unit/integration
tests → ADR → docs (`configs/tiers/README.md`, `scripts/scorecard/README.md`, README surface
matrix) → live tier-2 acceptance script → file the two upstream asks.

## Open Questions

- Whether `graph_search` should expose `searchGraph`'s optional shaping flags
  (`max_communities`, `include_relationships`, `include_sources`, `summarize_threshold`) or start
  with query-only and add them on demand. Defaults are server-side and sane, so starting query-only
  changes no spec requirement and no task boundary — it is a description-and-schema detail settled
  during implementation.
