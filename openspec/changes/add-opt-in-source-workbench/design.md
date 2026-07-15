## Context

SemSource currently has an optional `ui` Compose profile that targets a sibling SemTeams UI checkout.
The archived `add-ui-profile` change intentionally treated that profile as integration plumbing and
rejected a SemSource-focused frontend. That decision was appropriate for proving SemTeams as a
consumer, but it leaves standalone SemSource users without a product surface for source readiness,
search, evidence, project views, and interoperability actions.

The C360 repositories also contain several copies of the Svelte Sigma/Graphology graph explorer. The
shared work is substantial, but `semstreams-ui` is currently a private application rather than a
published component library, and downstream copies contain improvements that are not present in its
version. The workbench must consolidate that work rather than create another copy.

This change records a product-boundary pivot: SemSource owns an optional standalone workbench
experience, while remaining headless by default. It supersedes both archived `add-ui-profile`
decisions without rewriting that history: SemTeams no longer owns the app launched by SemSource, and
`../semteams/ui` is no longer the default UI checkout.

## Goals / Non-Goals

**Goals:**

- Keep headless and embedded SemSource complete and UI-free by default.
- Repurpose the explicit standalone `ui` profile to use a pinned released SemSource UI artifact.
- Reuse `semstreams-ui` as the canonical shared shell and graph-visualization owner.
- Keep source, evidence, materialized-view, and OKF semantics in SemSource backend contracts.
- Make the graph a drill-down from useful project views rather than the whole product experience.
- Establish correctness and accessibility gates for the canonical graph surface.

**Non-Goals:**

- Changing SemTeams code or deciding how SemTeams packages its own UI.
- Implementing materialized views, OKF interoperability, or one-action installation here.
- Requiring an npm component package before the first released workbench profile.
- Moving source interpretation or graph authority into Svelte.
- Eliminating every downstream UI copy as part of the SemSource implementation.
- Adding production authentication, TLS, or arbitrary repository writes.

## Decisions

### D1 - Headless remains the default and complete product contract

`docker compose up`, direct binary execution, MCP clients, and embedded sem* consumers SHALL NOT
resolve, pull, build, or start a UI artifact. Workbench activation is explicit through the existing
optional `ui` profile. All meaningful workbench actions remain backed by HTTP, MCP, GraphQL, or CLI
contracts.

This preserves SemSource's role as an optional service inside SemTeams, SemSpec, SemDragon, SemOps,
and other products. A browser improves standalone usability; it is not required for correctness.

### D2 - SemSource takes ownership of the existing `ui` profile

The deployment shapes are:

```text
docker compose up                  headless SemSource
docker compose --profile ui up     optional SemSource workbench
```

The existing `ui` profile becomes the SemSource workbench; a second `workbench` profile is not
introduced. This is an intentional breaking change for users who used SemSource's profile to launch
SemTeams. SemTeams now owns that application packaging and can connect to SemSource through the
headless contracts. Embedded consumers that never selected the profile are unaffected.

### D3 - SemSource owns semantics; `semstreams-ui` owns reusable presentation

SemSource owns project/source identity, readiness meaning, source inventory, provenance, authority,
freshness, materialized-view definitions, OKF behavior, backend APIs, and workbench acceptance tests.

`semstreams-ui` owns the reusable Svelte shell, graph renderer, layouts, generic search/filter/detail
components, accessible graph navigation, product-profile seam, and released UI artifact. A
SemSource-specific product profile can live in that UI repository while its product contract remains
defined and tested from SemSource.

SemSource SHALL NOT copy the renderer into this repository. The first delivery may consume a pinned
application image; publishing a standalone component package is optional follow-up work.

### D4 - Canonicalize the best graph implementation before workbench dependency

The shared UI team must compare the live graph implementations across `semstreams-ui`, SemTeams,
SemSpec, SemDragon, and SemConnect. The inventory records the strongest implementation and tests for:

- directed and multi-edge graph modeling;
- renderer lifecycle and attribute-aware updates;
- layout start, stop, refresh, and failure behavior;
- filters, selection, detail, search, and community navigation;
- truthful confidence, provenance, authority, and timestamp presentation;
- loading, empty, disconnected, and query-not-ready states;
- keyboard and screen-reader alternatives to WebGL interaction;
- responsive layout, test seams, performance, and large-project evidence; and
- packaging, tokens, and product-profile boundaries.

