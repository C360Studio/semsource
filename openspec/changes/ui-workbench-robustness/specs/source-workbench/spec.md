# Delta: source-workbench

## ADDED Requirements

### Requirement: The workbench tolerates every legitimate backend state

The workbench SHALL render usefully for every state the backend contract can legitimately report —
including `reset_required` index states, fusion error statuses beyond 400/503/504, and source
inventories containing multiple instances of one repo (distinct branch/language) — degrading the
affected panel, never the whole page.

#### Scenario: reset_required degrades gracefully

- **WHEN** the capabilities contract reports index state `reset_required`
- **THEN** the workbench renders with the affected capability marked degraded/not-ready and the
  rest of the page functional

#### Scenario: Two branches of one repo

- **WHEN** the source inventory lists two sources differing only by branch
- **THEN** the sources list renders both entries (no duplicate-key failure)

### Requirement: Projection retention preserves facts

The workbench SHALL retain previously resolved graph items WITH their facts when a projection
response is truncated, incoherent, or zero-revision — a prior-resolved node SHALL never be downgraded to an
unresolved stub by a partial update.

#### Scenario: Partial update keeps resolved details

- **WHEN** a node resolved in an earlier projection is absent from a truncated later projection
- **THEN** the workbench continues to show the node with its previously known facts

### Requirement: Readiness display refreshes

Panels gated on readiness SHALL re-poll while not-ready and unlock without a manual page reload;
the overall-readiness banner SHALL reflect (or explicitly name) the readiness dependencies it
covers, never reporting blanket readiness while an advertised dependency is still building.

#### Scenario: Panel unlocks on readiness

- **WHEN** the semantic index becomes ready while the page is open
- **THEN** the code search panel becomes usable without a manual reload

### Requirement: Drill-down presents resolved knowledge first

The graph drill-down SHALL present resolved entities before unresolved endpoints, select the
queried symbol's entity by default when present, and group/de-emphasize unresolved endpoint
classes (`builtin:`, `external:`, unhydrated in-graph references) so investigation starts from
knowledge, not noise.

#### Scenario: Queried symbol selected

- **WHEN** a drill-down query resolves the queried symbol
- **THEN** that entity is selected by default and unresolved endpoints are grouped below the
  resolved entities
