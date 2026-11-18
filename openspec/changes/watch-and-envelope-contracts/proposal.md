# Name the git-watch and published-envelope contracts that #270's tests verify

Approved-by:

Track: design (adds spec Requirements; test-only otherwise).

## Why

#270: the pre-tag audit found six mutants that pass everything CI runs. Four map to existing
Requirements, and their new tests cite them. Two protect behaviour no spec names, so their tests
have nothing honest to cite:

- **Git watch events carry typed state.** `processor/git-source` publishes a watch event's
  `EntityStates`; `handler/git/handler.go:314` builds them. Emptying them means commits made after
  startup never reach the graph, and no error is counted. `typed-source-change-events` covers the
  doc and URL watch paths only.
- **Published envelopes decode through the bootstrap registry.** `source-manifest` publishes its
  manifest, predicate-schema and status broadcasts on GRAPH as typed envelopes. Removing
  `sourcemanifest.RegisterPayloads` from `buildPayloadRegistry()` changes nothing any test sees, yet
  any decoder built on that registry (SemEngine's message logger) then fails on all three.

## What Changes

- `typed-source-change-events` gains a Requirement: a git watch event for a new HEAD carries the
  commit's typed entity state under the configured authority.
- `advertised-surface-coverage` gains a Requirement: every payload type SemSource publishes as a
  typed envelope decodes through the registry `buildPayloadRegistry()` returns.
- No production code changes. The six tests are in PR #311.

## Non-goals

- The audit's "related test gaps" (`e2e_test.go:1808` deferred guard, MCP Tier 0 roster, `/ws`
  frames, the governance re-implementation of `exact_seed.go`). They are separate findings.
- Removing the payload registration: the component pattern requires it, and the test makes it live.

## Impact

Two spec Requirements; tests in `handler/git` and `cmd/semsource`.