The initial live-repo audit selects this composite rather than treating any current copy as complete:

| Concern | Canonical source behavior |
| --- | --- |
| Renderer structure | SemSpec: lazy initialization, `MultiDirectedGraph`, failure state, layout refresh |
| Selection | SemDragon: explicit selected-node emphasis and selected/neighbor z-ordering |
| Search focus | SemConnect: focused result sets rather than only selected-node adjacency |
| Initial layout | SemConnect: deterministic ID-seeded positions, without its quadratic degree calculation |
| Force layout | `semstreams-ui`: worker-based bounded ForceAtlas2 controller |
| Panel shell | SemSpec: persisted panels, keyboard resize, safe shortcuts, and reduced-motion handling |
| Responsive access | SemConnect: stack/drawer behavior that keeps filters and evidence reachable |
| Filters | Composite: SemStreams dimensions/previews, SemDragon counts, SemConnect state, SemSpec totals |
| Entity/evidence detail | Composite: SemSpec identity/copy, SemConnect source-per-fact, shared context action |
| Source overview | SemDragon source/readiness cards plus the compact `semstreams-ui` graph overview |
| Search lifecycle | `semstreams-ui`: browse/replace/merge, cancellation, duration, errors, and result list |
| Partial failure | SemConnect: settled loading, cancellation, partial status, live/hybrid/error states |
| Test foundation | `semstreams-ui`: unit/attack coverage and the live SemSource Playwright stack |

SemTeams contributes the integration consumer and compatibility surface but no distinct newer graph
behavior in the audited copy. `semstreams-ui` remains the canonical destination and test harness;
SemSpec supplies the renderer/layout foundation, with selected interaction and resilience patterns
ported from SemDragon and SemConnect.

The canonical search behavior explicitly does not adopt SemSpec's heuristic that treats multiword text
as natural-language intent. Search classification remains a backend contract with visible evidence.

Two gates are non-negotiable. Missing evidence values remain unknown rather than being manufactured,
and stable entity/relationship IDs must not suppress attribute-only updates. The current shared code
does not yet meet either gate consistently. The canonical graph model must also preserve direction and
parallel predicates explicitly; it must not infer edges by testing whether a triple object merely looks
like a six-part entity ID.

### D5 - Released UI artifacts replace sibling-checkout requirements

The `ui` profile pulls a pinned `semstreams-ui` release image or immutable artifact.
End users do not need a sibling repository checkout, Node installation, or local UI build. A
SemSource-specific source override such as `SEMSOURCE_UI_CONTEXT` may remain available only for
development and compatibility tests. The former `UI_CONTEXT` meaning is not a stable production
contract.

The SemSource release pins the compatible artifact version or digest and runs a real SemSource
Playwright smoke against that pin.

### D6 - Workbench composition is capability-driven

A SemSource-owned browser capability contract tells the shared UI which product identity, readiness,
query surfaces, materialized views, and actions are available. Unsupported capabilities are absent or
explicitly unavailable; the UI does not probe SemTeams-only or other product routes and interpret
expected failures as degraded SemSource.

This change may start with existing status, source-manifest, search, and graph contracts. Later
materialized-view and OKF changes extend the capability contract without moving their semantics into
the UI.

### D7 - Project knowledge leads; the graph is drill-down

The workbench landing experience prioritizes project/source status, readiness and freshness, search,
source inventory, and opinionated materialized views when available. The graph explorer supports
investigation and evidence traversal, but a whole-graph canvas is not the default explanation of a
project.

The WebGL graph must have a synchronized, accessible entity/result navigator. Automated acceptance
tests must exercise user-visible list/detail behavior rather than depending only on browser test hooks
that bypass the canvas.

### D8 - Product actions execute through one backend contract

Future OKF preview/export, import validation, materialized-view refresh, and source actions call the
same SemSource backend contracts used by CLI and MCP automation. The browser does not implement a
second codec, planner, evidence model, or authority policy.

For the initial OKF slice, a browser download is preferred over granting server-side write authority.
Managed repository publication remains an explicit later mode.

