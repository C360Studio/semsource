# Tasks — git submodule support

Fixture: `C360Studio/semdev-test` dual-pins `semdev-test-sub` — root @
`b1256521` (v2, has `Farewell`), `nested/semdev-test-sub` @ `b191a7bf` (v1,
no `Farewell`). Unit/integration tiers build hermetic local fixtures; the
remote fixture is for the e2e tier.

## 1. Workspace git plumbing

- [x] 1.1 `workspace.ListSubmodules(ctx, path)` → `[]SubmoduleInfo{Path, URL,
      ResolvedURL, SHA, Materialized}`: parse `.gitmodules` via
      `git config -f`, gitlink SHAs via `git ls-files -s` (reads the PINNED
      gitlink from the index — `submodule status` reports checked-out HEAD,
      which can differ, and needs no init state), resolve relative URLs
      against the parent's remote; report declared-but-absent (stale
      `.gitmodules`) entries. Unit tests on hermetic temp-dir repos,
      including a relative-URL submodule.
- [x] 1.2 Recursion depth cap (10) in `ListSubmodules`: beyond-cap paths are
      returned as a distinct category, never silently dropped; tested via a
      capped-inventory test hook (`export_test.go`) on a 2-level chain.
- [x] 1.3 `EnsureRepo` submodule materialization behind an `Options` flag
      (default on, `SkipSubmodules` opt-out): both clone and pull paths run
      `git submodule update --init --recursive --depth 1` as a separate step
      (not clone flags — one shared path, tractable fallback); parent depth
      stays 1. Moved-gitlink resync tested. Auth extraheader is now
      HOST-SCOPED (recursion would otherwise send the parent token to every
      submodule host).
- [x] 1.4 Shallow-fetch fallback: on any shallow submodule-update failure,
      retry the whole update without `--depth` (materialized trees are
      no-ops). Hermetic test over file:// (which honors --depth) with a
      non-tip pin.

## 2. Walk-time git-boundary skip

- [x] 2.1 ast/doc/cfgfile walkers skip any subdirectory containing a `.git`
      entry (file or directory) via shared `internal/gitboundary`; applied at
      12 walk sites (ast-source parseDirectory, both fs watchers incl.
      runtime new-dir + flush-time `Under` guard for the materialize-while-
      watching race, doc/cfgfile/ast handlers, 5 language parsers, C call
      walks). ast-source counts skips (`boundaries_skipped` on its status
      report); doc/cfg handlers skip without a counter (no per-handler status
      channel — manifest surfacing lands with group 4).
- [x] 2.2 Tests: gitboundary unit tests + boundary tests at cfgfile, doc,
      and golang-parser walk sites (submodule gitlink + foreign nested repo
      + empty submodule dir → zero parent-scoped entities, no error). Full
      unit suite green.

## 3. Config surfaces

- [x] 3.1 `submodules *bool` (default nil=on) on the repo/git `SourceEntry`
      and git-source `Config` (schema default:true); expandSingleBranch
      propagates it to the git child; sourcespawn passes it through; git
      handler gains `SkipSubmodules` wired to `workspace.Options`. Decode
      test pins nil-vs-explicit-false.
- [x] 3.2 Optional `project` override on doc-source and cfgfile-source
      configs and handlers (`WithProject` / `Config.Project`): explicit value
      replaces the path-derived system slug (doc lifecycle trigger scopes by
      the same slug); absent keeps today's IDs byte-identical (pinned by the
      whole existing suite, which never sets it). sourcespawn doc/cfg
      builders use `src.Project` for slug + pass `project` through.
      Override tests at both handlers.

## 4. Probe and dynamic expansion in git-source

- [x] 4.1 Post-resolution probe in the git handler (initial seed and every
      poll): inventory classified onto git-source's status report with
      per-path state — materialized / unmaterialized / excluded_by_config /
      declared_but_absent / beyond_cap. Classification unit-tested; probe
      pinned by an integration test.
- [x] 4.2 Expansion via `internal/subwatch` at the composition root
      (branch-watcher precedent — components lack ConfigManager access):
      per-submodule ast/docs/cfg entries with canonical `project` =
      SystemSlug(URLToSlug(resolved URL)) and `version` = 12-hex gitlink
      prefix; doc/cfg ride `project-<sha12>` (keeps two pins' doc/config
      identity distinct); instance names parent+LINK scoped
      (`-via-<parent>-<path>` — same-SHA dual links in one parent must not
      share instances). Idempotent across ticks; dual-pin fixture-shaped
      test. Runtime-added repos: no expansion watcher yet (loud on status,
      never misattributed) — recorded in design D1.
- [x] 4.3 Gitlink move = version transition: spawn-new-then-remove-old via
      KV config delete (ServiceManager tears down reactively); ordering and
      old-instance removal tested.
- [x] 4.4 Loudness end-to-end: the `submodules` field flows git-source →
      SourceStatusReport → SourceStatus → StatusPayload; HTTP
      `/source-manifest/status` serves that payload verbatim and MCP
      `source_status` embeds the same raw JSON — one composition, no
      re-mapping to drop it. Probe refresh per poll clears the signal after
      materialization.

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
