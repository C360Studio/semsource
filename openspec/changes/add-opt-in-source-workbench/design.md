## Context

SemSource currently has an optional `ui` Compose profile that targets a sibling SemTeams checkout.
The archived `add-ui-profile` change intentionally treated that profile as integration plumbing and
rejected a SemSource-focused frontend. That decision proved SemTeams could consume SemSource, but it
does not give standalone SemSource users a focused surface for source readiness, search, evidence,
project views, and interoperability actions.

The C360 repositories contain several descendants of the same Svelte Sigma/Graphology interface.
No copy is complete: SemSpec has the strongest renderer lifecycle and panel shell, SemDragon has
stronger selection and source summaries, SemConnect has deterministic placement and responsive
patterns, and SemStreams UI has mature search and worker tests. Some copies also infer edges from
ID-shaped literals, fabricate evidence, use random layout, or miss attribute-only updates.

The corrected product decision is to build a SemSource-owned reference workbench under `ui/` by
porting selected behavior deliberately. Donor repositories are evidence and design inputs only. They
are not runtime, build, package, acceptance, or release dependencies.

## Goals / Non-Goals

**Goals:**

- Keep headless and embedded SemSource complete and UI-free by default.
- Repurpose the explicit `ui` profile for a SemSource-owned optional workbench.
- Build and test the workbench in this repository under `ui/`.
- Port the strongest audited behaviors from sem* product UIs with local ownership after port.
- Keep source, evidence, materialized-view, OKF, and action semantics in SemSource backend contracts.
- Make project/source readiness and search useful before graph drill-down is available.
- Establish correctness, accessibility, and failure-state gates for later governed graph support.

**Non-Goals:**

- Changing SemTeams code or deciding how SemTeams packages its own UI.
- Publishing a generic Svelte component package in this change.
- Depending on sibling UI checkouts or donor packages at runtime or build time.
- Implementing materialized views, OKF interoperability, or one-action installation here.
- Moving source interpretation or graph authority into Svelte.
- Adding production authentication, TLS, or arbitrary repository writes.

## Decisions

### D1 - Headless remains the default and complete product contract

`docker compose up`, direct binary execution, MCP clients, and embedded sem* consumers SHALL NOT
resolve, pull, build, validate, or start a UI artifact. Workbench activation is explicit through the
existing optional `ui` profile.

Every state-changing or artifact-producing workbench action uses a SemSource-owned backend contract
available to non-UI automation. Browser-local layout, filter, and selection controls may remain local.

### D2 - SemSource takes ownership of the existing `ui` profile

The deployment shapes are:

```text
docker compose up                  headless SemSource
docker compose --profile ui up     optional SemSource workbench
```

The existing `ui` profile becomes the SemSource workbench; a second `workbench` profile is not
introduced. This intentionally breaks the former behavior that launched SemTeams. SemTeams now owns
its own application packaging and connects through SemSource's headless contracts.

### D3 - SemSource owns the workbench source and artifact

SemSource owns:

- the Svelte 5 application under `ui/`;
- product/project/source composition and browser contract types;
- the local canonical graph model, renderer, adapter, layout, and accessible navigator;
- evidence fidelity, failure UX, component tests, and Playwright acceptance;
- Compose/Caddy integration and the released UI image; and
- maintenance of code deliberately ported from donor repositories.

SemStreams owns the governed graph projection contract. SemTeams owns its application. SemStreams UI,
SemSpec, SemDragon, and SemConnect are audited donors only; SemSource SHALL NOT import their source,
packages, build outputs, containers, or release artifacts.

The local implementation may later serve as a reference for a shared package or upstream
canonicalization. That extraction requires a separate decision after at least one external consumer
and a stable API exist.

### D4 - Port a tested best-of composite into `ui/`

The initial audit selects behavior rather than declaring any donor application canonical:

