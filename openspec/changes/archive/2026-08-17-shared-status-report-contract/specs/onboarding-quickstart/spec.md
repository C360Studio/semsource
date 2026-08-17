# onboarding-quickstart Delta

## MODIFIED Requirements

### Requirement: Troubleshooting is signal-keyed

Every troubleshooting entry in the quickstart SHALL name an observable signal
(aggregate phase, per-source phase, index/embedding readiness,
`files_parsed`/`bodies_offloaded` liveness, per-source `backpressure`,
submodule path states, error counts and `last_error`, query-route HTTP status
codes) and the action it indicates. Advice that cannot be tied to an
observable signal SHALL NOT appear.

#### Scenario: A stuck seed is diagnosable from the document

- **WHEN** a user's ingest appears stalled
- **THEN** the document routes them from what they can observe (which
  counters advance, which sub-signal is false, whether `backpressure` is
  set, which submodule state is shown) to the corresponding action, without
  reference to logs-only or source-code knowledge
