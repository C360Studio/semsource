# Design: SemSource-owned NATS test fixture

## Context

- The pin's helper is `natsclient/test_client.go` at semstreams `8b99efe9c66a` (987 lines). It provides:
  - phased start;
  - one mapped-port replacement (gh#736);
  - bounded test streams (`testStreamConfig`, `:908-923`);
  - JetStream only via `WithJetStream`, `WithKV` or `WithStreams` (`:440-478`).
- SemEngine's replacement, `internal/harness/natsfixture`, is internal to SemEngine. Its owner ruled consumer
  fixtures consumer-owned (semengine#568) and documents the recipe: `NewClient(url)`, then `Connect(ctx)`, then
  `Close(ctx)`.
- SemSource's surface, measured on main at `cb74efd`:
  - options used: `WithKV` 33, `WithStreams` 26, `WithJetStream` 1;
  - fields used: `.Client`, plus `.URL` in `cmd/semsource`;
  - every broker is per-test; none is shared.

## Goals / Non-Goals

- **Goals:**
  - Behavior parity with the pin for the surface SemSource uses.
  - At the port, only the `natsclient` import in `internal/natstest` changes.
  - Failure paths are bounded and visible.
- **Non-Goals:**
  - A broker shared per package.
  - SemEngine's 2.15.0 image.
  - A fixture importable by other repositories.

## Decisions

1. **Keep a per-test broker.** It preserves isolation by construction. SemEngine's shared broker with a reset
   between tests is faster but adds a reset contract and state-leak risk. Marked `// shortcut:`.
2. **Pin `nats:2.14.4-alpine` by digest.** That is the image the SETUP 03A lane runs. The pin floated
   `2.14-alpine`. Moving to 2.15 changes the server under test, so it belongs to the port.
3. **Collapse `WithKV` into `WithJetStream`.** The two were identical at the pin (`:455-460`).
4. **Use a narrow `broker` seam (`Host`, `MappedPort`, `Terminate`).** It lets the replacement, cancellation and
   cleanup rules be unit-tested without Docker. `testing/synctest` makes the window-expiry case deterministic.
5. **Replace only on observed absence.** A port is absent when testcontainers reports `errdefs.NotFound` for the
   whole 10 s window. Host failures, inspect failures and cancellation are not absence.

6. **State 7 tested behaviors that no spec states.** Each of the 4 requirements restates exactly what its tests
   assert, and nothing more. The owner ruled (#228, 2026-11-09) to add them here, so they're spec-backed now and
   not rediscovered at the port.

## Risks / Trade-offs

- 48 containers per integration run. That is the existing cost; the shared-broker upgrade addresses it.
- A cold image pull counts against the 30 s attempt bound. The pin had the same bound.
- The 4 new requirements describe behavior as tested, so a test's blind spot is also the spec's. 11 citations to
  existing requirements are medium-confidence fits and are listed on #228 for review.

## Migration Plan

The 38 files moved mechanically to `natstest.New`, `WithJetStream`, `WithStreams`, `Stream` and `*natstest.Server`.
`./internal/natstest/` joins CI's integration list.

## Open Questions

None. The CI list's other gaps are tracked separately in #254.
