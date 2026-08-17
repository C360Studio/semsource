# Design — onboarding quickstart

See `proposal.md` for motivation and `specs/onboarding-quickstart/spec.md`
for the behavior contract.

## Context

- `test/e2e` already builds the binary, runs NATS via Docker, drives
  subprocesses, and asserts on HTTP status + NATS entities — everything a
  doc-driver needs exists there (`e2e_test.go` harness; the beta.8 submodule
  e2e is the freshest model).
- `task core:smoke` boots the compose core profile and polls status; it
  proves compose wiring but not the documented user path.
- Public fixtures exist and are already load-bearing in CI:
  `C360Studio/semdev-test` (dual-pins `semdev-test-sub`) and
  `C360Studio/semdev-test-sub` — tiny, stable, append-only history.
- beta.8 shipped the signals the troubleshooting section needs: per-source
  phases, index/embedding readiness objects, seed-liveness counters,
  submodule path states, backpressure flag.

## Goals / Non-Goals

**Goals:**

- One source of truth for commands: the document. The test executes it; it
  never restates it.
- Marker grammar small enough to be obviously correct, big enough for two
  tracks.
- CI runtime bounded: fixtures are tiny; tracks share the harness.

**Non-Goals:**

- No testing of the *interactive* wizard path in the doc-driver (the
  quickstart documents the non-interactive commands; `cli-onboarding`'s spec
  already covers wizard behavior).
- No compose-in-CI build pipeline beyond what exists (see D3).

## Decisions

### D1. Marker grammar: fence info-string tokens

Marked blocks use the fence info string:

    ```bash quickstart:single
    semsource init --quick
    ```

Grammar: `quickstart:<track>` where `<track>` ∈ {`single`, `multi`}. Blocks
execute in document order within their track; a block may carry both tracks
(`quickstart:single,multi`). Unmarked blocks are prose examples and never
execute. The parser is ~40 lines over the raw markdown (fence scan, no
markdown library) and unit-tested with golden cases.

*Why info-string, not HTML comments?* The marker travels WITH the fence it
governs — a comment above a fence can drift apart from it in edits; an
info-string cannot. GitHub renders the block normally either way.

### D2. Assertions interleave by step index, not by markers in the doc

The driver executes track blocks in order; the TEST holds an assertion
function per step index (after step N, assert X — e.g. after the "start"
step, poll status to ready with the documented wait guidance; after the
"first query" step, assert the response contains the expected symbol). The
document stays purely user-facing. If the doc gains or loses a block, the
step-count assertion fails loudly, forcing the test to be looked at — that
coupling is deliberate: a new documented step SHOULD make someone decide
what must be true after it.

### D3. The doc's primary track runs the native binary; compose stays smoke-covered

The quickstart documents both starts: **Docker Compose** (primary for users)
and the **native binary** (secondary, "without Docker"). The doc-driver
executes the NATIVE track verbatim in `-tags=e2e` (fast: binary already
built by the harness, NATS already containerized). The compose start-step is
covered by the existing `core:smoke` (which this change points at the
quickstart's config rather than a synthetic one, so the compose path
exercises the same documented artifact). Full compose-driven doc execution
per PR is deliberately not attempted: image build time per PR buys little
beyond what core:smoke + the native track already prove, and the compose
YAML surface is pinned by its own smoke.

The substitution seam is exactly one step (how the engine starts); every
other command (init/add/config/status/query) is byte-identical between
tracks and executes verbatim.

### D4. Fixtures: the beta.8 pair, no new repos

- Single track: clone `semdev-test` (the "your repo" stand-in), init inside
  it, ready, query for a known symbol (`Classify` from its health package).
- Multi track: register `semdev-test` (remote URL) + a local clone of
  `semdev-test-sub` (the remote+local mix the issue asks for), each with
  explicit `project`; ready; query content from each scope; then the dedup
  demonstration — `semdev-test`'s submodule pin of `semdev-test-sub` at the
  same SHA merges with the standalone source's entities (ADR-0012 made this
  true by construction; the doc gets to SHOW it).

### D5. `add-one-action-local-start` is deferred past v1

The v1 bar is docs-alone-to-ready, and the quickstart + `semsource init
--quick` meets it in two commands. A launcher binary/script would add a
maintained surface (platform variance, upgrade behavior, failure modes)
without moving that bar. Deferred, not rejected: post-v1 the quickstart
becomes its executable skeleton. Recorded on #184 when this proposal is
accepted.

### D6. Placement and linkage

`docs/QUICKSTART.md`; README's current walkthrough-ish fragments become a
link ("Start here: QUICKSTART") plus the config *reference* it already is.
The doc carries the fresh-storage note inline at the upgrade-relevant step,
sourced from the same wording ROADMAP uses.

## Risks / Trade-offs

- [Network dependence: tracks clone public GitHub repos in CI] → same
  exposure the beta.8 e2e already accepted; fixtures are tiny; the e2e tier
  is not on the PR-blocking path for docs-only edits.
- [Marker grammar too clever] → two tokens, one separator; parser unit tests
  are the contract; anything fancier is rejected in review.
- [Step-index coupling makes doc edits touch the test] → intended (see D2);
  the failure message names the step and the doc heading to keep the loop
  short.
- [Readiness wait guidance vs slow CI runners] → the documented wait wording
  stays qualitative ("typically under a minute for a small repo"); the test's
  timeout is its own, generous constant — the doc never promises a number CI
  must hit.

## Open Questions

- Whether `semsource add`'s non-interactive flags cover the multi-repo track
  exactly as documented or the track uses a config-file example instead —
  resolved by writing the doc against the real CLI in task 2.x (either form
  satisfies the spec; the doc documents whichever is true today, and a gap
  files an issue rather than growing this change).
