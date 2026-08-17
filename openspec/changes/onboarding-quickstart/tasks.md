# Tasks — onboarding quickstart

Fixtures: `C360Studio/semdev-test` (single track; known symbol `Classify`)
and `semdev-test` + local `semdev-test-sub` clone (multi track; dedup via the
shared pin). Design decisions D1–D6 govern mechanics.

## 1. Doc-block driver

- [ ] 1.1 Marker parser in `test/e2e`: extract fenced blocks whose info
      string carries `quickstart:<track>[,<track>]` from a markdown file, in
      document order, per track. Unit tests with golden cases (marked,
      unmarked, multi-track, malformed marker = loud failure).
- [ ] 1.2 Track runner: execute a track's blocks in order in a scratch
      workdir (env: harness NATS URL, PATH with the built binary), invoking a
      per-step assertion function by step index; failure names the step index
      and nearest doc heading. (D2's deliberate step-count coupling.)

## 2. Quickstart document — single-repo track

- [ ] 2.1 Write `docs/QUICKSTART.md` single track: prerequisites, get a repo,
      `semsource init --quick` (or documented non-interactive equivalent),
      start (Compose primary; native binary as the "without Docker" variant —
      D3's single substitution seam), watch readiness on
      `/source-manifest/status` (documented wait guidance stays qualitative),
      first `code_search` + `code_context`, fresh-storage upgrade note at the
      upgrade-relevant step.
- [ ] 2.2 Troubleshooting section, signal-keyed only (spec req 4): aggregate/
      per-source phase, index/embedding readiness objects, seed-liveness
      counters, submodule path states, backpressure — each entry = observable
      signal → meaning → action.
- [ ] 2.3 README: replace walkthrough fragments with the QUICKSTART link;
      keep the config reference role.

## 3. Single-track verification

- [ ] 3.1 e2e: drive the single track verbatim against a local clone of
      `semdev-test`; assert ready within the test timeout and that the
      documented first query returns `Classify` content; assert no step
      outside the document was needed (clean scratch env).
- [ ] 3.2 Point `core:smoke` at the quickstart's documented compose config so
      the compose start-step exercises the same artifact (D3).

## 4. Multi-repo track

- [ ] 4.1 Doc section: register two sources (remote `semdev-test` URL +
      local `semdev-test-sub` clone) with explicit `project` identity;
      per-source readiness; identity scoping explained (org vs `public.*`,
      `project`/`version`, monorepo/submodule pointer to ADR-0012 semantics).
      Resolve the design open question (CLI flags vs config example) against
      the real CLI; file an issue if a gap appears, don't grow this change.
- [ ] 4.2 Dedup demonstration in the doc: the standalone `semdev-test-sub`
      source and `semdev-test`'s submodule pin at the same SHA merge to one
      entity — show the query that proves it.
- [ ] 4.3 e2e: drive the multi track verbatim; assert per-source readiness,
      aggregate ready, a query answering from each source's scope, and the
      dedup assertion (one merged entity, not forks).

## 5. CI, decision, wrap

- [ ] 5.1 CI: quickstart tracks run in the e2e workflow; runtime bounded;
      docs-only edits don't block PRs on the network tier.
- [ ] 5.2 Record the D5 decision on #184 (one-action-local-start deferred
      past v1) once Coby ratifies the proposal.
- [ ] 5.3 Full local gate green, `/opsx:verify`, sync deltas + archive on the
      branch, PR referencing #184.
