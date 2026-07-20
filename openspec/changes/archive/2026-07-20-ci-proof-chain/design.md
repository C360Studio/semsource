# Design: CI Proof Chain

## Context

The audit's test-honesty review found the advertised proof chain largely gate-less: CI runs unit
tests only; the compose smoke stops at `tools/list` name-matching; the default-component-set
guard covers only supersession (the exact bug class that shipped in beta.1); the GRAPH stream's
explicit subject list — the sole protection against the PubAck silent-empty-results footgun —
has no pinning test. Post-audit work sharpened the case: the extended smoke immediately caught a
real `add_source` bug that had made #81's round-trip unrunnable, and the staleness agent found
`TestIntegration_DomainScopedRetrieval_OnTheWire` broken on main — orphaned by #84's deliberate
docs-scope change, invisible precisely because integration tests never run in CI. Two more finds
join the scope: the older demotion integration tests build fusion engines without
`.WithSignals(fusionvocab.New())` (their ranking assertions can pass on resolve-order
coincidence), and `publish-ui` wedged twice in one day (36 and 76 minutes against a 3m25s
baseline) with no `timeout-minutes`, grinding toward GitHub's 6-hour default until manually
cancelled.

## Goals / Non-Goals

**Goals:**

- PR + main CI executes the integration-tagged critical path (with the one broken-on-main test
  fixed first) within a documented budget.
- The compose smoke asserts ANSWER CONTENT for every MCP tool.
- Unit gates pin the full default component spawn map and the GRAPH stream subject list.
- Ranking assertions in demotion tests exercise real salience, not resolve-order luck.
- Wedged publish jobs self-terminate.

**Non-Goals:**

- Running the compose smoke itself in CI (docker-compose-in-actions is a heavier lift with its
  own flake budget; the smoke stays the release-time/local gate, now content-asserting).
- New integration tests beyond the seed-window pin — this change wires and hardens what exists.

## Decisions

### D1: Integration job via testcontainers on the stock runner

New `test-integration` CI job on PR + main: `go test -tags=integration ./internal/governance/
./processor/mcp-gateway/ ./processor/code-context/` — the critical-path packages (MCP answers,
fusion pipeline, versionDiff, readiness, lifecycle). The suites already self-provision NATS via
testcontainers, and Actions' ubuntu runners ship Docker — no service-container choreography.
Budget: ~5 minutes observed locally (~17s test time + container pulls); `timeout-minutes: 15`
documents the ceiling. Runs parallel to the existing jobs; `build-and-push` gains it as a
dependency so nothing publishes on a red proof chain.

Precondition: fix `TestIntegration_DomainScopedRetrieval_OnTheWire` — its docs-lens expectation
updates from `[web]` to `[web, config]`, matching #84's deliberate `docScopeDomains` change
(the code comment already explains why config joined).

### D2: Smoke asserts answer content per tool

`scripts/core-profile-smoke.sh` (mcp_call + unescape already exist) gains one content-asserting
`tools/call` per remaining tool: `code_context` on the fixture's known symbol asserts a
verbatim-body fragment; `code_context` on a nonexistent symbol asserts the honest-miss shape
(did_you_mean/misses, never a fabricated node); `code_impact` asserts the relations/impact
shape; `doc_context` asserts fixture README content; `code_search` gates on `embedding.ready`
then asserts the fixture hit; `code_changes` without registered versions asserts the honest
"no indexed entities" note (content-asserted honesty, not skipped). add/remove/status keep
their existing lifecycle round-trip.

### D3: Unit pins for the spawn map and stream subjects

`cmd/semsource/run_test.go`: the default-component-set test asserts the COMPLETE required map
(mcp-gateway, code-context, doc-context, source-manifest, supersession, graph stack, …) — read
the expected set from the same source of truth run.go uses, assert every product-surface
component present. A second test pins the GRAPH stream subject list: explicit subjects only,
zero overlap with `rpcReplySubjects` (the PubAck footgun), and no wildcard that could swallow a
request/reply subject.

### D4: Seed-window phase pin

honest-readiness (#78) made mid-seed `phase: "seeding"` real and live-verified it; this change
adds the regression pin if #78's tests don't already assert the mid-seed window (verify first —
reference the existing test rather than duplicating if it exists).

### D5: Salience-real demotion assertions

`supersession_demote_integration_test.go` and `multi_source_lineage_integration_test.go` attach
`.WithSignals(fusionvocab.New())` exactly as the staleness test does, so demotion assertions
exercise signed salience. If either test then fails, that is a real finding to fix, not to
paper over.

### D6: Job timeouts

`timeout-minutes` on every job, sized ~3× observed baseline: test 10, lint 5, ui-quality 10,
test-integration 15, build-and-push 30, publish-ui 15, ui-release-smoke 15. A wedged QEMU build
now self-terminates in minutes instead of grinding toward the 6-hour default (twice in one day,
manually cancelled both times).

## Risks / Trade-offs

- [testcontainers flake on shared/loaded runners] → scoped package list, 15-min ceiling, and
  the suites' own health-error retries; a flaky-in-CI test is a finding, not a reason to unwire.
- [Timeout too tight on a legitimately slow day] → 3× baseline margins; a timeout failure is
  re-runnable and visible, unlike a silent 6-hour grind.
- [WithSignals may surface latent ranking assertions that were passing coincidentally] →
  intended; fix forward.

## Migration Plan

CI-only + test-only + one smoke-script extension. No product code. Rollback = revert.

## Open Questions

- None blocking.