The evidence below was audited at SemSpec `5a9496eecc45`, SemDragon `9417173a0b19`, SemConnect
`a8d1a7d15a94`, and SemStreams UI `3814b3d59dab`. Those revisions make the evidence reproducible;
they do not create a dependency or require later donor changes to be merged.

- **Renderer lifecycle**
  - Donor evidence: SemSpec `ui/src/lib/components/graph/SigmaCanvas.svelte` guards browser-only
    imports, uses `MultiDirectedGraph`, reports initialization/render failures, refreshes, and stops
    the layout and Sigma renderer during cleanup.
  - Local destination: `ui/src/lib/graph/SigmaCanvas.svelte` and
    `ui/src/lib/graph/SigmaCanvas.test.ts`.
  - Port rationale: preserve SSR-safe initialization, truthful failure UI, refresh, and cleanup.
  - Rejected: SemSpec product types, route coupling, and any graph adaptation not backed by the local
    governed graph contract fixtures.

- **Selected-neighbor emphasis**
  - Donor evidence: SemDragon `ui/src/lib/components/graph/SigmaCanvas.svelte` uses node and edge
    reducers to dim non-neighbors and place the selected node and neighbors at explicit z-indexes.
  - Local destination: `ui/src/lib/graph/selection.ts` and `ui/src/lib/graph/selection.test.ts`.
  - Port rationale: keep selection legible without making canvas state the source of truth.
  - Rejected: SemDragon entity types, colors, route state, and relationship data shaping.

- **Search-result focus**
  - Donor evidence: SemConnect `ui/src/lib/components/SigmaCanvas.svelte` accepts a focused-ID set
    independently of the selected entity and uses reducers to emphasize all focused results.
  - Local destination: `ui/src/lib/graph/selection.ts` and the synchronized result navigator under
    `ui/src/lib/components/search/`.
  - Port rationale: search may focus several results while keyboard selection remains singular.
  - Rejected: SemConnect demo types and its ID-only graph signature, which misses attribute-only
    updates.

- **Deterministic initial placement**
  - Donor evidence: SemConnect `ui/src/lib/utils/graphology-adapter.ts` derives x/y coordinates from
    the entity ID and preserves existing coordinates across synchronization.
  - Local destination: `ui/src/lib/graph/seeded-position.ts` and
    `ui/src/lib/graph/seeded-position.test.ts`.
  - Port rationale: make initial placement repeatable before the worker layout settles.
  - Rejected: random positions, demo colors/types, and the donor's per-node relationship scan for
    degree-derived size.

- **Bounded force-layout worker lifecycle**
  - Donor evidence: SemStreams UI `src/lib/utils/sigma-layout.ts` and
    `src/lib/utils/sigma-layout.test.ts` cover start, stop, timed shutdown, restart, and worker kill.
  - Local destination: `ui/src/lib/graph/force-layout.ts` and
    `ui/src/lib/graph/force-layout.test.ts`.
  - Port rationale: prevent leaked workers and unbounded background layout work.
  - Rejected: treating the donor timeout and tuning constants as a SemSource contract.

- **Keyboard-safe panel shell**
  - Donor evidence: SemSpec `ui/src/lib/components/layout/ThreePanelLayout.svelte`,
    `ResizeHandle.svelte`, and `ui/src/lib/stores/panelState.svelte.ts` cover keyboard resize,
    input-safe shortcuts, bounded widths, persistence, responsive collapse, and reduced motion.
  - Local destination: `ui/src/lib/components/layout/WorkbenchLayout.svelte`,
    `ResizeHandle.svelte`, and `ui/src/lib/state/panel-layout.svelte.ts`.
  - Port rationale: add the complexity only when graph and detail panels need persistent layout.
  - Rejected: SemSpec route structure, product shortcuts, panel defaults, and storage keys.

- **Responsive stacking**
  - Donor evidence: SemConnect `ui/src/routes/+page.svelte` and `ui/src/app.css` use explicit 1180 px
    and 820 px breakpoints and retain visible focus treatment while the shell collapses.
  - Local destination: `ui/src/routes/+page.svelte` and `ui/src/app.css`.
  - Port rationale: keep sources, search results, evidence, and filters reachable at narrow widths.
  - Rejected: the SemConnect demo shell, fixed product copy, icons, and telemetry-specific regions.

