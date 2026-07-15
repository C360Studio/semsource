## 1. Accept and record the cross-repo contract

- [ ] 1.1 Open and obtain both-owner acceptance of a coordinated `semstreams-ui` OpenSpec change for
      the best-of inventory, canonical graph surface, SemSource product profile, and released artifact.
      - Test: the linked change names the audited source implementations, repository owners, behavioral
        gates, and release handoff with architect sign-off from SemSource and shared UI ownership.
- [ ] 1.2 Record the accepted headless-default/workbench ownership split in ADR-0009, including the
      archived `add-ui-profile` D1 and D2 decisions it supersedes and the breaking `ui` flag takeover.
      - Test: ADR review confirms the decision, alternatives, consequences, and cross-repo owners match
        both accepted OpenSpec changes.

## 2. Gate the companion canonical UI change

- [ ] 2.1 Review the companion inventory and verify that `semstreams-ui` is the canonical destination,
      with the selected SemSpec, SemDragon, SemConnect, and SemStreams UI behaviors represented.
      - Test: the accepted inventory maps every D4 concern to behavioral acceptance tests and records
        rejected behavior, including SemSpec's multiword-NLQ heuristic.
- [ ] 2.2 Verify the companion contract covers explicit directed multi-predicate relationships,
      property facts, real evidence fields, revision synchronization, and no string-shape inference.
      - Test: shared contract tests cover opposite directions, parallel predicates, literal ID-like
        values, unknown evidence, same-ID attribute updates, and retraction/deletion.
- [ ] 2.3 Verify the companion behavioral suite covers renderer initialization failure, deterministic
      seeded layout, ForceAtlas worker cleanup/restart, stale search cancellation, partial loading,
      disconnected state, responsive evidence access, and product-profile isolation.
      - Test: each behavior has a named regression test; tests assert behavior rather than source-file
        lineage.
- [ ] 2.4 Verify the SemSource profile imports the canonical graph model, renderer, adapter, and layout
      rather than defining profile-local Sigma, Graphology, transform, or worker copies.
      - Test: a static dependency check and svelte-reviewer inspection find no duplicate graph stack in
        the SemSource profile.

## 3. Define and implement SemSource-owned browser contracts

- [x] 3.1 Audit the live SemStreams graph query/gateway payload for explicit nodes, directed edges,
      property facts, evidence/provenance, and query revision before designing an adapter.
      - Test: architect contract report maps each required field to a live response and test, or records
        the missing framework-shaped field.
- [x] 3.2 If the governed query contract is incomplete, record the gap in
      `docs/upstream/semstreams-asks.md`, file the SemStreams issue, and block workbench release rather
      than implementing a parallel SemSource graph projection.
      - Test: each missing field has an upstream issue link and the SemSource release task depends on
        its adopted and live-tested governed query contract. Any decision to omit graph drill-down
        requires a formal amendment to this change and its `source-workbench` specification.
- [ ] 3.3 Specify the versioned SemSource workbench capability response for product/project identity,
      authoritative readiness, query surfaces, supported/unavailable actions, and project-view
      availability without importing UI types into the Go backend.
      - Test: architect review approves JSON examples for ready, partially ready, unsupported, and
        backward-compatible additive states.
- [ ] 3.4 Write failing Go handler tests for the capability response in headless operation, including
      partial readiness and unavailable optional actions.
      - Test: focused tests fail before implementation for every capability-contract scenario.
- [ ] 3.5 Implement the capability response through existing SemSource HTTP/component patterns and wrap
      I/O errors with operation context.
      - Test: focused handler tests and `go test ./...` pass.
- [ ] 3.6 Run go-reviewer assessment for context handling, errors, product boundaries, compatibility,
      and advertised-surface evidence.
      - Test: go-reviewer signs off with no unresolved blocking findings.

## 4. Accept the final shared workbench artifact

- [ ] 4.1 Verify the companion SemSource profile composes project/source status, readiness, inventory,
      search, and graph drill-down from the capability response without probing unadvertised routes.
      - Test: request-contract tests cover ready, partial, unsupported, empty, disconnected, and
        query-not-ready states against SemSource-compatible fixtures.
- [ ] 4.2 Verify keyboard and screen-reader navigation exposes roles/names, focus and selected state,
      relationship source/predicate/target semantics, evidence labels, and synchronized detail
      announcements at desktop and narrow widths.
      - Test: component and Playwright accessibility assertions pass without using canvas-only test
        hooks.
