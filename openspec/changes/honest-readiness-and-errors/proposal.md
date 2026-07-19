# Proposal: Honest Readiness and Error Surfaces

**Priority: P0** (audit 2026-07-19, findings: critical ×2, high ×1, medium ×3)

## Why

The audit confirmed that the two signals agents are told to trust — "phase is ready" and "the tool
call succeeded" — can both lie:

1. **`phase:"ready"` flips when sources have merely STARTED seeding, not finished**
   (`processor/source-manifest/status.go:106`). The documented consumer gate (poll status until
   ready) passes mid-seed; the readiness note's promise that "a miss once ready is a genuine
   absence" is provably false during the seed window — it instructs agents to hallucinate certainty.
2. **All MCP gateway tools use plain NATS `Request`, so ADR-060 handler-error replies are returned
   as SUCCESSFUL tool results** (`processor/mcp-gateway/component.go:165`) — infrastructure failure
   masquerades as an answer, detectable only by the absence of `contract_version`.
3. The README instructs consumers to poll HTTP `/source-manifest/status` for
   `index.ready`/`embedding.ready` — fields that endpoint has never carried (live-verified); honest
   readiness exists only on the MCP `source_status` tool and `/source-manifest/capabilities`.
4. `source_status` silently omits the `index`/`embedding` keys when their responders are down
   (`processor/mcp-gateway/query_tools.go:125`) — the exact signal agents gate on vanishes without
   explanation.
5. Status entity counts are monotone publish counters (`entity_count`, `total_entities`) inflated
   by the 60s periodic reindex (folder ×4, repo ×4 within minutes, live-verified; graph itself
   clean) — operators and UIs read ever-growing fiction.

## What Changes

- `phase` semantics: `ready` SHALL mean every configured source completed its initial seed (or is
  explicitly errored/degraded); a distinct observable phase covers "seeding in progress".
- The readiness note and MCP tool descriptions are corrected to state exactly what each signal
  guarantees, including the seed window.
- HTTP `/source-manifest/status` carries the same `index`/`embedding` readiness objects as the MCP
  tool (single status shape across surfaces), and the README's polling instructions become true.
- `source_status` SHALL report a failed readiness sub-query explicitly (e.g.
  `index: {available:false, reason}`), never by omitting the key.
- MCP gateway maps ADR-060 error envelopes / X-Status replies to MCP `isError` results with the
  envelope message — infrastructure failure is never shaped like an answer.
- Entity counts reported by status/summary reflect distinct persisted entities; publish/reindex
  activity, if reported, is a separately named counter.

## Capabilities

### New Capabilities
- `ingestion-readiness`: the aggregate phase, per-source phases, readiness sub-signals, and entity
  counts tell the truth on every surface (NATS, HTTP, MCP), with seed-window semantics explicit.
- `mcp-gateway-contract`: MCP tool results are honest — downstream errors surface as `isError`,
  argument validation stays strict, and every response is attributable (contract_version present on
  fusion-backed answers).

### Modified Capabilities
- `advertised-surface-coverage`: the "Documented status and query routes have behavior tests"
  requirement gains scenarios pinning the ready-means-seeded gate and the error-envelope mapping
  (the audit showed the only phase-transition test was shaped to sidestep exactly the mid-seed
  window).

## Impact

- `processor/source-manifest/` (status aggregation, phase computation, counts),
  `processor/mcp-gateway/` (Request wrapper, tool descriptions, source_status assembly),
  README + docs/integration/mcp-quickstart.md (polling contract).
- Consumers: semspec polls `graph.query.status` for phase transitions (recorded consumer behavior)
  — phase semantics change is **BREAKING in timing** (ready arrives later, truthfully); wire shape
  stays backward compatible. Consumer heads-up required in the PR notes.
- Boundary check: ADR-060 envelope format and X-Status headers are semstreams contracts; SemSource
  consumes them correctly here. If header propagation is impossible with the current client API,
  file a framework ask in docs/upstream/semstreams-asks.md rather than patching semstreams.