- **Source overview information architecture**
  - Donor evidence: SemDragon `ui/src/lib/components/graph/GraphSummary.svelte` separates loading,
    error, empty, source, readiness, count, and refresh states. SemStreams UI
    `src/lib/components/runtime/GraphOverviewPanel.svelte` adds compact status and filter summaries.
  - Local destination: `ui/src/lib/components/SourceOverview.svelte` and its component tests.
  - Port rationale: expose source readiness and inventory before graph drill-down exists.
  - Rejected: donor graph counts as authority, raw agent-output panels, and donor product vocabulary.

- **Search cancellation and elapsed state**
  - Donor evidence: SemStreams UI `src/lib/components/runtime/NlqSearchBar.svelte`,
    `NlqSearchBar.cancel.test.ts`, and `NlqSearchBar.test.ts` cover cancel controls, elapsed loading,
    keyboard submission, accessible names, empty input, and error state.
  - Local destination: `ui/src/lib/components/SearchPanel.svelte`, `ui/src/lib/api/search.ts`, and
    `ui/src/routes/+page.svelte`.
  - Port rationale: keep long or replaced searches observable and cancellable.
  - Rejected: donor NLQ product copy and SemSpec's heuristic that treats any multiword text as NLQ.

- **Independent partial-failure state**
  - Donor evidence: SemConnect `ui/src/lib/stores/demoStore.svelte.ts` uses `Promise.allSettled`,
    retains fulfilled data, records per-source failures, and owns an `AbortController` per refresh.
  - Local destination: `ui/src/lib/api/workbench.ts`, `ui/src/routes/+page.svelte`, and
    `ui/src/lib/components/WorkbenchShell.svelte`.
  - Port rationale: an optional inventory or search failure must not discard valid capabilities.
  - Rejected: hybrid/demo fallback, merging CS API and graph facts in the browser, and fabricated data.

- **Failure-oriented test patterns**
  - Donor evidence: SemStreams UI `src/lib/components/runtime/SigmaCanvas.attack.test.ts`,
    `NlqSearchBar.attack.test.ts`, and `src/lib/services/graphTransform.test.ts` exercise lifecycle,
    malformed, empty, and adversarial cases.
  - Local destination: colocated `ui/src/lib/**/*.test.ts` suites plus `ui/e2e/` acceptance tests.
  - Port rationale: retain the adverse cases while making all fixtures and assertions SemSource-owned.
  - Rejected: importing donor test helpers, fixtures, packages, test IDs, or expected product copy.

SemStreams UI `src/lib/services/graphTransform.ts` is rejection evidence, not a donor behavior: its
ID-shaped-literal relationship inference and manufactured evidence SHALL NOT enter the local adapter.
The local graph also rejects plain undirected `Graph`, ID-only synchronization, and any donor route,
fixture, vocabulary, or style without a SemSource contract and a failing local behavior test first.

The first implementation slice does not need Sigma. It proves capability-driven project, readiness,
source inventory, and search behavior, with graph drill-down truthfully unavailable. Graph code is
added only behind explicit local contract fixtures and later connected to the governed backend.

### D5 - The release artifact is SemSource-owned

Source and development builds live at `ui/`. The released profile uses an immutable image built from
`ui/Dockerfile`, with the intended repository and naming shape:

```text
ghcr.io/c360studio/semsource-ui:<matching-semsource-version>@sha256:<digest>
```

The development and compatibility path may explicitly build `./ui`. The released profile requires no
sibling checkout, host Node toolchain, or donor repository. Release evidence records the SemSource
commit, UI image version/digest, and exact gates run against that digest.

The UI image is independently replaceable but versioned with SemSource compatibility. A mutable
`latest` tag is not a production acceptance artifact.

