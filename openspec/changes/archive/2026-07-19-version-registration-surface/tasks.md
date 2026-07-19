## 1. Version on every surface (D1)

- [x] 1.1 `config.SourceEntry` gains `Version string`; `astComponentConfig` passes it to
      `watch_paths[].version`. Proves: `TestASTComponentConfig_Version` (set → present,
      unset → absent/empty, byte-identical config otherwise).
- [x] 1.2 `AddSourceInput` gains `version`, mapped in `sourceEntryFromAddInput`. Proves:
      mapping test case + tool description mentions it.
- [x] 1.3 Repo expansion propagates `Version` (single-branch, multi-branch,
      `BranchWatcherRef`) to the ast leaf. Proves: expansion propagation test.

## 2. Restart-stable non-semver ordering (D2)

- [x] 2.1 Natural version-string comparison replaces `indexedAt` in `candidateLess` and
      `versionComparable`; `indexedAt` leaves the ordering path. Proves:
      `TestCandidateLess_NaturalNonSemver` (v9<v10, r2023b<r2024a, equal-version
      incomparable, transitivity spot-checks) + `TestOrdering_RestartStable` (rewritten
      created-timestamps cannot flip direction).

## 3. Honest diff hydration (D3)

- [x] 3.1 `hydrateOne` distinguishes failure from absence; response gains additive
      per-symbol `body_error` + failure-count note. Proves: `TestVersionDiff_BodyErrorVisible`
      (failing resolver → body_error true, absent body → no flag).

## 4. Reachability proof (D4)

- [x] 4.1 `TestIntegration_VersionRegistrationToDiff` in `internal/governance/`: two fixture
      versions through `sourcespawn.Build` with explicit versions → real ast-source components
      → live graph stack → supersession run → `graph.query.versionDiff` answers the fixture's
      known counts + bodies.
- [x] 4.2 Docs: `version` documented on the config source entry (README source table or
      config reference) and in the `add_source` tool description; spec deltas re-validated
      (`openspec validate --all`).

**Evidence (2026-07-19):** design amended during apply — version alone cannot create
correspondence (supersession corresponds BY project, and path slugs differ per version
directory), so `Project` joined `Version` on every surface (SourceEntry, add_source, expansion),
with version-suffixed ast instance names for uniqueness; absent both, IDs and instance names are
byte-identical to today. `TestIntegration_VersionRegistrationToDiff` PASSES against the real
graph stack: SourceEntry{project: depA, version: 1.9.0/1.10.0} → sourcespawn.Build → real
ast-source components → `graph.query.versionDiff` answers added1/removed1/changed2/unchanged1
(the dep.go FILE entity legitimately counts as changed alongside Run) with hydrated
before/after bodies, and the supersession pass emits lineage edges. Non-semver ordering is now
a pure function of version strings (natural compare) — the old same-timestamp incomparability
and timestamp-order tests were pinned to the restart-unstable behavior and were updated to the
new contract. Body hydration failures are marked per-symbol (`from/to_body_error`) + counted
(`failed_bodies`), distinct from absence and budget skips.
