# Design: UI Workbench Robustness

## Context

The audit's browser review confirmed six defects, all in `ui/` presentation/resilience — the
honesty architecture underneath (truthful readiness lifecycle, capability gating, per-fact
provenance) is sound and unchanged. Ground truth per site:

1. `WorkbenchShell.svelte` keys the sources list by `type:location` — two branches of one repo
   share both and throw Svelte's duplicate-key error, taking the page down.
2. `graph/model.ts` `syncGraph` on a partial projection seeds the merge from previous nodes
   (retention), but `incomingNodes` synthesizes `resolved:false, facts:[]` stubs for every edge
   endpoint absent from `projection.nodes` — and the merge loop overwrites retained resolved
   nodes with those stubs. Retention destroys exactly what it promises to keep.
3. `contracts/capabilities.ts` `readinessSignal` accepts index states
   `[ready, building, degraded, unknown]` — but the fusion contract (and backend) also emit
   `reset_required` (`contracts/fusion.ts` already accepts it). A capabilities payload carrying
   it fails parsing and the whole workbench dies during a graph-state incident.
4. `+page.svelte` runs `refresh()` once in `onMount` — a not-ready panel never updates without
   a manual reload; the overall banner can say Ready while the semantic index is Building.
5. `api/search.ts` honors the fusion error contract only for `[400, 503, 504]` — 500/502/405/413
   flatten to generic status errors (502 shown non-retryable); `GraphPanel.svelte`'s Retry
   re-submits with whatever is in the input, so a cleared input silently dismisses the error.
6. Drill-down entity list mixes unresolved endpoints (raw 6-part IDs, `builtin:`/`external:`
   markers) with resolved nodes in one flat list, can auto-select an unresolved stub, and buries
   the queried symbol. Plus the missing favicon (the page's only console error).

Constraint: UI-side only — backend hydration/edge-emission policy belongs to
go-callgraph-recall (shipped) and upstream asks. Contracts are unchanged; this ships as a new
immutable workbench image through the existing publish/verify pipeline.

## Goals / Non-Goals

**Goals:**

- No legitimate data shape can crash the page (dup sources, `reset_required`, partial
  projections).
- Retention retains: a partial merge never downgrades a previously-resolved node.
- Readiness self-heals: not-ready panels poll until ready; the banner names what it covers.
- The fusion error contract is honored for every status the backend classifies; Retry always
  re-runs the errored query.
- Drill-down presents resolved entities first with the queried symbol selected; unresolved
  endpoints are grouped and de-emphasized, never auto-selected.

**Non-Goals:**

- Backend neighbor hydration or callee-edge emission policy (tracked upstream / shipped in
  go-callgraph-recall).
- Contract or wire changes of any kind; SemTeams packaging unaffected.
- Visual redesign — presentation changes are ordering, grouping, and labeling only.

## Decisions

### D1: Sources keyed by full identity

The `{#each}` key becomes `type:location:branch:languages` (missing parts rendered as empty
segments). Branch (and language set) are the legitimate differentiators the audit hit; encoding
them all keeps the key stable AND unique without inventing synthetic indices (which would break
keyed-list reconciliation on reorder).

### D2: Merge never downgrades — stub-set only on absence

`syncGraph`'s merge applies an incoming node only if it is resolved OR the handle is not
already present (`if (!node.resolved && nodes.has(handle)) continue`). This preserves the
existing complete-projection path byte-for-byte (fresh map — nothing to downgrade) and makes
the partial path actually retentive. The fix lives in `syncGraph`, not `incomingNodes`, so the
stub synthesis (which the complete path legitimately needs for edge endpoints) stays intact.

### D3: `reset_required` joins the capabilities index-state contract

`readinessSignal`'s index-state list gains `reset_required` (matching `contracts/fusion.ts`,
which already accepts it — the two lists drifted). Presentation: a distinct not-ready state
("Index reset required — reindex in progress/needed"), never a parse failure. The
ready↔state consistency checks are unchanged.

### D4: Readiness polls while not-ready

`+page.svelte` keeps its one-shot `refresh()` on mount and adds an interval (10s) that calls
`bootstrap.refresh()` while any readiness surface is not ready; the interval stops when
everything is ready (and restarts if readiness regresses — cheap: derive from the same
capabilities state). Manual Retry stays. The overall banner derives from ALL advertised
readiness signals (source + index + embedding when present) and labels its coverage; Ready-
while-Building becomes impossible by construction.

### D5: Error contract honored by classification, not status allowlist

`search.ts` attempts the fusion-error parse for ANY non-ok response when the error contract is
advertised — the contract envelope itself says whether it applies; the status allowlist was a
proxy that drifted. Parse failure falls back to today's `statusError`. 502 inherits the
envelope's retryability (transient upstream = retryable). `GraphPanel`'s Retry re-submits the
LAST ERRORED query (captured at error time), not the live input — clearing the input can no
longer silently dismiss an error.

### D6: Drill-down grouping + favicon

The entity list renders in two labeled groups: resolved entities (queried symbol first and
auto-selected — matched by name against the submitted query; falls back to the first resolved
node) and "Unresolved endpoints" (raw handles, `builtin:`/`external:` markers — de-emphasized,
labeled by their marker kind, never auto-selected). Pure presentation over the existing
canonical graph model. A static favicon (existing logo asset or a minimal SVG) removes the
page's only console error.

### D7: Proof at two levels

Vitest unit pins for the pure logic (model merge retention, capabilities `reset_required`,
search error mapping, sources key derivation); Playwright specs in `ui/` for the rendered
behavior the audit exercised (dup-branch sources render, `reset_required` page alive,
partial-projection retention visible, not-ready panel refreshes without reload). The container
runner (`task ui:e2e`) stays the execution vehicle; no new infra.

## Risks / Trade-offs

- [Polling adds load] → one capabilities fetch per 10s only while not-ready; stops at ready.
- [Retry-last-errored-query surprises a user who edited the input] → the button is labeled with
  the errored query when it differs from the live input; submitting the form with new input
  still works unchanged.
- [Queried-symbol matching by name can miss (sanitized names)] → fallback is the first resolved
  node — strictly better than today's possible unresolved auto-select; exact-match seeds
  (go-callgraph-recall) make name-match the common case.
- [Broadened error-contract parsing could surface malformed envelopes] → parse failure falls
  back to the generic status error — never worse than today.

## Migration Plan

UI-only; ships as the next immutable workbench image through the existing trusted publish +
smoke pipeline. No contract, config, or compose changes. Rollback = previous image digest.

## Open Questions

- None blocking.
