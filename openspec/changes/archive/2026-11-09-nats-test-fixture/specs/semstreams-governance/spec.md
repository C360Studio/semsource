## ADDED Requirements

### Requirement: Component Stop keeps a subscription until its callbacks drain

When a component's `Stop` cannot drain a request subscription before its context ends, because a callback is still
running, `Stop` SHALL return an error and SHALL keep the subscription and its running state, so a later `Stop` can
retry the drain. A later `Stop` that drains successfully SHALL clear both, and the in-flight request SHALL complete.
This applies to code-context, source-manifest, and supersession.

#### Scenario: Stop's deadline expires during a callback

- **GIVEN** a component holding a request subscription whose handler is blocked mid-callback
- **WHEN** `Stop` runs with an already-ended context
- **THEN** it returns an error and the component still holds the subscription

#### Scenario: Stop is retried after the callback finishes

- **WHEN** the callback is released and `Stop` runs again with a live context
- **THEN** it returns nil, the component is no longer running, it holds no subscriptions, and the request completes
