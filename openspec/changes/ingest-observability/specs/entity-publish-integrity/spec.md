# entity-publish-integrity delta — backpressure that does not drop

## ADDED Requirements

### Requirement: Backpressure is visible even when nothing is lost

The publisher already drops loudly. Backpressure that does NOT drop — the bounded
retry path taken when the transport refuses a publish — SHALL also be observable,
because a publisher retrying every entity is functionally stalled while reporting
no drops, no failures, and no errors.

Sustained retrying SHALL raise a `Warn` naming the condition, on entering that
state rather than per attempt, and SHALL clear when publishing recovers. The
cumulative retry count SHALL be exported as a metric and surfaced in source
status alongside the existing published, failed, and dropped counts.

#### Scenario: A retrying publisher is not silent

- **GIVEN** a transport applying sustained backpressure so that publishes are
  retried rather than refused outright
- **WHEN** the instance runs at the default log level
- **THEN** a `Warn` names the backpressure condition, and the retry count is
  visible in metrics and source status

#### Scenario: Retrying does not log per attempt

- **WHEN** backpressure persists across many entities and many retry attempts
- **THEN** the condition is logged on entry and on recovery, not once per attempt

#### Scenario: Recovery is reported

- **WHEN** the transport accepts publishes again after a period of backpressure
- **THEN** the recovery is recorded and the condition is no longer reported as active
