# Tasks — git submodule support

Fixture: `C360Studio/semdev-test` dual-pins `semdev-test-sub` — root @
`b1256521` (v2, has `Farewell`), `nested/semdev-test-sub` @ `b191a7bf` (v1,
no `Farewell`). Unit/integration tiers build hermetic local fixtures; the
remote fixture is for the e2e tier.

## 1. Workspace git plumbing

- [ ] 1.1 `workspace.ListSubmodules(ctx, path)` → `[]SubmoduleInfo{Path, URL,
      ResolvedURL, SHA, Materialized}`: parse `.gitmodules` via
      `git config -f`, gitlink SHAs via `git submodule status --recursive`,
      resolve relative URLs against the parent's remote; report
      declared-but-absent (stale `.gitmodules`) entries. Unit tests on
      hermetic temp-dir repos, including a relative-URL submodule.
- [ ] 1.2 Recursion depth cap (10) in `ListSubmodules`: beyond-cap paths are
      returned as a distinct category, never silently dropped; test with a
      deep synthetic nesting chain.
- [ ] 1.3 `EnsureRepo` submodule materialization behind an `Options` flag
      (default on): clone adds `--recurse-submodules --shallow-submodules`;
      pull runs `git submodule update --init --recursive` after the parent
      syncs; parent depth stays 1. Test that a moved gitlink resyncs the tree.
- [ ] 1.4 Shallow-fetch fallback: when a submodule update fails on a non-tip
      pinned SHA, retry that submodule with a full fetch. Hermetic test with
      a local bare remote pinned to a non-tip commit.

## 2. Walk-time git-boundary skip

- [ ] 2.1 ast/doc/cfgfile walkers skip any subdirectory containing a `.git`
      entry (file or directory) and count skipped boundaries in per-source
      status detail (shared helper if the walk layer allows).
- [ ] 2.2 Tests: a submodule dir and an unrelated nested git repo under a
      watch path produce zero parent-scoped entities; skip counts visible;
      empty (unmaterialized) submodule dirs produce nothing and no error.

## 3. Config surfaces

- [ ] 3.1 `submodules *bool` (default true) on the repo/git `SourceEntry` and
      git-source `Config` (schema tags, `Validate`, `DefaultConfig`); strict
      decoding still rejects unknown keys.
- [ ] 3.2 Optional `project` override on doc-source and cfgfile-source
      configs: explicit value replaces the path-derived system slug; absent
      keeps today's IDs byte-identical (regression-test both paths).

## 4. Probe and dynamic expansion in git-source

- [ ] 4.1 Post-`EnsureRepo` probe (initial seed and every poll cycle):
      `ListSubmodules` inventory onto git-source status detail with per-path
      state — materialized / unmaterialized / excluded_by_config /
      declared-but-absent / beyond-cap.
- [ ] 4.2 Spawn per-submodule ast/doc/cfg entries via `sourcespawn.Add`:
      canonical `project` = URLToSlug(resolved URL), `version` = 12-hex
      gitlink SHA prefix, org inherited; instance names parent-scoped
      (`…-via-<parentproject>`); doc/cfg entries carry the 3.2 override.
      Idempotent across probe re-runs.
- [ ] 4.3 Gitlink move = version transition: after tree sync, spawn
      new-version instances, then `sourcespawn.Remove` old-version instances;
      test ordering and that removal follows the source-lifecycle contract.
- [ ] 4.4 Loudness end-to-end: unmaterialized submodule paths appear on MCP
      `source_status` and HTTP `/source-manifest/status` (shared composition)
      naming the paths; signal clears within one aggregation pass once the
      tree materializes; opt-out shows `excluded_by_config`.

## 5. Identity assertions and ADR

- [ ] 5.1 ADR (one page): submodule identity — project from resolved URL
      slug, version from fixed 12-hex gitlink SHA prefix, org from parent
      source; why runtime expansion and parent-scoped instances.
- [ ] 5.2 ID tests: submodule symbol IDs carry submodule project + version
      scope and zero parent contribution; same (URL, SHA) from two parents →
      byte-identical IDs; two SHAs → distinct version scopes; all IDs valid
      6-part NATS-KV-safe via `entityid.*`.

## 6. Integration and e2e against the fixture

- [ ] 6.1 Integration test (hermetic local remotes, fixture-shaped: dual pin
      + nested path): clone → probe → expansion → ingestion; assert dedup,
      version distinction via the `Farewell` marker, parent exclusion, and
      probe status content.
- [ ] 6.2 e2e (`-tags=e2e`): remote clone of `C360Studio/semdev-test`; wait
      ready; assert root pin has `Farewell` under its version scope, nested
      pin does not, parent scope contains no submodule symbols; local
      plain-clone scenario surfaces uninitialized paths on status.
- [ ] 6.3 Runtime acceptance on a real stack (compose): seed semdev-test,
      verify graph queries return submodule entities with expected identity
      and status surfaces tell the submodule story; record evidence on #185.

## 7. Docs, verify, archive

- [ ] 7.1 Update user-facing docs: submodule behavior, `submodules` opt-out,
      identity/version semantics (feeds #184's multi-repo onboarding story);
      `add_source` tool description mentions submodule expansion.
- [ ] 7.2 Full local gate green (`gofmt`, `go vet`, revive, `go test -race
      -tags=integration ./...`), then `/opsx:verify`, sync deltas + archive
      on a branch, PR referencing #185.
