# ADR-0009: Headless SemSource with an Optional Shared Workbench

> **Status:** Proposed — SemSource ownership and architecture review approved on 2026-07-15;
> shared UI owner acceptance remains pending in
> [semstreams-ui#2](https://github.com/C360Studio/semstreams-ui/issues/2) and its coordinated OpenSpec
> change. | **Date:** 2026-07-15
> **Cross-repo owners:** SemSource owns product semantics and deployment; `semstreams-ui` owns reusable
> presentation and the released artifact; SemStreams owns governed graph query contracts; SemTeams
> owns its application packaging as a headless SemSource consumer.
> **Graph release gate:** [semstreams#533](https://github.com/C360Studio/semstreams/issues/533)

## Context

The archived `2026-07-15-add-ui-profile` change proved that SemTeams could consume SemSource through a
SemSource-owned Compose profile. Its D1 assigned the application to SemTeams, and D2 made
`../semteams/ui` the default checkout.

That integration proof is not the intended standalone product. It requires a sibling source checkout,
launches a different product's application, and leaves standalone users without a focused surface for
source readiness, project search, evidence, and interoperability actions.

The C360 UI repositories also contain several descendants of the same Svelte graph explorer. The
strongest behaviors are distributed across those copies. Creating another renderer in SemSource would
increase drift instead of establishing a reusable owner.

## Decision

### 1. Headless SemSource remains the default and complete contract

`docker compose up`, direct binary use, MCP clients, and embedded consumers do not resolve, pull,
build, or start a UI. Every state-changing or artifact-producing workbench action uses a
SemSource-owned backend contract available to non-UI automation. Local presentation controls may
remain browser-local.

The browser is an optional standalone workbench, not a correctness dependency.

### 2. SemSource takes ownership of the existing `ui` flag

The supported deployment shapes are:

```text
docker compose up
docker compose --profile ui up
```

The existing `ui` profile launches the SemSource workbench. We do not add a second `workbench` flag.

This is a deliberate breaking change for users who relied on the SemSource profile to launch SemTeams.
SemTeams becomes a consumer of headless SemSource and owns its own application packaging. Consumers
that never selected the optional profile are unaffected.

### 3. Product semantics and reusable presentation have separate owners

- **SemSource:** product identity, source/project semantics, readiness, provenance, materialized views,
  OKF behavior, browser capability contracts, Compose profile, and acceptance tests.
- **`semstreams-ui`:** reusable Svelte shell, canonical graph model and renderer, layouts, generic
  search/filter/detail components, accessibility, product-profile seam, and released artifact.
- **SemTeams:** its product application and packaging as a consumer of headless SemSource.
- **SemStreams:** governed graph query and projection contracts.

A SemSource product profile may live in `semstreams-ui`, but its product behavior remains specified and
accepted by SemSource. SemSource does not copy Sigma, Graphology, layout workers, graph stores,
transforms, or adapters into this repository.

### 4. Composition is capability-driven

The workbench bootstraps from SemSource's versioned `GET /source-manifest/capabilities` document. It
renders only advertised queries, actions, readiness signals, and project-view availability. It does
not probe SemTeams, flow-builder, trajectory, or admin routes and interpret expected failures as
SemSource degradation.

Project and source status, readiness, inventory, and search lead the experience. The graph is an
evidence drill-down, not the default explanation of a project.

### 5. The optional profile consumes a pinned released artifact

The `ui` profile consumes an immutable `semstreams-ui` release artifact or image. Production use does
not require a sibling checkout, Node installation, or local frontend build. The release handoff records
the source commit, version, digest, and compatibility gates; a mutable `latest` tag is not an accepted
pin.

A SemSource-specific source override may remain for development and compatibility testing. It is not a
released product prerequisite.

### 6. The canonical graph remains governed by SemStreams

Fusion v1 can support project, search, list, and detail views. It does not provide the lossless,
directed, evidence-bearing graph projection required by the canonical explorer.

The missing framework contract is tracked in
[semstreams#533](https://github.com/C360Studio/semstreams/issues/533). SemSource does not infer
relationships from string shape, manufacture evidence, or build a parallel graph substrate.

The workbench may truthfully render fusion-backed slices while the capability document reports
`graph_projection` as unsupported. The graph-enabled final workbench release remains gated on adoption
and live validation of the governed projection contract. Omitting graph drill-down requires a formal
amendment to the active change.

## Cross-repo acceptance

This ADR records the SemSource side of the contract. Final acceptance requires:

1. SemSource owner and architect approval of this ADR and `add-opt-in-source-workbench`.
2. Shared UI owner approval of a linked `semstreams-ui` OpenSpec change.
3. The companion change naming audited source implementations, behavioral gates, repository owners,
   and the immutable artifact handoff.

Opening [semstreams-ui#2](https://github.com/C360Studio/semstreams-ui/issues/2) requests that acceptance;
it does not constitute shared-owner sign-off. This ADR remains Proposed until the linked change and
approval are recorded.

## Superseded and preserved decisions

On acceptance, this ADR supersedes only these decisions from the archived
`2026-07-15-add-ui-profile` design:

- D1, “SemSource owns the profile, SemTeams UI owns the app.”
- D2, “Default UI checkout is `../semteams/ui`.”

The archived change remains historical evidence of the integration proof. Its D3 same-origin routing
and D4 active-polling principles remain useful, but their targets move from SemTeams to the pinned
SemSource workbench. Headless-default behavior and SemSource-owned validation also remain compatible
with this decision.

## Consequences

### Enables

- A focused standalone SemSource workbench without coupling embedded consumers to a frontend.
- One reusable owner for the strongest graph, shell, accessibility, and test behaviors.
- A pinned artifact that can support a later one-action local installation.
- SemTeams integration through stable headless contracts instead of repository coupling.

### Costs and risks

- Existing `ui` profile users receive a breaking application change.
- Delivery depends on coordinated work and independent release ownership in `semstreams-ui`.
- Canonical graph drill-down cannot ship until the governed projection contract is proven.
- SemSource must maintain headless release evidence and product-level Playwright acceptance against the
  exact pinned artifact.

## Migration and rollback

The profile is not migrated until the shared UI change is accepted and its artifact passes the live
SemSource compatibility suite. The migration release publishes the old-to-new command mapping and the
SemTeams packaging handoff.

Rolling back to the previous SemSource release restores the former profile packaging. Rolling back the
new workbench pins an earlier independently released `semstreams-ui` digest. Neither path changes graph
state, and the headless core remains available throughout.

## Rejected alternatives

- **Keep SemTeams as the default application.** This preserves the integration proof but does not
  provide a standalone SemSource product surface.
- **Build a SemSource-local frontend or renderer.** This creates another divergent graph copy and
  assigns reusable presentation work to the wrong repository.
- **Make the UI mandatory.** This breaks embedded and agent-first consumers without improving backend
  correctness.
- **Add a second `workbench` profile.** This avoids naming the breaking change but leaves two competing
  optional UI contracts to support.
- **Ship an inferred graph from fusion v1.** This loses direction, predicates, evidence, and revision
  truth and violates the SemStreams ownership boundary.
- **Pin a mutable image tag.** This prevents reproducible compatibility evidence and safe rollback.
