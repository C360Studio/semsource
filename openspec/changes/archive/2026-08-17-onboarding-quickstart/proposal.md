# Easy-button onboarding: the quickstart that proves itself

Tracks [#184](https://github.com/C360Studio/semsource/issues/184).

## Why

A new user with Docker and a repo cannot currently reach `phase: ready` plus a
correct first query from the docs alone — the README is a config reference,
the wizard produces working configs but nothing walks the zero-to-first-query
path, and nothing proves that path continuously. `task core:smoke` and the OSH
scorecard prove boot→ready→query on paths *users never follow*. This is a v1
blocker (#184): the multi-repo half was deliberately sequenced after #185,
whose submodule identity/config semantics (now shipped in beta.8) are exactly
what the multi-repo story must describe.

## What Changes

- **A single quickstart document** (`docs/QUICKSTART.md`, linked prominently
  from the README): init → start → watch readiness → first
  `code_search`/`code_context`, with the fresh-storage upgrade note and a
  troubleshooting section keyed to signals that actually exist (aggregate
  phase, index/embedding readiness, `files_parsed`/`bodies_offloaded` seed
  liveness, per-path submodule states, backpressure).
- **A multi-repo section**: several sources with explicit `project`/`version`
  identity, remote + local mix, per-source readiness, what identity scoping
  means for cross-repo queries (org sovereignty vs `public.*`), and the
  monorepo/submodule case (pointer to beta.8 semantics: canonical submodule
  identity, dedup by pin).
- **A doc-driven e2e test**: the quickstart's command blocks carry machine
  markers; the test extracts and executes them **verbatim, in order** against
  fixtures (single-repo and two-repo), asserting readiness and first-query
  correctness between steps. Doc drift fails CI instead of a user.
- **Decision (gap 4)**: `add-one-action-local-start` is **deferred past v1**,
  not created — the quickstart + `semsource init` is v1's easy button; a
  bespoke launcher adds surface area without moving the docs-alone bar.
  Recorded on #184 on acceptance of this proposal.

## Capabilities

### New Capabilities

- `onboarding-quickstart`: the documented zero-to-first-query contract and its
  continuous verification — the quickstart reaches ready + correct first query
  on the documented commands alone; those commands are executed verbatim by
  CI; multi-repo identity semantics are part of the documented surface;
  troubleshooting is signal-keyed.

### Modified Capabilities

_None._ The quickstart documents existing behavior; `cli-onboarding`,
`compose-deployment`, and `ingestion-readiness` requirements are unchanged.

## Impact

- **Docs**: new `docs/QUICKSTART.md`; README gains the link and sheds any
  duplicated walkthrough content.
- **Tests**: `test/e2e` gains the doc-block driver (marker parser + runner)
  and two quickstart tracks; the parser itself is unit-tested.
- **Fixtures**: reuses the public beta.8 fixtures — `C360Studio/semdev-test`
  (single-repo track) and `semdev-test` + `semdev-test-sub` as two sources
  (multi-repo track; also demonstrates cross-repo dedup, since the standalone
  repo and the submodule pin at the same SHA merge to identical entity IDs).
  No new fixture repos required.
- **CI**: the e2e workflow runs the doc-driver tracks; runtime stays bounded
  (fixtures are tiny).
- **Consumers**: SemTeams/SemSpec/SemDragon operators get the canonical
  "connect an assistant to a repo" path their own docs can link instead of
  restating.

## Non-goals

- No one-action launcher binary/script (deferred past v1 — see Decision).
- No new CLI surface, config fields, or readiness signals — the quickstart
  documents what beta.8 already ships; gaps it exposes are filed, not patched
  in here.
- No TLS/reverse-proxy hardening (same-LAN posture unchanged; the doc says
  so).
- No substrate work: query correctness assertions use existing MCP/HTTP
  surfaces only.
