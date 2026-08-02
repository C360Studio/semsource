# ingestion-readiness delta — the surfaces must be reachable while not ready

## ADDED Requirements

### Requirement: Starting a source SHALL NOT prevent the service surfaces from becoming reachable

Bringing a source up SHALL NOT block the service's HTTP status surface or its
metrics endpoint from binding. A source's start SHALL complete promptly, doing
only fast deterministic setup, and SHALL perform its initial seed — and any work
sequenced after it, such as watcher startup — without holding the runtime's
startup path.

This is the availability precondition for every other guarantee in this
capability: a readiness signal that cannot be reached reports nothing, and a
consumer polling it cannot distinguish "still seeding" from "process is gone".

#### Scenario: Status is reachable while a long seed is still running

- **GIVEN** a source seeding a corpus large enough to take many minutes
- **WHEN** the status surface is polled shortly after process start
- **THEN** it answers, reporting the source in its seeding phase — rather than
  refusing the connection

#### Scenario: Metrics are reachable while a long seed is still running

- **WHEN** the metrics endpoint is scraped during a long initial seed
- **THEN** it answers and serves the publish counters for that source

#### Scenario: Unreachable source paths do not black out the surfaces

- **GIVEN** a source whose configured paths do not exist yet, so it is retrying
- **THEN** the status and metrics surfaces are still reachable throughout the
  retry window, and the source reports the condition rather than the process
  appearing dead

#### Scenario: Readiness gating is unchanged

- **WHEN** a consumer gates on the aggregate phase being `ready`
- **THEN** the gate behaves exactly as before — `ready` still means every
  configured source finished its initial seed, and a source that has started but
  not finished seeding does NOT report ready

### Requirement: A failed initial seed is reported, not swallowed

Because the initial seed no longer completes within the source's start, a seed
failure SHALL be surfaced through the source's own error reporting — its error
count and last-error detail, and a log at `Warn` or above — rather than being
silently lost when the start path returns successfully.

Failures that are immediate and deterministic — invalid configuration, or an
inability to construct the publisher — SHALL continue to fail the start itself,
so genuine misconfiguration still fails fast rather than becoming a runtime
surprise.

#### Scenario: Seed failure is visible on the status surface

- **GIVEN** a source whose initial seed fails after start has returned
- **WHEN** the status surface is polled
- **THEN** the source's error count is greater than zero, `last_error` describes
  the failure, and the source does not report itself as ready

#### Scenario: Invalid configuration still fails the start

- **GIVEN** a source configured with values that fail validation
- **THEN** the failure occurs during start, not asynchronously

### Requirement: Shutdown during a seed is safe

Stopping a source while its initial seed is still running SHALL cancel that seed
and wait for it to finish before tearing down the delivery path, so that no seed
work publishes into a stopped publisher and shutdown does not race the seed.

#### Scenario: Stop during an in-flight seed

- **GIVEN** a source whose initial seed is in progress
- **WHEN** the component is stopped
- **THEN** the stop returns only after the seed has stopped, and no entity is
  published after the publisher has been shut down
