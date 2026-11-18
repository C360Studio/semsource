## Context

Each new test in this repository cites the Requirement it verifies (`ateam test-spec`). #270's
git-watch and payload-registration tests verify behaviour no Requirement names. Seeding the two
Requirements is the lazy seeding the spec rules allow: written from the code as it is today.

## Goals / Non-Goals

Goal: an honest citation for each of the six tests. Non-goals are in `proposal.md`.

## Decisions

### D1. The git-watch Requirement extends typed-source-change-events

That capability is the contract between a handler's `Watch` channel and its processor. Git is a
third handler on the same contract; the Requirement says what its event must carry, not how the
poll loop finds a change.

### D2. The envelope Requirement goes in advertised-surface-coverage

Its Purpose covers direct consumer contracts. The GRAPH broadcasts are one, and the Requirement is
about evidence that they stay decodable, which is that capability's job.

## Risks / Trade-offs

None beyond the tests themselves; no runtime behaviour changes.

## Migration Plan

None.

## Open Questions

None.