### D6 - Workbench composition is capability-driven

`GET /source-manifest/capabilities` tells the UI which product identity, readiness signals, query
surfaces, project views, and actions exist. Unsupported behavior is absent or explicitly unavailable;
the UI does not probe SemTeams, flow-builder, trajectory, or other product routes.

The first workbench uses supported source inventory/status/summary and fusion search/context routes.
Later materialized-view and OKF changes extend the capability map without moving their semantics into
the browser.

### D7 - Project knowledge leads; graph is drill-down

The landing experience prioritizes project/source status, readiness and freshness, search, source
inventory, and opinionated project views when available. Whole-graph visualization is not the default
explanation of a project.

When enabled, the WebGL graph has a synchronized keyboard- and screen-reader-accessible entity/result
navigator. Acceptance tests exercise visible list/detail behavior rather than canvas-only test hooks.

### D8 - Product actions execute through one backend contract

Future OKF preview/export, import validation, materialized-view refresh, and source actions call the
same SemSource backend contracts used by CLI, MCP, or HTTP automation. The browser does not implement
a second codec, planner, evidence model, or authority policy.

For the initial OKF slice, a browser download is preferred over server-side write authority. Managed
repository publication remains a later explicit mode.

### D9 - Adopt the SemStreams #533 governed projection without a parallel surface

