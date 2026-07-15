## 1. Repair and accept SemSource-local ownership

- [x] 1.1 Replace the shared-UI ownership model in the proposal, design, specs, and integration notes
      with SemSource ownership of `ui/`, its test toolchain, Compose integration, and release image.
      - Test: architect review finds no cross-repo runtime, build, package, acceptance, or release
        dependency and confirms the preserved headless and SemStreams substrate boundaries.
- [x] 1.2 Revise ADR-0009 to record the accepted SemSource-owned optional workbench, the breaking `ui`
      flag takeover, and the archived `add-ui-profile` D1/D2 decisions it supersedes.
      - Test: ADR review confirms ownership, alternatives, consequences, rollback, and #533 scope match
        the repaired OpenSpec change.
- [x] 1.3 Correct and close
      [semstreams-ui#2](https://github.com/C360Studio/semstreams-ui/issues/2) as superseded, and mark the
      old UI ask as historical donor evidence rather than a delivery request.
      - Test: the issue records the corrected local-ownership decision and no active SemSource task
        depends on shared UI owner acceptance.

## 2. Lock the local best-of architecture

- [x] 2.1 Review the D4 inventory and verify every selected SemSpec, SemDragon, SemConnect, and
      SemStreams UI behavior names donor evidence, local destination, port rationale, and rejected
      behavior.
      - Evidence: review the pinned donor revisions and exact file paths in D4; for each row, trace the
        selected behavior to its SemSource-owned destination and trace the rejection to a local test or
        explicit graph-contract prohibition.
      - Test: architect and svelte-reviewer confirm that every row has all four fields, no donor is
        canonical, `graphTransform.ts` is rejection evidence only, and no unsafe transform is hidden
        behind copied code.
- [x] 2.2 Define the local module boundary for contracts, API clients, state, project/readiness/source
      components, search, graph-unavailable state, and later graph modules under `ui/src/lib/`.
      - Test: a dependency check finds no imports, source paths, containers, or packages from donor UI
        repositories.
- [x] 2.3 Specify the owned UI gates: format, lint, Svelte check, unit/component, accessibility, build,
      Playwright, and production image verification.
      - Evidence: D11 names the canonical SemSource-owned command for install and every gate, defines
        accessibility behavior beyond compile checks, and defines a clean production-image run with
        no host dependencies or development server.
      - Test: each command exists and passes using `ui/package-lock.json`; accessibility covers
        automated rules plus keyboard/focus behavior, and image verification builds without host
        `node_modules`, starts the production image without bind mounts, proves a non-zero runtime UID,
        waits for health, fetches SemSource shell content from the container port, and records the
        tested image ID and OCI content digest.
      - Rejected: SemTeams Node or Playwright dependencies, donor test fixtures, mutable `latest`, a
        cached image without rebuild evidence, bind-mounted build output, and Vite as production proof.

## 3. Define and implement SemSource-owned browser contracts

- [x] 3.1 Audit the live SemStreams graph query/gateway payload for explicit nodes, directed edges,
      property facts, evidence/provenance, and query revision before designing an adapter.
      - Test: architect contract report maps each required field to a live response and test, or records
        the missing framework-shaped field.
- [x] 3.2 Record the incomplete governed projection in `docs/upstream/semstreams-asks.md`, file
      [semstreams#533](https://github.com/C360Studio/semstreams/issues/533), and prohibit a parallel
      SemSource graph projection.
      - Test: `graph_projection` remains unsupported until the adopted contract is live-tested; this
        blocks graph drill-down acceptance but not the source/readiness/search MVP.
- [x] 3.3 Specify `GET /source-manifest/capabilities` as the versioned SemSource workbench response for
      product/project identity, authoritative readiness, query surfaces, actions, and project views.
      - Test: JSON examples cover ready, partial, unsupported, additive states, and the
        deployment-namespace project key.
- [x] 3.4 Write failing Go handler tests for capability discovery in headless operation, including
      partial readiness and unavailable optional actions.
      - Test: focused tests fail before implementation for every capability-contract scenario.
- [x] 3.5 Implement capability discovery through existing SemSource HTTP/component patterns.
      - Test: focused handler tests and `go test ./...` pass.
- [x] 3.6 Run go-reviewer assessment for context handling, errors, product boundaries, compatibility,
      and advertised-surface evidence.
      - Test: go-reviewer signs off with no unresolved blocking findings.

## 4. Build the SemSource-owned non-graph MVP with TDD

- [x] 4.1 Scaffold `ui/` as a Svelte 5 strict-TypeScript application with its own package lock,
      format/lint/check/unit/component/Playwright commands, lightweight local styling, and production
      Dockerfile.
      - Test: the first shell test fails before implementation; format, lint, check, unit, and build
        pass afterward without a sibling checkout.
- [x] 4.2 Add typed version-1 capability parsing and a classified HTTP client for ready, partial,
      unsupported, additive-unknown, incompatible-version, invalid-payload, and disconnected states.
      - Test: contract fixtures fail before implementation and cover every state without probing
        unadvertised routes.
- [x] 4.3 Implement the project-first shell with SemSource identity, project namespace, readiness,
      source inventory, project summary, and concrete unavailable cards for project views, OKF, and
      graph projection.
      - Test: component tests cover ready, partial, empty, degraded, unsupported, narrow-width, and
        disconnected rendering with accessible names and focus order.
- [x] 4.4 Implement fusion search/list/detail using only advertised capability URLs, preserving
      readiness, provenance, truncation, and classified 400/503/504 behavior.
      - Test: request, miss, empty, truncated, partial, and error fixtures pass; absent evidence stays
        unknown rather than being manufactured.
- [x] 4.5 Add local request-generation plus `AbortController` cancellation so stale search or bootstrap
      responses cannot replace newer state.
      - Test: controlled deferred-response tests prove cancellation and out-of-order suppression.
- [x] 4.6 Run svelte-reviewer assessment for Svelte 5 runes, contract fidelity, accessibility,
      responsive behavior, cancellation, and failure UX.
      - Test: no unresolved blocking findings remain before Compose integration.

## 5. Prepare canonical local graph behavior without bypassing #533

- [ ] 5.1 Define explicit local graph contract fixtures for nodes, directed relationships, property
      facts, evidence, and view revision, but do not adapt fusion v1 into this shape.
      - Test: fixtures cover opposite directions, parallel predicates, ID-like literals, unknown
        evidence, same-ID attribute updates, revision-only updates, and retraction/deletion.
- [ ] 5.2 Port the selected renderer/layout/accessibility behaviors into local modules only after the
      explicit fixtures fail, keeping the live graph capability disabled while #533 is open.
      - Test: unit/component tests cover deterministic placement, renderer initialization failure,
        worker cleanup/restart, attribute refresh, and synchronized keyboard/list/detail selection.
- [ ] 5.3 Enable the live graph adapter only after semstreams#533 is adopted and live-tested.
      - Test: a real SemSource compatibility fixture proves supplied direction, predicates, evidence,
        identity, retraction, and view revision without inference.

## 6. Make the breaking `ui` profile migration

- [x] 6.1 Document the old-to-new mapping: SemSource's `ui` flag changes from a sibling SemTeams
      checkout to the SemSource-owned workbench; SemTeams owns replacement packaging.
      - Test: architect and technical-writer confirm the changed command, unaffected headless path,
        owner handoff, and rollback.
- [x] 6.2 Replace the Compose `ui` service with the SemSource-owned production image and an explicit
      `./ui` development build; remove `UI_CONTEXT`, sibling mounts, and donor images.
      - Test: default Compose renders with an intentionally unavailable UI image because the omitted
        profile never resolves it; the `ui` profile references only SemSource-owned paths/artifacts.
- [x] 6.3 Keep Caddy limited to the workbench shell and advertised SemSource health, source-manifest,
      fusion, GraphQL, MCP, metrics, and raw graph routes.
      - Test: UI-profile Playwright exercises each advertised proxy route through Caddy, rejects
        fallthrough to misleading UI HTML, and finds no stale SemTeams, flow-builder, trajectory, or
        unshipped OKF/project-view routes.
- [x] 6.4 Update `task ui:e2e` and `task ui:smoke` to use the owned UI Playwright dependency and
      production image/development build, never SemTeams tooling.
      - Test: preflight and execution succeed with no sibling checkout and include final HTTP/UI state
        on forced failures.

## 7. Prove headless and workbench behavior

- [x] 7.1 Add a headless smoke assertion that no UI image, source build, Node process, proxy, or UI
      registry credential is required while HTTP, MCP, readiness, and graph query remain available.
      - Test: core smoke passes with an intentionally unreachable UI image and no local Node toolchain.
- [x] 7.2 Add UI-profile Playwright coverage through Caddy against real SemSource for shell branding,
      capability bootstrap, readiness, source inventory, search, keyboard result/detail selection,
      graph-unavailable state, and every advertised proxied route.
      - Test: tests use visible accessible UI, not canvas-only hooks, prove backend routes do not fall
        through to UI HTML, and pass at desktop and narrow widths.
- [ ] 7.3 Add production-image evidence tying the tested image to its SemSource commit, version, and
      immutable digest.
      - Test: the digest tested by SemSource matches the profile pin; mutable `latest` is not accepted.

## 8. Documentation and release gates

- [x] 8.1 Update operator docs with default headless mode, optional SemSource `ui` mode, SemTeams
      consumer handoff, local development, rollback, and proposed follow-on changes.
      - Test: technical-writer confirms commands, ownership, optionality, and non-goals match the
        implementation; former UI integration notes are clearly historical.
- [x] 8.2 Update advertised-surface coverage only after the repurposed profile and routes are proven.
      - Test: every newly advertised command and route maps to an automated assertion.
- [x] 8.3 Run SemSource gates: strict OpenSpec validation, `task lint`, `go vet ./...`,
      `go test ./...`, core e2e, owned UI format/lint/check/unit/build, workbench Playwright, headless
      smoke, and production-image smoke.
      - Test: all commands pass with no revive warnings or unresolved reviewer findings.
