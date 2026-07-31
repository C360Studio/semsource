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

- New MCP tools on the gateway exposing the GraphRAG query surface, routed to the **existing**
  semstreams subjects (`graph.query.localSearch`, `graph.query.globalSearch`,
  `graph.query.summary`, `graph.query.searchGraph`). SemSource adds gateway routes and tool
  contracts only; the substrate APIs are semstreams' and are not reimplemented or modified. Tool
  count, naming, and argument shapes (one tool with a mode argument vs. separate tools) are a
  design decision, made against what the subjects actually accept at the pinned semstreams version.
- **Honest capability gating.** GraphRAG requires tier 2 (`enable_clustering`, and
  `clustering_llm` for LLM community summaries). On a tier-0/1 stack the new tools MUST say so —
  a truthful "capability not enabled at this tier" `isError`, never an empty result
  indistinguishable from a miss. Readiness must gate on the community index actually being
  populated, in the same spirit as the existing `phase`/`index.ready`/`embedding.ready` envelope.
- **ADR revising the ADR-0004 MCP boundary.** ADR-0004's MCP note scoped the surface to "a thin
  wrapper over the same fusion contract." Adding non-fusion tools is a genuine, deliberate
  revision of that stance and gets a one-page decision record (fusion tools stay deterministic
  and fusion-backed; GraphRAG tools are a second, capability-gated family — not fusion, and not a
  reason to blur the two).
- **Live tier-2 acceptance, deterministic only.** A runtime acceptance check that boots a tier-2
  stack and verifies the tools' contracts: honest gating at tier 1, non-empty community results at
  tier 2 on a known corpus, envelope/shape invariants. Answer-quality grading of summary prose is
  explicitly out (see Non-goals) — a drifting judge cannot join a deterministic set.

## Non-goals

- Implementing or modifying GraphRAG itself — clustering, community summaries, and the
  `graph.query.*` handlers are substrate (semstreams). Any gap found in the subjects' contracts is
  a semstreams ask (`docs/upstream/semstreams-asks.md` + GitHub issue), never a reimplementation.
- LLM-judged answer-quality grading, in the scorecard or beside it. The scorecard's own rationale
  stands: a drifting judge cannot support an A/B. A GraphRAG quality instrument is a separate
  change with its own design.
- Changing the GraphQL surface — it already exposes GraphRAG.
- Making tier 2 the default, packaging seminstruct, or changing tier semantics.
- New scorecard questions for GraphRAG (they would require the judge this change declines to add).

## Consumers

External MCP agents (Claude Code, Cursor, Codex) gain the tools directly. SemSpec's agentic loop —
whose grep-fallback post-mortem motivated ADR-0004 — is the primary internal consumer; SemDragon
and SemOps consume the same gateway surface unchanged.

## Capabilities

### New Capabilities

- `graphrag-access`: what the GraphRAG MCP tools guarantee — routing to the substrate's community
  query surface, truthful tier/capability gating, readiness semantics, and result envelope
  invariants.

### Modified Capabilities

- `mcp-gateway-contract`: the tool roster grows beyond fusion-backed tools; honesty requirements
  (isError mapping, truthful signal statements) must extend to the non-fusion family, including
  the "capability not enabled" state that fusion tools never had.
- `advertised-surface-coverage`: the advertised tool/route surface must include the new tools and
  their tier-conditional availability, so a consumer can discover whether GraphRAG is reachable
  before calling it.

## Impact

- `processor/mcp-gateway/` — tool registration, argument schemas, routing to the four
  `graph.query.*` subjects, capability gating.
- `processor/source-manifest/` — readiness/capability advertisement if the community-index signal
  joins the status payload (`workbench_capabilities.go`, `readiness.go`).
- `config/config.go` already models `enable_clustering`/`clustering_llm`; the gating logic reads
  it — no config shape change expected.
- `docs/adr/` — new ADR revising the ADR-0004 MCP boundary; pointer update in ADR-0004.
- `configs/tiers/README.md` and `scripts/scorecard/README.md` — both currently state the MCP
  tools never reach GraphRAG; both must be updated to describe the new family and why the
  scorecard still does not grade it.
- Acceptance harness (location per design: `test/e2e/` or `scripts/`) booting
  `tier2-semantic-instruct.json` / the tier-2 compose overlay.
- Dependency: the pinned semstreams version's `graph.query.localSearch`/`globalSearch`/`summary`/
  `searchGraph` contracts — the design phase must verify their request/response shapes against
  beta.159 before fixing tool schemas.
