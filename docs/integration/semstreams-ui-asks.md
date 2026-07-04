# SemSource — UI asks for the semstreams-ui team

**Status (2026-07-04):** the SemSource backend reasoning core is **MVP-validated live** — a 21,507-entity
semstreams graph was indexed and queried entirely over MCP; structural queries (context / callers /
callees / impact) decisively beat grep, and tier-1 semantic search (`code_search`) is reasoning-grade.
This doc is what we'd like the **semstreams-ui** team to build on top of the surfaces that already exist.
It is a request/contract doc, not a spec — the backend contracts below are live today unless a **gap** is
flagged.

## The surface to build against

SemSource exposes two equivalent read surfaces (both served by the running `semsource` process on its
HTTP port, default `:8080`):

- **MCP (Streamable-HTTP)** at `/mcp-gateway/mcp` — tools: `code_context`, `code_search`, `code_impact`,
  `doc_context`, `source_status`, `add_source`, `remove_source`. Bearer auth via `SEMSOURCE_API_TOKEN`
  (permissive when unset). This is the agent-facing surface.
- **REST** (per-component) — the same fusion queries as verbs:
  `/code-context/{context,callers,callees,impact,file,search}`, `/doc-context/{…}`,
  `/source-manifest/{sources,status,…}`. POST JSON `{"query":"<symbol-or-phrase>"}`. This is the
  convenient surface for a browser UI.

**Common response shape** (fusion contract v1):
```jsonc
{
  "index":  { "ready": true, "state": "ready", "indexed_revision": 53490, "target_revision": 53490, "lag": 0 },
  "provenance": "deterministic",            // or "embedding" for code_search
  "nodes": [ {
    "name": "rankEntities", "kind": "method", "path": "pkg/fusion/engine_lens.go", "lines": [254,282],
    "body": "func (e *Engine) rankEntities(...) { ... }",     // verbatim source
    "relations": { "caller": [{"name":"Fuse","path":"…"}], "callee": [{"name":"entitySalience","path":"…","line":301}], "container": [...] },
    "class": "http://www.ontologyrepository.com/CommonCoreOntologies/Algorithm",   // BFO/CCO class
    "handle": "c360.semsource.golang.semstreams.method.pkg-fusion-…-rankEntities"  // opaque = entity ID; do NOT parse
  } ],
  "impact": { "nodes": 5, "files": 3, "truncated": false },  // present for code_impact (blast-radius counts)
  "truncated": false, "contract_version": "1"
}
```

**Readiness is honest (semstreams ADR-066) — gate the UI on it.** Every response carries `index`
(structural readiness) and `source_status` also carries `embedding` (semantic readiness). Gate
structural views (context/callers/impact/byName) on `index.ready`; gate `code_search` reliability on
`embedding.ready`. Both expose `indexed_revision`/`target_revision`/`lag` for a live progress bar.

## Asks (priority order)

### 1. Source & health dashboard  ·  contract: EXISTS
Render `source_status` (MCP tool, or HTTP `/source-manifest/status`): namespace + overall `phase`;
per-source `{instance_name, source_type, phase, entity_count, error_count, type_counts}`; and the two
readiness blocks (`index`, `embedding`) with revision/lag for progress bars. This is the operator's
"is it up, what's indexed, is it ready to query" view. Everything is one call; no backend work needed.

### 2. Query console  ·  contract: EXISTS
A search/inspect panel that runs the query verbs and renders results richly:
- **byName / context** → the symbol card: verbatim `body`, `path:lines`, `kind`, ontology `class`,
  and resolved `relations` (callers/callees/contains) as clickable chips → navigate the graph by hop.
- **impact** → blast-radius summary (`impact.nodes`/`files`) — "changing this touches N symbols across M
  files" (the reasoning grep can't do).
- **search** → ranked hits with score ordering (tier-1 semantic; show `provenance`).
- **doc_context** → the docs-side equivalent (READMEs/ADRs/prose).
All served by the REST verbs / MCP tools above. No backend work needed.

### 3. Graph explorer  ·  contract: PARTIAL — likely one backend gap
Interactive visualization of entities + relationships (contains / calls / implements / references /
lineage). `code_context` already returns a node's direct `relations`, so a **click-to-expand** explorer
(seed → fetch neighbors on click) works against the existing contract today. **Gap to confirm:** a
bounded **subgraph-fetch** endpoint (neighborhood to depth K, capped) would make a smooth explorer
turnkey rather than N per-node calls — flag if you want it and we'll add it (small backend Tier-B item).

### 4. Versioned-source lineage view  ·  contract: EXISTS (our differentiator)
SemSource retains every indexed version of a source and relates them (ADR-0008, shipped). Surface:
- **supersession chains** per symbol — `v1.9.0 → v1.10.0 → v1.11.0` via `code.lineage.supersedes`
  (newer→older) / `code.lineage.superseded_by` edges;
- **current vs historical** — the current (un-superseded) version leads; historical versions are
  ranking-demoted but still present (a "show historical" toggle);
- **changed vs unchanged** per adjacent pair — `code.lineage.change`; drives a "what changed in `X`
  between `v1.9` and `v1.10`" diff view — the payoff of retaining versions.
Contract: the triples exist on the entities today (queryable via `graph.query.*` / relationships). A
dedicated lineage-walk query endpoint would make this turnkey — flag if wanted.

## Notes
- **Deployment:** the UI talks HTTP to a running `semsource` (self-hosts the whole graph stack; needs
  only a NATS+JetStream). A bundled `docker compose` (NATS + semsource + semembed) is a backend Tier-A
  item in flight — the UI can develop against a local `semsource run` today.
- **Auth:** set `SEMSOURCE_API_TOKEN` for the MCP bearer in any non-local deployment.
- **Tiers:** structural views (asks 1–4 except search ranking) work at every tier with no embedder;
  `code_search` NL quality needs tier-1 semembed running (see `configs/tiers/README.md`).

Owner: SemSource (Complete 360). Questions / contract changes: open an issue on the semsource repo.
