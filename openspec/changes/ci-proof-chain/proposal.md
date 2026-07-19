# Proposal: CI Proof Chain

**Priority: P2** (cheap, high leverage — turns "claimed proven" into "gate-proven")

## Why

The audit's test-honesty review found that the behaviors the docs and roadmap describe as proven
are largely not exercised by any automated gate:

1. **The integration-tagged proof chain never runs in CI** (`.github/workflows/ci.yml:24` runs
   unit tests only): MCP answer content, the fusion pipeline, versionDiff, and readiness gating
   are all `-tags=integration` suites that no gate executes.
2. **No gate performs a real MCP `tools/call`** — smoke stops at `tools/list` name-matching and a
   probe that accepts HTTP 400 (`scripts/core-profile-smoke.sh:293`). The audit's own graded
   interrogation was the first end-to-end MCP answer verification this repo has had.
3. **The default-component-set guard covers only supersession** (`cmd/semsource/run_test.go:16`) —
   mcp-gateway/code-context/doc-context could drop from the spawn map with all unit gates green.
   This exact bug class shipped in beta.1 (supersession missing from the default set).
4. **The GRAPH stream's explicit subject list — the only protection against the documented PubAck
   silent-empty-results footgun — has no pinning test** (`cmd/semsource/run.go:935`).
5. The only phase-transition test for aggregate status is shaped to sidestep the mid-seed window
   (companion to `honest-readiness-and-errors`, which fixes the behavior; this change pins it).

## What Changes

- CI runs the integration suite (NATS service container or dockerized step) on PR + main; runtime
  budget kept sane by scoping to the critical-path packages.
- The compose smoke performs at least one real `tools/call` per MCP tool with content assertions
  (a miniature of the audit's graded set: known symbol → verbatim-body check; nonexistent symbol →
  honest miss; status → readiness shape).
- The default-component-set test asserts the full required spawn map (every component the product
  surface depends on), so a dropped registration fails unit CI.
- A pinning test locks the GRAPH stream subject list against overlap with request/reply subjects.
- A seed-window phase test asserts `ready` is not reported mid-seed (lands with or after
  `honest-readiness-and-errors`).

## Capabilities

### Modified Capabilities
- `advertised-surface-coverage`: all five gaps above become ADDED requirements with scenarios —
  "integration proof chain runs in CI", "MCP tools have answer-content smoke coverage",
  "default component set is pinned", "GRAPH stream subjects are pinned", "phase gate covers the
  seed window".

### New Capabilities
<!-- none — this is entirely coverage of already-specified behavior -->

## Impact

- `.github/workflows/ci.yml`, `scripts/core-profile-smoke.sh`, `cmd/semsource/run_test.go`,
  `test/` integration harness; CI wall-clock (budgeted in design).
- Consumers: none directly; every future release inherits real proof.
- Boundary check: product-side CI only.