The beta.145 audit found fusion v1 sufficient for search/list/detail/body/impact views but not for a
lossless graph projection. SemStreams resolved that framework gap in
[PR #577](https://github.com/C360Studio/semstreams/pull/577), shipped it in
[`v1.0.0-beta.153`](https://github.com/C360Studio/semstreams/releases/tag/v1.0.0-beta.153), and closed
[semstreams#533](https://github.com/C360Studio/semstreams/issues/533). SemSource adopts the additive
facet through its existing `POST /code-context/context` route by sending `want: ["graph"]`. This
change adds no graph-projection endpoint, no browser reconstruction, and no GraphQL dependency.

The returned `graph` facet is authoritative for typed property facts, explicit directed edges,
original predicates, opaque source and target handles, optional verbatim evidence, view revision,
and graph-local truncation. A handle absent from the returned node details may appear only as an
explicit edge endpoint and is displayed as unresolved; dotted or six-part-looking literals never
create an edge. Missing evidence remains missing.

A response is deletion-authoritative only when its graph projection is untruncated and its view
revision is coherent, equal at start and end, and nonzero. Truncated, incoherent, or zero-revision
responses merge supplied updates but retain previously known nodes and edges, identify the view as
partial, and invite retry. Top-level fusion truncation remains separate from `graph.truncated` and
does not erase the graph facet's explicit completeness signal.

### D10 - Bootstrap through one product-owned HTTP document

The workbench bootstraps from `GET /source-manifest/capabilities`. The source-manifest component owns
the document because it is always present in headless SemSource and already owns authoritative source
status and inventory.

The version-1 response contains:

- `contract_version` and stable product identity;
- a deployment-namespace `project.key`, explicitly not a six-part entity ID;
- source, structural-index, and semantic-index readiness;
- query and action maps with `ready`, `not_ready`, or `unsupported` availability;
- project-view availability; and
- additive contract metadata.

Unknown fields and map keys are additive within version 1. Missing or timed-out optional readiness
responders degrade only the affected capability with a sanitized reason. The implemented Go contract
and its tests remain authoritative for the exact wire example.

### D11 - The workbench owns its test toolchain

The `ui/` directory owns its package lock, format/lint/Svelte checks, unit/component tests, build,
accessibility assertions, and Playwright dependency. SemSource's `task ui:e2e` and `task ui:smoke` use
that toolchain or the released image's owned test runner; they never borrow SemTeams dependencies.

The live smoke verifies capabilities, readiness, source inventory, search/query behavior, keyboard
result/detail navigation, and the capability-advertised graph state. Headless smoke independently
proves the UI profile and image are not resolved.

The UI-profile Playwright smoke owns Caddy integration proof. It exercises the shell and every
advertised proxied route through the profile entry point and rejects misleading UI-HTML fallthrough.
The standalone UI image gate does not claim to validate Caddy or backend route wiring.

The canonical owned gates are:

- install: `npm --prefix ui ci`;
- formatting: `npm --prefix ui run format:check`;
- lint: `npm --prefix ui run lint`;
- Svelte and TypeScript: `npm --prefix ui run check`;
- unit and component behavior: `npm --prefix ui test`;
- accessibility: `npm --prefix ui run test:a11y`;
- production bundle: `npm --prefix ui run build`;
- browser acceptance: `npm --prefix ui run test:e2e`; and
- production image: `task ui:image:verify`.

`test:a11y` is a separate release gate, not a synonym for Svelte compile success. It covers automated
accessibility rules, accessible names and state announcements, keyboard-only focus order and result
selection, narrow-width reachability, and reduced-motion behavior. Playwright asserts visible roles,
names, list/detail state, and focus; canvas-only test hooks cannot satisfy the gate.

`ui:image:verify` builds `ui/Dockerfile` from a clean `ui/` context with no host `node_modules`, cache,
bind mount, or sibling checkout. It starts the resulting production image as its declared non-root
user with no development server or donor dependency, verifies the runtime UID is non-zero, waits for
image health, and fetches SemSource shell content directly from the container port. It records the
tested image ID and OCI content digest. A locally cached image, mutable `latest`, bind-mounted build
output, or Vite development server cannot satisfy this gate. Task 7.3 separately ties the tested
content to the registry digest, SemSource commit, and release version.

The accessibility and image-verification scripts may be introduced during implementation, but task
2.3 remains incomplete until every command above exists in SemSource and passes without SemTeams.

## Follow-on change sequence

These IDs remain proposed planning handles, not created or approved changes:

- `materialize-project-views`: bounded project-scoped materialized context views;
- `add-okf-interop-mvp`: consume-only import plus bounded OKF export; and
- `add-one-action-local-start`: headless start plus an explicit UI option.

A future `extract-shared-workbench-components` change is considered only after the SemSource UI proves
a stable model and another product requests an importable contract.

## Risks / Trade-offs

- **SemSource now owns frontend maintenance.** → Keep the first UI small, contract-driven, and tested;
  port behaviors selectively rather than copying whole applications.
- **Donor fixes may diverge.** → Record source provenance and behavior tests; local ownership begins at
  port time, with future upstreaming handled separately.
- **The `ui` flag changes meaning.** → Publish the old-to-new mapping and SemTeams handoff.
- **Graph code could outrun its contract.** → Pin the adopted beta.153 contract, keep explicit local
  fixtures, require a meaningful coherent nonzero revision for deletion, and never adapt fusion v1
  role maps into inferred edges.
- **Optional UI can become untested.** → Run independent headless and owned-workbench release gates.

## Migration Plan

1. Repair this change and ADR-0009 to record SemSource-local ownership; close the superseded shared-UI
   coordination issue.
2. Scaffold `ui/` with its owned Svelte 5, strict TypeScript, unit/component, Playwright, and Docker
   toolchain.
3. Implement capability bootstrap, project/readiness/source overview, and a capability-gated graph
   state with failing tests first.
4. Add fusion search/list/detail with cancellation, stale-response, partial, empty, and error tests.
5. Replace the Compose/Caddy/Task/Playwright sibling path with the SemSource-owned image and `./ui`
   development build.
6. Run adversarial Svelte/accessibility review and live headless/workbench gates.
7. Publish and pin the exact SemSource UI image digest for release.
8. Adopt the beta.153 `want: ["graph"]` facet through the existing code-context route, connect the
   local renderer, and run compatibility and partial-view acceptance.

Rollback pins the previous SemSource release or earlier SemSource UI digest. Neither path changes
graph state, and the headless core remains unchanged throughout.
