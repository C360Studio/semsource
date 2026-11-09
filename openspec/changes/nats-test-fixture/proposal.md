# SemSource-owned NATS test fixture

Closes [#228](https://github.com/C360Studio/semsource/issues/228). Track: `design` (adds a spec Requirement).

Approved-by:

## Why

SemEngine does not carry SemStreams' `natsclient.NewTestClient`, and it ruled a consumer test fixture
consumer-owned ([semengine#568](https://github.com/C360Studio/semengine/issues/568)). Without a fixture of our own,
all 38 SemSource integration test files (48 call sites) stop compiling at the port and have nowhere to move.
`internal/natstest` (PR #252) replaces the helper. Its contract is not yet stated anywhere, so its tests cannot
cite one (`ateam test-spec`).

## What Changes

- Add one requirement to `integration-fixture-reproducibility`: the NATS fixture's contract. That covers:
  - per-test ownership;
  - JetStream only on request;
  - bounded streams;
  - bounded setup;
  - a single, guarded replacement when the client port is unpublished;
  - visible cleanup and cancellation causes.
- Add `// spec:` citations to the 12 fixture tests and the 46 migrated integration tests. Each migrated test cites
  the existing requirement it proves.

## Non-goals

- Sharing one broker per package (SemEngine's shape). It stays a marked `// shortcut:` upgrade.
- Changing the NATS image to SemEngine's 2.15.0. That waits for the port.
- Any production code change.

## Impact

- Test-only code: `internal/natstest/`, 38 `_test.go` files, `.github/workflows/ci.yml` (integration list),
  `go.mod` (tidy).
- One capability spec gains a requirement. No consumer-visible surface changes.
