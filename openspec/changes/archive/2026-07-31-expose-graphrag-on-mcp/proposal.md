## Why

An agent connected over MCP — the door external clients actually use — cannot reach GraphRAG at
all. `localSearch`, `globalSearch`, `summary`, and `searchGraph` exist only as `graph.query.*`
NATS subjects and GraphQL fields; none of the MCP gateway's eight tools routes to them.
`configs/tiers/README.md` and ADR-0004 currently present this as design ("the MCP tools resolve
through the fusion gateway and never reach GraphRAG"), but the owner ruling (2026-07-31) is that it
is a **gap**: community/summary reasoning is a large part of what semstreams and SemSource offer,
and agents should be able to reach it.

The gap also distorts measurement. The graded scorecard drives the MCP surface, so it has never
measured GraphRAG — not because the instrument skips it, but because there is no door. Measuring
only the deterministic surface quietly redefines "quality" as retrieval precision.

## What Changes

Verification against the pinned semstreams version corrected the premise this change started from:
the four `graph.query.*` subjects are not one family behind one tier-2 gate (see `design.md` —
Context). `summary` never touches clustering; `globalSearch`/`searchGraph` answer at every tier
through non-community paths; only `localSearch` is hard-gated. Scope is narrowed accordingly.

- **One new ungated MCP tool**, `graph_search`, routed to the **existing** semstreams subject
  `graph.query.searchGraph` (which delegates to `graph.query.globalSearch` and returns it unchanged
  when non-empty). SemSource adds a gateway route and tool contract only; the substrate APIs are
  semstreams' and are not reimplemented or modified. A second tool over `graph.query.summary` was
  withdrawn during runtime acceptance: that subject is answered by both SemSource's source-manifest
  and the substrate's graph-query, so a tool on it returns a nondeterministic payload (design.md — D6).
- **Answers escalate with the stack; the tool never disappears.** A result carries entity hits at
  every tier, plus community summaries where clustering is enabled, plus an LLM-synthesized answer
  where an LLM is configured. No tool is gated on clustering, and no tool requires an LLM to answer —
  query classification has a deterministic keyword floor.
- **Honest disclosure of the rung reached.** Because the substrate does not put its strategy on the
  wire, each result carries a SemSource-derived disclosure — community-backed or not, and
  LLM-synthesized or template — sitting alongside a verbatim substrate payload. The discriminator is
  `answer_model`, not the substrate's `degraded` flag, which is false by design on a template-only
  deployment.
- **ADR revising the ADR-0004 MCP boundary.** ADR-0004 scoped the MCP surface to "a thin wrapper over
  the same fusion contract". Adding non-fusion tools is a deliberate revision and gets a one-page
  decision record.
- **Deterministic acceptance on the default stack.** Hermetic tests cover disclosure derivation
  across all three rungs from recorded substrate responses; the existing tier-0 compose smoke gains a
  real `tools/call` per new tool. No clustered stack, model registry, or new infrastructure is
  required.
- **Two upstream asks filed**, neither blocking: populate `GlobalSearchResponse.Strategy` on all
  paths, and publish a `graph-clustering` `GRAPH_STATUS` readiness envelope.

## Non-goals

- **`community_context` / `graph.query.localSearch` — a follow-up change.** It is the only
  hard-gated subject and carries nearly all of this change's original cost (config plumbing, a
  `COMMUNITY_INDEX` probe, three-state readiness, a `source_status` extension, a live tier-2 harness
  with a model dependency), while being the least reachable capability: of four shipped tier configs
  only `tier2-compose-dev.json` enables clustering. Deferring it leaves the no-responder trap unfixed
  but unreachable — the status quo, not a regression.
- Implementing or modifying GraphRAG itself — clustering, community summaries, and the
  `graph.query.*` handlers are substrate (semstreams). Any gap found in the subjects' contracts is a
  semstreams ask (`docs/upstream/semstreams-asks.md` + GitHub issue), never a reimplementation.
- LLM-judged answer-quality grading, in the scorecard or beside it. The scorecard's own rationale
  stands: a drifting judge cannot support an A/B. A quality instrument is a separate change.
- Changing the GraphQL surface — it already exposes this surface.
- Making clustering the default, packaging seminstruct, or changing tier semantics.
- New scorecard questions (they would require the judge this change declines to add).

## Consumers

External MCP agents (Claude Code, Cursor, Codex) gain the tools directly. SemSpec's agentic loop —
whose grep-fallback post-mortem motivated ADR-0004 — is the primary internal consumer; SemDragon
and SemOps consume the same gateway surface unchanged.

## Capabilities

### New Capabilities

- `graphrag-access`: what the graph-query MCP tools guarantee — routing to the substrate's query
  surface, availability at every tier, honest disclosure of the capability rung an answer reached,
  and citable result attribution.

### Modified Capabilities

- `mcp-gateway-contract`: the tool roster grows beyond fusion-backed tools; the honesty requirements
  (isError mapping, truthful signal statements) extend to the non-fusion family, including a
  graph-query success that legitimately carries no `contract_version`.
- `advertised-surface-coverage`: the new tools need named test evidence proving they answer on the
  default stack and disclose the rung reached, with the escalated rungs proven from recorded
  substrate responses.

## Impact

- `processor/mcp-gateway/` — one tool registration, argument schema, routing to
  `graph.query.searchGraph`, and the disclosure derivation.
- `docs/adr/` — new ADR revising the ADR-0004 MCP boundary; pointer update in ADR-0004.
- `configs/tiers/README.md` and `scripts/scorecard/README.md` — both currently state the MCP tools
  never reach GraphRAG; both must describe the new family and the capability ladder, and say why the
  scorecard still does not grade it.
- The existing tier-0 compose smoke — a real `tools/call` asserting the answer and its disclosure.
- `docs/upstream/semstreams-asks.md` — two asks filed.
- No change to `config/config.go`, `processor/source-manifest/`, or the compose profiles: nothing in
  this scope is gated, so there is no availability signal to plumb or publish.
- Dependency: the pinned semstreams version's `graph.query.summary` / `searchGraph` / `globalSearch`
  contracts, verified against `v1.0.0-beta.159` in `design.md` — Context.
