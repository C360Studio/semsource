# Proposal: UI Workbench Robustness

**Priority: P1** — the workbench is a stated requirement for Graphify ease-of-use comparisons

## Why

The browser review found the workbench's honesty architecture excellent (truthful readiness
lifecycle, capability gating, per-fact provenance) and its resilience/presentation layer carrying
six confirmed defects — three of which can take the page down or destroy state, and one of which
makes the flagship drill-down look broken even when it works:

1. **Duplicate-key crash**: the sources list is keyed without branch/language
   (`ui/src/lib/components/WorkbenchShell.svelte:186`) — two legitimate sources (two branches of
   one repo) throw Svelte's duplicate-key error and break the page.
2. **"Retention" that destroys**: on partial/truncated projections, previously-resolved nodes are
   downgraded to unresolved stubs, losing their facts (`ui/src/lib/graph/model.ts:120`) — the
   exact opposite of the spec's retain-prior-items promise.
3. **`reset_required` kills the workbench**: the capabilities parser rejects this legitimate index
   state (`ui/src/lib/contracts/capabilities.ts:150`), so the UI is down exactly during a
   graph-state incident.
4. Readiness is a one-shot snapshot — a "Not ready" panel never refreshes without a manual reload
   (`ui/src/routes/+page.svelte:56`); the "Overall: Ready" banner shows while the semantic index
   is still Building.
5. Error-contract gaps: only 400/503/504 fusion errors are honored (500/502/405/413 flattened;
   502 shown non-retryable) (`ui/src/lib/api/search.ts:124`); Retry with a cleared input silently
   dismisses errors (`GraphPanel.svelte:212`).
6. Drill-down presentation: the entity list is dominated by unresolved endpoints (local receiver
   calls, `builtin:`/`external:` markers, raw 6-part IDs for in-graph entities the backend didn't
   hydrate), auto-selects an unresolved stub, and buries the queried symbol. Backend hydration is
   tracked upstream; the UI must still present what it gets usefully. Plus: missing favicon (the
   only console error on the page).

## What Changes

- Fix the six defects above; drill-down groups/labels unresolved endpoints (resolved entities
  first, queried symbol selected by default; `builtin:`/`external:`/unhydrated grouped and
  de-emphasized), and readiness surfaces poll/refresh while any panel is not-ready.
- The overall-readiness banner reflects all advertised readiness dependencies (or names what it
  covers).
- Backend half of drill-down noise (neighbor hydration in the projection; local/builtin callee
  edge emission policy) is scoped to `go-callgraph-recall` and upstream asks — this change is
  UI-side presentation and resilience only.

## Capabilities

### Modified Capabilities
- `source-workbench`: "Truthful evidence presentation" (retention semantics, banner honesty),
  "Capability-gated explicit directed graph projection" (reset_required handling, unresolved
  grouping, refresh path), "Accessible graph investigation" (keyboard nav verified end-to-end —
  the audit could not confirm it manually) — delta adds resilience/presentation requirements to
  this capability.

### New Capabilities
<!-- none — all inside the existing source-workbench capability -->

## Impact

- `ui/src/lib/**`, `ui/src/routes/**`, Playwright smoke additions in `test/ui/` pinning: dup-key
  regression, reset_required render, retention-preserves-facts, refresh-on-not-ready.
- Ships as a new immutable workbench image via the existing publish/verify pipeline (ui-profile
  spec unchanged).
- Consumers: workbench operators; SemTeams packaging is unaffected (contracts unchanged).
- Boundary check: pure UI + tests; backend contract changes belong to other changes.
