# ADR-0009: SemSource-Owned Optional Workbench

> **Status:** Accepted — corrected ownership model approved by the SemSource owner, architect, and
> technical writer on 2026-07-15. | **Date:** 2026-07-15
> **Owners:** SemSource owns the workbench source, tests, packaging, and artifact. SemStreams owns the
> governed graph projection. SemTeams owns its application as a headless SemSource consumer.
> **Graph adoption:** [semstreams#533](https://github.com/C360Studio/semstreams/issues/533) was
> resolved by SemStreams PR #577; SemSource pinned `v1.0.0-beta.153` for adoption on 2026-07-19.

## Context

The archived `2026-07-15-add-ui-profile` change proved that SemTeams could consume SemSource through a
SemSource-owned Compose profile. Its D1 assigned the application to SemTeams, and D2 made
`../semteams/ui` the default checkout.

That integration proof is not the intended standalone product. It requires a sibling checkout,
launches a different product's application, and leaves standalone users without a focused surface for
source readiness, project search, evidence, and interoperability actions.

Several sem* product repositories contain descendants of the same Svelte graph interface. Their best
behaviors are distributed, and some copies contain unsafe inference or stale-update behavior. A first
attempt at this ADR assigned the SemSource product profile and release artifact to `semstreams-ui`.
That also missed the intended boundary: the goal is a SemSource-owned workbench that deliberately
ports the best proven behaviors and may later serve as a reference model.

## Decision

### 1. Headless SemSource remains the default and complete contract

`docker compose up`, direct binary use, MCP clients, and embedded consumers do not resolve, pull,
build, or start a UI. State-changing and artifact-producing workbench actions use SemSource backend
contracts available to non-UI automation. Browser-local presentation controls may remain local.

The browser is an optional standalone workbench, not a correctness dependency.

### 2. SemSource takes ownership of the existing `ui` flag

The supported deployment shapes are:

```text
docker compose up
docker compose --profile ui up
```

The existing `ui` profile launches the SemSource workbench. We do not add a second `workbench` flag.

This intentionally breaks the former behavior that launched SemTeams. SemTeams becomes a consumer of
headless SemSource and owns its own application packaging. Consumers that never selected the optional
profile are unaffected.

### 3. SemSource owns the workbench implementation

SemSource owns:

- the Svelte 5 source under `ui/`;
- browser contracts and project/source/readiness/search composition;
- the local graph model, renderer, adapter, layout, and accessible navigator;
- evidence fidelity, failure UX, component tests, and Playwright acceptance;
- Compose/Caddy integration; and
- the production workbench image and compatibility evidence.

SemStreams UI, SemSpec, SemDragon, and SemConnect are audited donors. SemSource may port selected
behavior and tests but does not import their source trees, packages, images, builds, or release
processes. Maintenance becomes local when behavior is ported.

SemStreams owns the governed graph projection. SemTeams owns its own application and packaging.

### 4. The workbench may become a model later

The SemSource UI should use clear internal module boundaries so proven graph, accessibility, search,
and evidence patterns can inform other sem* products. This ADR does not publish a shared package or
promise compatibility to external importers.

Extraction requires a later decision after the local implementation proves a stable API and another
product requests it. Premature packaging would freeze the interface before the product is understood.

### 5. Composition is capability-driven

The workbench bootstraps from SemSource's versioned `GET /source-manifest/capabilities` document. It
renders only advertised queries, actions, readiness signals, and project-view availability. It does
not probe SemTeams, flow-builder, trajectory, or admin routes and interpret expected failures as
SemSource degradation.

Project/source status, readiness, inventory, and search lead the experience. Graph is an evidence
drill-down, not the default explanation of a project.

### 6. The release artifact is SemSource-owned

The released profile uses an immutable image built from `ui/Dockerfile`, versioned for the matching
SemSource release and pinned by digest. Production use requires no sibling checkout, donor repository,
host Node installation, or local frontend build.

An explicit development path may build `./ui`. A mutable `latest` tag is not release evidence.

### 7. Adopt SemStreams #533 without moving graph authority

Fusion v1 supports project, search, list, detail, body, and impact views. It does not provide the
lossless directed, evidence-bearing graph projection required by the explorer.

SemStreams supplied the missing contract in
[PR #577](https://github.com/C360Studio/semstreams/pull/577), released as
[`v1.0.0-beta.153`](https://github.com/C360Studio/semstreams/releases/tag/v1.0.0-beta.153). SemSource
adopts its additive graph facet through the existing `POST /code-context/context` request with
`want: ["graph"]`; it does not add a projection endpoint or require GraphQL for the drill-down.

The workbench treats returned handles as opaque, creates unresolved display nodes only for explicit
edge endpoints, preserves typed facts, directed predicates, and supplied evidence, and keeps graph
truncation separate from top-level fusion truncation. Only an untruncated response with an equal,
coherent, nonzero view revision may delete previously displayed nodes or edges. Partial or
incoherent responses merge supplied updates without treating omissions as deletion.

SemSource still does not infer relationships from string shape, manufacture evidence, or build a
parallel graph substrate. The local WebGL/Sigma renderer, valid miss/not-ready behavior, classified
errors, stale-revision protection, and ready-graph browser/accessibility/live-route behavior are
covered by local and real-profile acceptance tests.

## Superseded and preserved decisions

This ADR supersedes only these decisions from the archived `2026-07-15-add-ui-profile` design:

- D1, “SemSource owns the profile, SemTeams UI owns the app.”
- D2, “Default UI checkout is `../semteams/ui`.”

The archived D3 same-origin routing and D4 active-polling principles remain useful, with the
SemSource-owned workbench as their UI target. Headless-default behavior and SemSource-owned validation
also remain compatible.

This corrected ADR also supersedes its own unaccepted shared-UI draft. That draft's external
`semstreams-ui` product-profile and release-ownership model has no implementation authority.

## Consequences

### Enables

- A focused standalone SemSource workbench without coupling embedded consumers to a frontend.
- Deliberate consolidation of the best sem* UI behaviors in a product-owned reference implementation.
- A later extraction decision grounded in a proven API rather than an assumed shared abstraction.
- SemTeams integration through stable headless contracts instead of repository coupling.

### Costs and risks

- SemSource now owns a Node/Svelte toolchain and frontend maintenance.
- Ported donor behavior can diverge; provenance and behavior tests must make that explicit.
- Existing `ui` profile users receive a breaking application change.
- The workbench must preserve the governed projection's completeness and revision semantics rather
  than treating every bounded response as deletion-authoritative.
- SemSource must maintain independent headless and workbench release evidence.

## Migration and rollback

The profile migrates only after the local workbench passes component, accessibility, Playwright, and
production-image gates. The migration release publishes the old-to-new command mapping and SemTeams
packaging handoff.

Rolling back the SemSource release restores the prior profile packaging. Rolling back only the new
workbench pins an earlier SemSource UI digest. Neither path changes graph state, and the headless core
remains available throughout.

## Rejected alternatives

- **Keep SemTeams as the default application.** This preserves the integration proof but does not
  provide a standalone SemSource product surface.
- **Put the SemSource product profile and artifact in `semstreams-ui`.** This replaces a sibling-source
  dependency with cross-repo product and release coupling and does not produce the intended local
  reference implementation.
- **Publish an importable component package first.** This freezes an API before the workbench proves
  which boundaries are stable or another consumer exists.
- **Make the UI mandatory.** This breaks embedded and agent-first consumers without improving backend
  correctness.
- **Add a second `workbench` profile.** This leaves two competing optional UI contracts to support.
- **Ship an inferred graph from fusion v1.** This loses direction, predicates, evidence, and revision
  truth and violates the SemStreams ownership boundary.
