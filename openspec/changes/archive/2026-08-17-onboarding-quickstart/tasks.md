# Tasks — onboarding quickstart

Fixtures: `C360Studio/semdev-test` (single track; known symbol `Classify`)
and `semdev-test` + local `semdev-test-sub` clone (multi track; dedup via the
shared pin). Design decisions D1–D6 govern mechanics.

## 1. Doc-block driver

- [x] 1.1 Marker parser in `test/e2e`: extract fenced blocks whose info
      string carries `quickstart:<track>[,<track>]` from a markdown file, in
      document order, per track. Unit tests with golden cases (marked,
      unmarked, multi-track, malformed marker = loud failure).
      (`test/e2e/quickstart_doc.go` — untagged so the grammar tests run in
      the ordinary unit gate.)
- [x] 1.2 Track runner: execute a track's blocks in order in a scratch
      workdir (env: harness NATS URL, PATH with the built binary), invoking a
      per-step assertion function by step index; failure names the step index
      and nearest doc heading. (D2's deliberate step-count coupling.)
      (`test/e2e/quickstart_runner.go`; persistent-cwd wrapper emulates one
      user shell across blocks; background steps get their own process
      group.)

## 2. Quickstart document — single-repo track

- [x] 2.1 Write `docs/QUICKSTART.md` single track: prerequisites, get a repo,
      `semsource init --quick` (or documented non-interactive equivalent),
      start (Compose primary; native binary as the "without Docker" variant —
      D3's single substitution seam), watch readiness on
      `/source-manifest/status` (documented wait guidance stays qualitative),
      first `code_search` + `code_context`, fresh-storage upgrade note at the
      upgrade-relevant step.
- [x] 2.2 Troubleshooting section, signal-keyed only (spec req 4): aggregate/
      per-source phase, index/embedding readiness objects, seed-liveness
      counters, submodule path states, backpressure — each entry = observable
      signal → meaning → action.
- [x] 2.3 README: replace walkthrough fragments with the QUICKSTART link;
      keep the config reference role.

## 3. Single-track verification

- [x] 3.1 e2e: drive the single track verbatim against a local clone of
      `semdev-test`; assert ready within the test timeout and that the
      documented first query returns `Classify` content; assert no step
      outside the document was needed (clean scratch env).
      (`TestE2E_QuickstartSingleTrack` — green locally in ~24s; caught one
      real doc drift before passing: namespace derives from the git remote
      owner, not the directory name.)
- [x] 3.2 Point `core:smoke` at the quickstart's documented compose config so
      the compose start-step exercises the same artifact (D3). (The smoke
      already boots the documented default `configs/mvp.json`; a preflight
      guard now fails the smoke if the doc stops naming that config or
      `SEMSOURCE_TARGET`.)

## 4. Multi-repo track

- [x] 4.1 Doc section: register two sources (remote `semdev-test` URL +
      local `semdev-test-sub` clone) with explicit `project` identity;
      per-source readiness; identity scoping explained (org vs `public.*`,
      `project`/`version`, monorepo/submodule pointer to ADR-0012 semantics).
      Resolve the design open question (CLI flags vs config example) against
      the real CLI; file an issue if a gap appears, don't grow this change.
- [x] 4.2 Dedup demonstration in the doc: the standalone `semdev-test-sub`
      source and `semdev-test`'s submodule pin at the same SHA merge to one
      entity — show the query that proves it.
- [x] 4.3 e2e: drive the multi track verbatim; assert per-source readiness,
      aggregate ready, a query answering from each source's scope, and the
      dedup assertion (one merged entity, not forks).
      (`TestE2E_QuickstartMultiTrack` — green locally in ~14s; the dedup
      response shows exactly one merged Farewell handle at the canonical
      submodule scope + pin.)

## 5. CI, decision, wrap

- [x] 5.1 CI: quickstart tracks run in the e2e workflow; runtime bounded;
      docs-only edits don't block PRs on the network tier.
      (`.github/workflows/quickstart.yml` — new bounded job running only the
      two quickstart tracks (both <30s locally; 30-min cap); PR trigger is
      path-filtered so unrelated docs-only edits skip the network tier; the
      marker-grammar unit tests already run in the ordinary `test` job. The
      general e2e suite stays deliberately un-promoted. Gaps found while
      writing the doc filed as #188 (backpressure dropped by the status
      aggregator) and #189 (no add --project/--version flags).)
- [x] 5.2 Record the D5 decision on #184 (one-action-local-start deferred
      past v1) once Coby ratifies the proposal. (Ratified 2026-08-17 —
      "Docker Compose is the v1 launcher"; recorded in
      [#184 comment](https://github.com/C360Studio/semsource/issues/184#issuecomment-5318346799).)
- [x] 5.3 Full local gate green, `/opsx:verify`, sync deltas + archive on the
      branch, PR referencing #184. (Gate green 2026-08-17: gofmt/vet/revive,
      `go test -race ./...`, the three CI integration packages, and both
      quickstart tracks under the exact CI invocation (38s). Verify: no
      criticals, no warnings — one attribution assert strengthened and the
      spec's backpressure mention corrected to reality during the pass.)