- [ ] 4.3 Run svelte-reviewer assessment for Svelte 5 runes, evidence fidelity, directed semantics,
      attribute refresh, accessibility, responsive behavior, and failure UX.
      - Test: svelte-reviewer signs off with no unresolved blocking findings.
- [ ] 4.4 Publish the final versioned `semstreams-ui` artifact only after the SemSource profile and all
      canonical graph gates pass; record its immutable version and digest.
      - Test: artifact build is reproducible, reports its digest, and starts without a source checkout.

## 5. Make the breaking `ui` profile migration

- [ ] 5.1 Document the old-to-new mapping before implementation: SemSource's `ui` flag changes from a
      sibling SemTeams checkout to the SemSource workbench, and SemTeams owns its replacement packaging.
      - Test: architect and technical-writer review confirm the migration, affected command, unaffected
        headless consumers, rollback, and cross-repo handoff are explicit.
- [ ] 5.2 Replace the existing Compose `ui` service target with the pinned accepted `semstreams-ui`
      artifact; do not add a separate `workbench` service or profile.
      - Test: rendered config assigns the SemSource workbench only to `ui`; default startup succeeds
        with an intentionally unavailable UI image override and never resolves or pulls it; HTTP, MCP,
        readiness, and graph query remain available.
- [ ] 5.3 Extend the same-origin proxy only for the workbench shell and existing SemSource health,
      source-manifest, search, GraphQL, MCP, metrics, and graph routes.
      - Test: proxy contract tests exercise every advertised existing route, include no unshipped OKF
        or project-view route, reject fallthrough to misleading UI HTML, and include no stale
        SemTeams-specific route.
- [ ] 5.4 Replace or rename `UI_CONTEXT` with an explicit SemSource development-only source override
      without making it a released-workbench prerequisite.
      - Test: the pinned profile starts with no sibling checkout; an explicit SemSource override
        renders the documented development context, and the former SemTeams default is absent.
- [ ] 5.5 Record the release-note and SemTeams handoff item without editing the SemTeams repository.
      - Test: the handoff names the changed command, headless integration path, owning team, and
        previous-release rollback option, and links the SemTeams team's acknowledgment.

## 6. Prove headless and workbench behavior

- [ ] 6.1 Add a headless smoke assertion that no workbench image, checkout, Node process, proxy, or UI
      registry credential is required while HTTP, MCP, readiness, and graph query remain available.
      - Test: the core smoke passes with no UI checkout/toolchain and an unreachable workbench image.
- [ ] 6.2 Update `task ui:smoke` in place to use the pinned artifact and cover authoritative readiness,
      source inventory, search/query, keyboard entity selection, graph/detail synchronization, and
      concrete failures.
      - Test: the smoke passes against real SemSource and reports the final HTTP/UI state on a forced
        readiness failure.
- [ ] 6.3 Add a live attribute-refresh regression proving changed evidence is visible when entity and
      relationship IDs remain stable.
      - Test: Playwright observes the new provenance/freshness value without a full page reload.

## 7. Documentation and release gates

- [ ] 7.1 Update operator docs with the default headless mode, optional SemSource `ui` mode, SemTeams
      external-consumer handoff, and proposed not-yet-approved follow-ons: `materialize-project-views`,
      `add-okf-interop-mvp`, and `add-one-action-local-start`.
      - Test: technical-writer review confirms commands, ownership, optionality, dependency order, and
        non-goals match the implemented profile, and `semteams-ui-profile-feedback.md` is marked as
        historical evidence.
- [ ] 7.2 Update advertised-surface coverage only after the repurposed `ui` profile and routes are
      proven.
      - Test: every newly advertised command and route maps to a concrete automated assertion.
- [ ] 7.3 Run SemSource gates: `openspec validate add-opt-in-source-workbench --strict`, `task lint`,
      `go vet ./...`, `go test ./...`, the core e2e suite, and the workbench smoke.
      - Test: all commands pass with no revive warnings.
- [ ] 7.4 Run the companion UI format, lint, Svelte check, unit/component, build, accessibility, and live
      SemSource Playwright gates against the exact pinned artifact.
      - Test: the release digest tested by SemSource matches the artifact recorded in the workbench
        profile.
