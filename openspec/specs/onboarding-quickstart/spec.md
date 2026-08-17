# onboarding-quickstart Specification

## Purpose

A new user with Docker and a repo (or several) reaches `phase: ready` and a
correct first query by following the quickstart document alone — and that
exact documented path is executed verbatim by CI against real fixtures, so
documentation drift fails a build instead of a user.

## Requirements

### Requirement: The documented path reaches ready and a correct first query

Following the quickstart's commands verbatim, in order, on a machine with
only the documented prerequisites SHALL take a repository from nothing to
aggregate `phase: ready` and a first query whose result contains documented,
expected content. The document SHALL include the fresh-storage upgrade note
(the beta.160+ contract) at the point where it applies.

#### Scenario: Single-repo zero to first query

- **WHEN** the single-repo track's commands run verbatim against a real
  repository fixture
- **THEN** the status surface reaches `phase: ready` within the documented
  wait guidance, and the documented first query returns the expected symbol
  or passage from that repository

#### Scenario: No undocumented steps

- **WHEN** the track is executed on a clean environment with only the
  documented prerequisites
- **THEN** no step outside the document is required to reach ready — a
  missing step is a document defect, not user error

### Requirement: The quickstart is executable truth

The quickstart's command blocks SHALL carry machine-readable track markers,
and CI SHALL extract and execute the marked blocks verbatim, in document
order, per track. The verification MUST NOT maintain a separate copy of the
commands.

#### Scenario: A command edit is exercised by CI

- **WHEN** a marked command block in the document changes
- **THEN** the next CI run executes the changed command — a command that no
  longer produces the documented outcome fails the build

#### Scenario: Assertions live in the test, not the document

- **WHEN** the extracted blocks are executed
- **THEN** readiness and query-correctness assertions run between steps
  without appearing as user-facing content in the document

### Requirement: Multi-repo onboarding is documented and verified

The quickstart SHALL document registering multiple repositories — explicit
`project` (and `version` where it applies) identity per source, a remote +
local mix, and per-source readiness — and SHALL explain what identity scoping
means for cross-repo queries: org-sovereign namespaces versus `public.*`,
and the monorepo/submodule case (canonical submodule identity and
dedup-by-pin, per ADR-0012). The multi-repo track SHALL be executed by CI
against a two-repository fixture.

#### Scenario: Two repositories to ready

- **WHEN** the multi-repo track's commands run verbatim against two
  repository fixtures
- **THEN** each source reports its own readiness on the status surface, the
  aggregate reaches `ready`, and a documented query returns content
  attributable to each repository's own identity scope

#### Scenario: Shared code dedups across sources

- **WHEN** the two fixtures contain the same code at the same pinned identity
  (the standalone repo and a submodule pin at the same SHA)
- **THEN** the documented identity explanation matches observed behavior:
  one merged entity, not per-source forks

### Requirement: Troubleshooting is signal-keyed

Every troubleshooting entry in the quickstart SHALL name an observable signal
(aggregate phase, per-source phase, index/embedding readiness,
`files_parsed`/`bodies_offloaded` liveness, submodule path states,
error counts and `last_error`, query-route HTTP status codes) and the action
it indicates. Advice that cannot be tied to an observable signal SHALL NOT
appear. (The per-source backpressure flag was proposed for this list but is
not served on any status surface today — the aggregator drops it; tracked in
#188. When it surfaces, a troubleshooting entry SHOULD key on it.)

#### Scenario: A stuck seed is diagnosable from the document

- **WHEN** a user's ingest appears stalled
- **THEN** the document routes them from what they can observe (which
  counters advance, which sub-signal is false, which submodule state is
  shown) to the corresponding action, without reference to logs-only or
  source-code knowledge
