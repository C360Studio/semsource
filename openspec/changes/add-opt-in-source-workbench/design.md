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

- **Renderer lifecycle:** SemSpec `ui/src/lib/components/graph/SigmaCanvas.svelte` supplies the SSR-safe
  `MultiDirectedGraph` lifecycle, initialization failure state, refresh, and cleanup reference.
- **Selection:** SemDragon's Sigma canvas supplies selected/neighbor emphasis and z-order behavior.
- **Search focus:** SemConnect's Sigma canvas supplies focused result sets instead of only selected
  adjacency.
- **Initial layout:** SemConnect supplies deterministic ID-seeded placement. Random positions and its
  quadratic degree work are rejected.
- **Force layout:** SemStreams UI `src/lib/utils/sigma-layout.ts` supplies bounded worker
  start/stop/restart behavior and tests.
- **Panel shell:** SemSpec layout components supply keyboard resize, input-safe shortcuts, persistence,
  and reduced-motion behavior when graph/detail panels justify the complexity.
- **Responsive access:** SemConnect route and app styles supply stack/drawer access that keeps evidence
  and filters reachable.
- **Source overview:** SemDragon graph summary and SemStreams UI overview supply the information
  architecture without their product vocabulary.
- **Search lifecycle:** SemStreams UI `NlqSearchBar.svelte` tests supply cancellation and elapsed-state
  behavior. SemSource implements local request generation and `AbortController` ownership.
- **Partial failure:** SemConnect settled refresh behavior supplies independent loading/error state;
  its semantic fallback and demo data are rejected.
- **Test foundation:** SemStreams UI unit/attack patterns inform equivalent SemSource-owned
  contract/component/Playwright gates.

The local implementation explicitly rejects:

- SemStreams UI's `graphTransform.ts` ID-shape relationship inference and manufactured evidence;
- plain undirected `Graph` when directed parallel relationships are required;
- random initial positions;
- synchronization based only on entity and relationship IDs;
- SemSpec's multiword-text-equals-NLQ heuristic; and
- donor product routes, fixtures, vocabulary, or styling copied without a SemSource contract.

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

### D9 - SemStreams #533 gates graph drill-down, not the workbench MVP

The beta.145 audit found fusion v1 sufficient for search/list/detail/body/impact views but not for a
lossless graph projection. Relation references omit target handles, predicates, direction, evidence,
edge identity, property facts, truncation detail, and a coherent view revision.

The framework gap is tracked in
[semstreams#533](https://github.com/C360Studio/semstreams/issues/533). Until it is adopted and
live-tested, the capability document reports `graph_projection: unsupported`, and the UI shows a
concrete unavailable state without requesting or inventing graph data.

The non-graph workbench MVP and its SemSource-owned image may ship before #533. Graph-enabled
drill-down acceptance remains blocked. SemSource SHALL NOT infer relationships from literal string
shape, manufacture evidence, or implement a parallel graph substrate.

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
result/detail navigation, and the graph-unavailable state while #533 remains open. Headless smoke
independently proves the UI profile and image are not resolved.

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
- **Graph code could outrun its contract.** → Keep graph unavailable until #533; start with explicit
  contract fixtures and never adapt fusion v1 into inferred edges.
- **Optional UI can become untested.** → Run independent headless and owned-workbench release gates.

## Migration Plan

1. Repair this change and ADR-0009 to record SemSource-local ownership; close the superseded shared-UI
   coordination issue.
2. Scaffold `ui/` with its owned Svelte 5, strict TypeScript, unit/component, Playwright, and Docker
   toolchain.
3. Implement capability bootstrap, project/readiness/source overview, and graph-unavailable state with
   failing tests first.
4. Add fusion search/list/detail with cancellation, stale-response, partial, empty, and error tests.
5. Replace the Compose/Caddy/Task/Playwright sibling path with the SemSource-owned image and `./ui`
   development build.
6. Run adversarial Svelte/accessibility review and live headless/workbench gates.
7. Publish and pin the exact SemSource UI image digest for release.
8. Graph fixtures and the local renderer may be prepared independently. After #533 is live, connect
   the governed adapter and run live graph acceptance.

Rollback pins the previous SemSource release or earlier SemSource UI digest. Neither path changes
graph state, and the headless core remains unchanged throughout.
