# ADR-0004: Deterministic Fusion Gateway for Agent Context

> **Status:** Accepted → **Converged upstream** (2026-07-01) | **Date:** 2026-06-27
>
> **Update (beta.123):** The decision stands, but the *implementation* moved. The engine,
> Lens SPI, honesty envelope, and hydration contract were lifted into semstreams `pkg/fusion`
> ([semstreams#376](https://github.com/C360Studio/semstreams/issues/376) / ADR-062) and
> semsource converged onto it (PRs #15, #16) — the local `source/fusion` engine, the
> `natsgraph` client, and the `impact` extension were **deleted**. semsource now keeps only
> the `code`/`docs` lenses (`source/fusion/lens/*`) over `pkg/fusion` + `pkg/fusion/fusionnats`.
> Anything below describing the *local* engine is historical: `Lens.Hydrate` returns a
> `*message.StorageReference` handle (no worktree read), the fusion `Request` has no `repo`
> field, fusion readiness reads `graph.index.query.status` (not `graph.query.status`),
> deterministic symbol resolution uses `graph.query.byName`, and verbatim bodies are offloaded
> to an ObjectStore at ingest and dereferenced by the engine.

## Context

Downstream agents (SemSpec's agentic loop) bail to `grep` instead of using
our graph. The recorded post-mortem (`semspec/prompt/tool_filter.go:36-45`,
ADR-036) is concrete: across 4+ real-LLM `@hard` runs, agents called
`graph_*` tools ~zero times, and on the rare call got empty results / EOF
and retreated to bash + curl. Graph tools were removed from every planning
role.

The diagnosis that matters: **this is an exposure problem, not a missing-data
problem.** In SemSource the data and the embeddings already exist — `ast-source`
populates `code.relationship.calls`, code symbols carry `content` indexing
profile so `graph-embedding` embeds them, and `graph-query` already serves
`semantic`, `similar`, `relationships`, `prefix`, and `pathSearch`. What we
handed agents was *triples addressed by 6-part entity IDs*: to use it an agent
had to learn the ID scheme, compose GraphQL, traverse one hop per call, and
then still `Read` the file — because the response was metadata *about* code,
not code. `grep` wins because `grep` returns code.

### The mis-step this ADR corrects

The initial response (see the superseded plan) was to build a bespoke,
`codegraph`-shaped index in SemSource: a second tree-sitter parse, its own
`fsnotify` watcher, an in-memory symbol store, and a planned parallel
embedding tier. A phase-gate review caught that this **duplicates the SKG
framework** — parse (`ast-source`), watch (`ast-source`), store (graph KV),
call-graph (`graph-index` `OUTGOING`/`INCOMING`), embeddings
(`graph-embedding`), and status gating (`graph.query.status`) are all already
provided. Standing up a parallel source-of-truth that can drift from the real
graph is a known, costly trap. The only things the framework genuinely does
*not* provide are (1) a **fused response shape** and (2) **verbatim source
bytes** (the graph stores `code.artifact.path` + line ranges + signature/doc,
not the function body).

### The generalization that settles placement

`code_context` is, in shape, a **deterministic `research_graph`**:
`research_graph` (semstreams, ADR-045) runs resolve → route → execute(multi-hop)
→ assess → synthesize and terminates in *LLM-synthesized prose*; the thing we
want runs resolve → expand → hydrate → assemble and terminates in *verbatim
fused payload*. Same idea — move the join/hop/assemble off the agent and into a
layer over the SKG — minus the LLM.

The decisive test: **what happens when someone wants the same for docs?** A
doc agent asking "show me what we wrote about retry policy + the related
sections" is the twin of "show me the auth handler + its callers." Code and
docs differ in exactly one place — a **lens**:

| Pipeline stage (generic) | code lens | docs lens |
|---|---|---|
| **resolve** seeds | `graph.query.semantic` / symbol index | `graph.query.semantic` / title index |
| **expand** neighborhood | `calls`→callees, reverse→callers, `contains` | `links`→references, `contains`→sections |
| **hydrate** verbatim body | file `[start:end]` | passage / section text |
| **render** shape | symbols + source + call-paths + blast | passages + links + section tree |

If the only domain-specific part is a small lens, the fusion itself is a
**framework primitive**, and "code-flavored" is not grounds to make it
bespoke — it is just the code lens.

## Decision

**Build a deterministic fusion gateway over the existing graph query/search
infrastructure, parameterized by domain lenses, returning verbatim fused
payloads with entity IDs demoted to opaque handles. Do NOT build a parallel
index.**

### Shape

A fixed, deterministic pipeline that reuses existing `graph.query.*`:

1. **Resolve** the query (NL / symbol / path) → seed entity IDs via
   `graph.query.semantic` (the framework's embeddings — code stays `content`),
   symbol/alias index, or `graph.query.prefix`.
2. **Expand** the neighborhood along lens-specified relationship predicates
   (bounded hops) via `graph.query.relationships` (`OUTGOING`/`INCOMING`) and
   `pathSearch` for transitive impact.
3. **Hydrate** verbatim body per the lens (code: read file `[start:end]`;
   later, optionally offload bodies to the object store via
   `StorageReference`).
4. **Assemble**: rank, byte-budget, demote IDs to opaque handles, render per
   the lens; attach a readiness/provenance envelope. Ranking is **ontology-aware
   (BFO/CCO) per ADR-0005**, not the ad-hoc lexical/domain heuristic the spike
   used.

This is `research_graph`'s skeleton with the LLM `synthesize` stage replaced
by deterministic `assemble`, and ideally shares its resolve / multi-hop
machinery when upstreamed.

### Lens

The only domain-specific surface, declared per domain:

- `resolve` — which search modes / indexes.
- `edges` — relationship predicates and their roles (callers/callees vs
  links/references).
- `hydrate` — how to fetch the verbatim body.
- `render` — the response template.

Ship **two** lenses up front — **code** and **docs** — so the abstraction is
validated by the rule of two rather than asserted.

### Placement and sequencing (Option B)

The engine + lens interface belong in **semstreams**, beside `research_graph`.
But we prove it **in SemSource first**: implement it as a fusion gateway over
`graph.query.*` with the code and docs lenses, validate against SemSpec's
`mavlink-hard` loop, *then* propose lifting the engine into semstreams via a
follow-on ADR. The gateway-over-graph-query shape is correct regardless of
where the engine ultimately lives, and this moves now without cross-repo
coordination.

### Indexing profile

Code symbols **stay `content`** (embedded). Strong NL / similarity over code
*requires* the framework's embeddings, which the resolve stage consumes.
Earlier consideration of downgrading to `control`/`trace` was rejected: it
would remove code from the neural substrate we depend on, and `control` would
not even achieve that (its "cardinality guard" is documented in
`vocabulary/predicates.go` but unimplemented in beta.116). The high-cardinality
*triple* concern is real but separate, and is the framework's profile/cardinality
problem to solve — not a reason to drop embeddings.

## Consequences

### What this enables

- Agents get one fused, source-first call keyed by what they already know —
  no ID scheme, no GraphQL, no per-hop traversal, no re-`Read`.
- Reuses parse / watch / store / call-graph / embeddings / status gating from
  the SKG; the only new code is fusion/assembly + body hydration.
- A second lens (docs) lands almost for free, proving the primitive
  generalizes; config/URL lenses follow the same way.
- A clean upstream story: the engine becomes the deterministic counterpart to
  `research_graph` in semstreams.

### What is kept from the spike

The spike's **contract** (fused shape, `provenance`, the load-bearing
ready≠not-found envelope) and **assembly semantics** (relevance ranking,
byte budgeting, `did_you_mean`, call-path / blast-radius *shaping*) are
data-source-agnostic and port directly onto the engine's output and the code
lens's renderer.

### What is dropped

The spike's **sourcing** half — the parallel tree-sitter parse, the second
`fsnotify` watcher, and the in-memory symbol store — is removed. Freshness
comes from `ast-source`'s existing watch→graph pipeline; readiness from
`graph.query.status`.

### Deferred / out of scope

- **M:N concurrent-worktree read-your-writes on uncommitted edits.** The
  framework's single-graph model may not cover the eventual multi-agent
  dev-loop (ADR-0003 territory). Defer, and *prove* the framework cannot do it
  before building a per-worktree index — do not assume.
- **Upstreaming the engine to semstreams.** Follow-on ADR after the
  SemSource prototype validates the shape with two lenses.
- **MCP surface.** A thin wrapper over the same fusion contract for external
  clients (Cursor / Claude / Codex); deferred, not the way in for our own loop
  (which drives Go-registered tools, not an MCP client).

## References

- ADR-045 (semstreams) — `research_graph` rule-chain; this is its deterministic
  sibling.
- ADR-0005 — BFO/CCO ontology alignment; provides this gateway's ranking signal.
- ADR-054 (semstreams) — indexing-profile eligibility; code stays `content`.
- ADR-0003 — programmatic source-add; the deferred dynamic-worktree path.
- `semspec/prompt/tool_filter.go:36-45` — the grep-fallback post-mortem.
- `cmd/semsource/run.go` — `graph-query` wiring and the `graph.query.*` surface
  the gateway composes over.