### D9 - Explicit graph projection stays in the governed query boundary

The workbench needs explicit nodes, directed relationships, property facts, evidence metadata, and a
query/view revision. Before UI implementation, the architect must audit the live SemStreams graph
query/gateway contract to determine whether that payload already exists. If it does not, SemSource
records the framework gap in `docs/upstream/semstreams-asks.md` and raises it with the SemStreams team.

The beta.145 audit found that lens-driven fusion v1 already supports the primary workbench
search/list/detail/body/relations/impact views through SemSource HTTP, but it is not a lossless graph
projection: relation refs omit target handles, predicates, direction, evidence, and edge identity;
property facts are absent; relationship truncation is silent; and index readiness is not a coherent
view revision. The framework gap is recorded as upstream ask 18 and
[semstreams#533](https://github.com/C360Studio/semstreams/issues/533). Fusion-backed non-graph views
may proceed; the canonical graph drill-down remains gated on adoption and live validation of that
additive governed projection contract.

SemSource SHALL NOT infer relationships from literal string shape, manufacture evidence, or implement
a parallel graph substrate to satisfy the UI. The pinned workbench release is blocked until a real
backend payload and live compatibility test prove the required semantics.

## Follow-on change sequence

These IDs are proposed planning handles, not created or approved changes:

- `canonicalize-shared-graph-workbench` (`semstreams-ui`): after the cross-repo contract, deliver the
  canonical renderer and SemSource profile.
- `materialize-project-views` (SemSource): after the query/readiness contract, deliver bounded
  project-scoped materialized context views.
- `add-okf-interop-mvp` (SemSource): after project views, deliver consume-only import and bounded OKF
  export.
- `add-one-action-local-start` (SemSource): after the released workbench artifact, deliver headless
  start plus an explicit UI option.

A materialized project view is the project-scoped profile of the design note's generic materialized
context view. The profile narrows subjects, evidence, revision, and budgets; it is not a second
abstraction or an OKF page.

## Risks / Trade-offs

- **Cross-repo delivery can drift.** → Pin the UI artifact and gate SemSource releases with a real
  compatibility smoke.
- **A shared UI can absorb product semantics.** → Keep capability definitions, actions, and acceptance
  scenarios in SemSource; pass typed data to generic presentation components.
- **The `ui` flag changes meaning.** → Mark the takeover as breaking, publish the old-to-new mapping,
  and hand SemTeams packaging to the SemTeams team before release.
- **Canonicalization can become a large rewrite.** → Inventory and select behavior first; require only
  the renderer, evidence, accessibility, and product-profile gates needed by the initial workbench.
- **A graph canvas can dominate the UX because it already exists.** → Make project views and source
  status the primary navigation and treat visualization as drill-down.
- **Optional can become untested.** → Run both headless and pinned-workbench smokes in the relevant
  release gate.

## Migration Plan

1. Completed: archive `add-ui-profile` to preserve its SemTeams integration decision as historical
   truth and seed the current `ui-profile` specification.
2. Accept the cross-repo contract, then record the breaking ownership pivot and both superseded
   decisions in ADR-0009.
3. Complete the cross-product graph UI inventory in a coordinated `semstreams-ui` change.
4. Prove or obtain the explicit governed graph/evidence/revision query contract.
5. Fix evidence fidelity, attribute refresh, accessible navigation, and selected canonical behaviors.
6. Add the SemSource product profile, then publish the final pinned `semstreams-ui` artifact.
7. Replace the existing Compose `ui` service target with the pinned artifact and publish the SemTeams
   handoff and release note.
8. Prove headless and workbench behavior against the same SemSource revision.
9. Propose the follow-on changes in the dependency order recorded above.

Operational rollback pins the previous SemSource release or restores the former `ui` service
definition. The additive capability endpoint may remain inert, and no graph-state migration is
involved. The independently versioned shared UI artifact is rolled back in `semstreams-ui`, never by
changing SemTeams code from SemSource. The headless core remains unchanged throughout.

## Open Questions

1. Should the pinned workbench be a separate container image or static assets embedded into a small
   SemSource web service?
2. What compatibility/version field should the SemSource browser capability document use?
