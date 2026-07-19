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
      components, search, capability-gated graph state, and graph modules under `ui/src/lib/`.
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
      - Test: `graph_projection` remained unsupported until the adopted contract was live-tested;
        this blocked graph drill-down acceptance but not the source/readiness/search MVP.
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

## 5. Adopt the canonical governed graph projection

- [x] 5.1 Define explicit local graph contract fixtures for nodes, directed relationships, typed
      property facts, verbatim optional evidence, opaque handles, explicit unresolved endpoints,
      meaningful view revision, and graph-local truncation. Do not adapt fusion v1 role maps or
      ID-shaped literals into this shape.
      - Test: `contracts/graph.test.ts` and the owned graph fixture cover opposite directions,
        parallel predicates, ID-like literals, absent evidence, per-fact/per-edge truncation,
        malformed revisions, and explicit edge endpoints whose node details were not returned.
      - Test: `graph/model.test.ts` covers same-handle attribute updates, revision-only updates, and
        deletion only for a complete projection with a coherent equal nonzero revision; truncated,
        incoherent, and zero-revision responses retain previously known nodes and edges.
- [x] 5.2 Implement the specified local WebGL/Sigma renderer, deterministic worker layout, failure
      handling, and synchronized accessible navigator against the explicit fixtures.
      - Test: renderer/layout and `GraphPanel` tests cover deterministic placement, SSR-safe renderer
        initialization failure, worker and Sigma cleanup/restart, same-handle refresh,
        partial-response retention, distinguishable unresolved endpoints, visible truncation detail,
        and synchronized keyboard/list/detail selection.
- [x] 5.3 Adopt semstreams#533 from SemStreams `v1.0.0-beta.153` through the existing
      `POST /code-context/context` contract with `want: ["graph"]`; do not add another endpoint,
      synthesize a projection, or make GraphQL part of this slice.
      - Test: `TestHTTPGraphProjectionCompatibility` runs the real beta.153 fusion engine and proves
        typed property facts, verbatim optional evidence, explicit source/predicate/target direction,
        parallel and opposite-direction edges, an ID-like literal that remains a fact, and a
        meaningful coherent nonzero view revision.
      - Test: `queryGraph` and `GraphPanel` tests prove the advertised route/request, strict response
        parsing, opaque-handle behavior, explicit unresolved endpoint rendering, independent graph
        truncation, revision-regression rejection, fusion HTTP error-envelope classification, valid
        no-graph miss/not-ready responses, and non-deletion on truncated, incoherent, or
        unavailable-revision responses.

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
      capability-gated graph state, and every advertised proxied route.
      - Test: tests use visible accessible UI, not canvas-only hooks, prove backend routes do not fall
        through to UI HTML, exercise the real ready graph route plus no-graph states, pass automated
        accessibility and keyboard assertions, and pass at desktop and narrow widths.
- [x] 7.3 Publish and capture the first production-image evidence tying the tested image to its
      SemSource commit, version, immutable manifest digest, and successful trusted CI run.
      - [x] 7.3.1 Add PR-safe UI/browser/clean-image gates and trusted main/tag multi-platform
        publication to `ghcr.io/c360studio/semsource-ui`, with deterministic tags, explicit OCI
        version/full-revision labels, scoped package permissions, and publish-job outputs.
        - Test: `task ui:image:release:test` proves event/permission boundaries, main
          `latest` plus `sha-<full-revision>`, release `v<semver>` plus plain `<semver>`, label inputs,
          job outputs, and the separation of publish from verification without registry access.
      - [x] 7.3.2 Add independent exact-manifest and released-profile verification.
        - Test: verifier contract tests cover tag-to-manifest equality, required platforms, OCI
          version/revision, local `RepoDigest`, rejection of digest-qualified `latest`, exact Compose
          and running-container pins, success evidence/run URL, and retained failure diagnostics.
      - [x] 7.3.3 Record the first real trusted-run evidence.
        - Evidence: trusted `main` UI publish/smoke jobs for revision
          `25b2816d14a147c1d6eb7b54e40668b51ba3574a` published and passed against
          `ghcr.io/c360studio/semsource-ui:sha-25b2816d14a147c1d6eb7b54e40668b51ba3574a@sha256:43edacf62e7908681e7bedd193d1b18f3ebe8f3de438d417c6c091517020ea20`.
        - Test: [Actions run 29693062800, attempt 1](https://github.com/C360Studio/semsource/actions/runs/29693062800)
          verified `linux/amd64` and `linux/arm64`, OCI version/full revision, local `RepoDigest`, the
          exact Compose-rendered and running-container pins, and released-profile browser smoke 6/6.
          [Evidence artifact 8444245976](https://github.com/C360Studio/semsource/actions/runs/29693062800/artifacts/8444245976)
          records the successful release-smoke evidence. All six workflow jobs completed green,
          including `build-and-push` and `ui-release-smoke`.

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
