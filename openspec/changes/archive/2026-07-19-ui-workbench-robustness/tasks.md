## 1. Crash-proofing (D1 + D3)

- [x] 1.1 Sources list keyed by full identity (`type:location:branch:languages`) in
      `WorkbenchShell.svelte`. Proves: vitest key-derivation pin + Playwright dup-branch
      sources render (two branches of one repo, no duplicate-key crash).
- [x] 1.2 `reset_required` accepted as an index state in `contracts/capabilities.ts`, rendered
      as a distinct not-ready presentation. Proves: capabilities unit test (parse + consistency
      rules) + Playwright reset_required page-alive spec.

## 2. Retention that retains (D2)

- [x] 2.1 `syncGraph` merge never downgrades: an unresolved incoming stub is skipped when the
      handle is already present. Proves: model unit test (partial projection preserves a
      previously-resolved node's facts; complete path byte-identical).

## 3. Readiness self-heals (D4)

- [x] 3.1 `+page.svelte` polls `bootstrap.refresh()` every 10s while any readiness surface is
      not ready; stops at ready; restarts on regression. Proves: unit test on the derive/timer
      seam + Playwright not-ready-panel-refreshes-without-reload spec.
- [x] 3.2 Overall banner derives from all advertised readiness signals and labels its coverage.
      Proves: banner unit test (Ready only when every signal ready; Building shown otherwise).

## 4. Error contract honored (D5)

- [x] 4.1 `search.ts` parses the fusion error envelope for any non-ok status when the contract
      is advertised; fallback to statusError on parse failure; 502 inherits envelope
      retryability. Proves: search error-mapping unit table (400/500/502/503/504/405/413).
- [x] 4.2 `GraphPanel` Retry re-submits the last errored query (labeled when it differs from
      the live input); cleared input can no longer silently dismiss an error. Proves:
      component/unit test on the retry path.

## 5. Drill-down presentation (D6)

- [x] 5.1 Entity list grouped: resolved first (queried symbol matched by name, auto-selected;
      fallback first resolved), unresolved endpoints grouped, marker-labeled, de-emphasized,
      never auto-selected. Proves: grouping/selection unit tests.
- [x] 5.2 Favicon added (static asset) — zero console errors on load. Proves: Playwright
      console-clean assertion in an existing spec.

## 6. Gates

- [x] 6.1 `ui` unit suite (vitest), lint, and svelte-check green; Playwright specs green via
      the container runner (`task ui:e2e`), evidence recorded in this change.

**Gate evidence (2026-07-19):** vitest 174/174 (21 files; new: drilldown, readiness,
readinessPoller; extended: model, capabilities, search, sourceManifest, WorkbenchShell,
GraphPanel); `npm run lint` clean; svelte-check 0 errors/0 warnings (477 files); prettier clean;
`npm run test:e2e` **20/20** (both viewports — 4 new specs: dup-branch sources no-crash,
reset_required page-alive, 10s not-ready refresh via page.clock, console-clean; the pre-existing
main spec's "Partial" assertion was pinned to the fixture's inconsistent `overall` field while
all three signals were ready — reconciled to assert "Ready" + the full coverage label, proving
the banner derives from the signals); `task ui:smoke:dev` real-stack smoke exit 0 (locally-built
workbench, isolated project/ports, clean teardown incl. nats-data volume).
