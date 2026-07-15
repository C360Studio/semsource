## ADDED Requirements

### Requirement: Versioned fusion HTTP error envelope

Every unsuccessful response produced by a `/code-context/*` or `/doc-context/*` fusion route SHALL
use `Content-Type: application/json` and SHALL contain an `error` object with
`contract_version`, `code`, `class`, `message`, and `retryable`. The contract version SHALL be `1`;
codes SHALL be stable lowercase snake case; class SHALL be `invalid`, `transient`, or `fatal`; and
messages SHALL be sanitized summaries that do not expose raw dependency or internal error text.

#### Scenario: Dependency detail is not exposed

- **GIVEN** a dependency failure contains an internal NATS subject, storage identifier, or entity ID
- **WHEN** either fusion HTTP instance returns the failure
- **THEN** the JSON envelope contains the stable public code and sanitized message
- **AND** the raw internal text is absent from the response body
- **AND** logs contain safe route, verb, code, and class context without requiring verbatim cause text

#### Scenario: Method is not allowed

- **WHEN** a caller uses a method other than POST on a fusion HTTP route
- **THEN** the response is 405 with code `method_not_allowed`, class `invalid`, and retryable `false`
- **AND** the response includes `Allow: POST`

### Requirement: Honest local request classification

The fusion HTTP boundary SHALL classify failures attributable to the caller before invoking or while
decoding the local request. An oversized request SHALL return 413 `request_too_large`; malformed JSON
SHALL return 400 `invalid_json`; and a locally invalid request such as a blank query SHALL return 400
`invalid_request`. These errors SHALL have class `invalid` and retryable `false`. Unknown JSON fields
SHALL remain compatible and SHALL NOT be rejected solely because the server does not use them.

#### Scenario: Oversized body is distinct from malformed JSON

- **WHEN** a request body exceeds the configured fusion HTTP limit
- **THEN** the response is 413 with code `request_too_large`
- **AND** it is not reported as malformed JSON or a dependency failure

#### Scenario: JSON cannot decode

- **WHEN** a bounded request body is not valid JSON
- **THEN** the response is 400 with code `invalid_json`
- **AND** retryable is `false`

#### Scenario: Query is blank

- **WHEN** a decoded fusion request has an empty or whitespace-only query
- **THEN** the response is 400 with code `invalid_request`
- **AND** no graph dependency request is made

### Requirement: Honest dependency and server classification

The fusion HTTP boundary SHALL distinguish lifecycle readiness, dependency availability, deadlines,
upstream contract defects, fatal upstream failures, and local internal failures. A fusion component
that has not started SHALL return 503 `component_not_ready`. Explicit transient dependency failures
and `natsclient.IsNoResponders`, `natsclient.ErrNotConnected`, or
`natsclient.ErrCircuitOpen` SHALL return 503 `dependency_unavailable`; a server-side deadline or NATS
request timeout SHALL return 504 `upstream_timeout`; an internally generated upstream request
rejected as invalid or an undecodable upstream response SHALL return 502
`upstream_contract_error`; an explicit fatal or otherwise unclassified dependency-origin failure
SHALL return 502 `upstream_failure`; and an unclassified SemSource-local failure SHALL return 500
`internal_error`. The implementation SHALL preserve request, dependency, and local origin through
typed stages or separated control flow and SHALL NOT classify failures by matching error-message
text. Caller cancellation SHALL stop work without requiring a synthetic response.

#### Scenario: Component has not started

- **WHEN** a fusion HTTP route is called before its component has completed startup
- **THEN** the response is 503 with code `component_not_ready`, class `transient`, and retryable `true`

#### Scenario: Upstream invalid does not blame the caller

- **GIVEN** the fusion engine constructed the graph request
- **WHEN** the graph dependency rejects it as invalid or returns an undecodable contract payload
- **THEN** the HTTP response is 502 with code `upstream_contract_error`
- **AND** it is not returned as a caller-attributable 400

#### Scenario: Temporary dependency failure can be retried

- **WHEN** a graph dependency has no responders, is disconnected, has an open circuit, or returns an
  explicitly transient failure
- **THEN** the response is 503 with code `dependency_unavailable`, class `transient`, and retryable `true`

#### Scenario: Server deadline expires

- **WHEN** the fusion operation exceeds its server-side deadline
- **THEN** the response is 504 with code `upstream_timeout`, class `transient`, and retryable `true`

#### Scenario: Unknown local failure is not made transient

- **WHEN** SemSource encounters an unclassified local failure while serving a fusion HTTP request
- **THEN** the response is 500 with code `internal_error`, class `fatal`, and retryable `false`
- **AND** a generic classifier default does not convert it to a retryable 503

#### Scenario: Unknown dependency failure is not treated as local

- **WHEN** an unclassified error originates in the fusion engine or graph dependency stage
- **THEN** the response is 502 with code `upstream_failure`, class `fatal`, and retryable `false`
- **AND** error-message text is not used to distinguish it from a local failure

### Requirement: Successful fusion semantics remain unchanged

Every successful fusion HTTP request SHALL continue to return the SemStreams `fusion.Response`
unchanged with HTTP 200. A not-ready index response and a ready response containing `misses` SHALL be
treated as successful honesty states, not transport failures. The same success and error behavior
SHALL apply to every registered verb under both the code and docs route prefixes.

#### Scenario: Index is not ready

- **WHEN** fusion returns an honesty envelope with `index.ready` false
- **THEN** the HTTP response remains 200 with the original `fusion.Response`
- **AND** it is not converted to `component_not_ready` or `dependency_unavailable`

#### Scenario: Ready query has no result

- **WHEN** fusion returns a ready response containing a miss
- **THEN** the HTTP response remains 200 with the original `misses`
- **AND** it is not converted to 404 or an error envelope

#### Scenario: Code and docs routes stay aligned

- **WHEN** the same success or failure class is exercised through any registered code or docs verb
- **THEN** both component instances return the same status and wire shape
