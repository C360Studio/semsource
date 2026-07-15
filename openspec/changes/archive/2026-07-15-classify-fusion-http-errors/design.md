## Context

`processor/code-context` serves both the code and docs fusion lenses over HTTP. Successful requests
marshal the SemStreams `fusion.Response` directly, but nearly every failure after method/lifecycle
checks is currently emitted as plain-text `400 bad request`. That loses the distinction between a
request the caller can correct, a dependency outage worth retrying, an upstream contract defect, and
a SemSource-local failure.

The optional workbench is the first browser consumer that needs to branch on those distinctions.
This design changes only the SemSource HTTP adapter. SemStreams continues to own the fusion engine,
NATS error classifications, and success contract.

## Goals / Non-Goals

**Goals:**

- Give every code/docs fusion HTTP error a stable, sanitized JSON representation.
- Map locally attributable request failures, dependency failures, deadlines, and server faults to
  honest HTTP statuses.
- Keep successful, not-ready, and ready-with-miss `fusion.Response` payloads unchanged.
- Apply one contract to every registered verb under both route prefixes.

**Non-Goals:**

- Changing SemStreams `pkg/fusion`, graph-query, NATS RPC, GraphQL, or MCP contracts.
- Adding server retries, backoff, request IDs, authentication, or a repository-wide HTTP framework.
- Defining fusion v2 graph facts/evidence; that framework gap is tracked by semstreams#533.
- Passing arbitrary upstream error messages or structured details to a browser.

## Decisions

### D1 - Version and sanitize a fusion-HTTP-specific JSON error envelope

Errors use this shape with `Content-Type: application/json`:

```json
{
  "error": {
    "contract_version": "1",
    "code": "dependency_unavailable",
    "class": "transient",
    "message": "graph query service is temporarily unavailable",
    "retryable": true
  }
}
```

`code` is stable lowercase snake case. `class` is `invalid`, `transient`, or `fatal`.
`retryable` is explicit rather than inferred by each client. `message` is a fixed public summary;
responses never expose raw NATS, storage, entity, or stack detail. Logs carry safe operational
context such as component, route, verb, public code, and boundary class; a cause may be included only
after redaction under the logging policy. The envelope version is independent of
`fusion.ContractVersion` because it versions the SemSource HTTP error boundary, not the successful
SemStreams response. The class vocabulary parallels SemStreams, but its value describes the action
at the SemSource HTTP boundary; it is not a pass-through copy of an upstream class.

An RFC problem-details response was considered. A small owned envelope is preferred because clients
need a domain-stable code, retryability, and SemStreams-compatible class, while introducing a second
content type and URI registry would not improve this bounded surface.

### D2 - Attribute failure origin before applying classification

The mapping is:

| Status | Code | Class | Retryable | Origin |
|---|---|---|---|---|
| 405 | `method_not_allowed` | invalid | false | non-POST request; also set `Allow: POST` |
| 413 | `request_too_large` | invalid | false | `*http.MaxBytesError` |
| 400 | `invalid_json` | invalid | false | request JSON cannot decode |
| 400 | `invalid_request` | invalid | false | local validation such as blank query |
| 503 | `component_not_ready` | transient | true | fusion component has not started |
| 503 | `dependency_unavailable` | transient | true | named unavailable transport or classified transient |
| 504 | `upstream_timeout` | transient | true | server deadline or NATS request timeout |
| 502 | `upstream_contract_error` | fatal | false | internally built request rejected or response cannot decode |
| 502 | `upstream_failure` | fatal | false | fatal or unclassified dependency-origin failure |
| 500 | `internal_error` | fatal | false | unclassified SemSource-local failure |

The handler SHALL preserve origin mechanically with a private typed stage (`request`, `dependency`,
or `local`) or equivalent separated stages; it SHALL NOT infer origin or class from error-message
substrings. Mapping uses this precedence:

1. If the caller request context is canceled, stop work without promising another write.
2. `context.DeadlineExceeded` and NATS request-timeout errors map to 504.
3. `natsclient.IsNoResponders`, `natsclient.ErrNotConnected`, `natsclient.ErrCircuitOpen`, and
   explicitly classified transient dependency errors map to 503.
4. Dependency-origin invalid/contract-decode errors map to 502 `upstream_contract_error`.
5. Explicit fatal and otherwise unclassified dependency-origin errors map to 502 `upstream_failure`.
6. Otherwise unclassified local-origin errors map to 500.

Two tempting generic mappings are rejected. `errs.IsInvalid(err) => 400` would blame the caller when
the fusion engine itself constructed the rejected graph request. `errs.Classify(err)` is also not an
acceptable default because SemStreams classifies unknown errors as transient, which would turn local
defects into endlessly retryable `503` responses.

The server does not invent a `Retry-After` delay.

The pinned SemStreams fusion NATS client wraps standard-library `*json.SyntaxError` and
`*json.UnmarshalTypeError` values when an upstream response cannot decode; it does not yet expose a
fusion-specific contract-error type. SemSource recognizes those concrete typed causes with
`errors.As` at this HTTP boundary. This is deliberate, bounded coupling to Go's JSON decoder, not
message matching. If SemStreams adds a stable fusion transport error type later, the adapter should
prefer that type and retain these checks only for compatibility with older clients.

### D3 - Preserve successful honesty semantics

`IndexStatus.Ready == false` is a successful fusion answer and remains HTTP 200. A ready graph with
no result remains HTTP 200 with `misses`; absence is not HTTP 404. Successful bodies remain the exact
SemStreams `fusion.Response`, with no SemSource wrapper.

### D4 - Share one adapter across code and docs routes

The code/docs component instances use one private error type, classifier, and writer in
`processor/code-context`. The private representation carries the typed origin/stage needed to keep
dependency and local failures distinct without string matching. Tests run the same table across both
lens instances and every registered verb. No lens-specific status mapping is permitted.

## Risks / Trade-offs

- **Existing clients may compare the old plain-text body or expect 400.** → Treat this as a public
  contract change, document the new envelope, and keep success responses byte-compatible.
- **Over-broad error helpers can misattribute upstream faults.** → Test downstream classified
  invalid as 502 and unknown local errors as 500; do not use one generic class-to-status switch.
- **Sanitization can remove useful diagnostics.** → Log safe structured component, route, verb,
  code, and class fields; include a cause only through the repository redaction policy.
- **Duplicating an HTTP convention can create later drift.** → Keep the helper private and bounded;
  generalize only after another SemSource HTTP capability proves the same contract.

## Migration Plan

1. Add failing contract tests without changing production behavior.
2. Add the private envelope, classifier, and writer; then wire all code/docs verbs.
3. Document the error contract and run focused, package, lint, vet, and full Go gates.
4. Archive this change before a workbench client treats the HTTP error shape as stable.

Rollback restores the previous handler while leaving successful fusion responses untouched. There is
no graph-state or stored-data migration.

## Open Questions

None blocking. A future workbench capability response may advertise an identifier such as
`semsource.fusion-http-error.v1`; that discovery mechanism belongs to the workbench capability slice,
not this transport change.
