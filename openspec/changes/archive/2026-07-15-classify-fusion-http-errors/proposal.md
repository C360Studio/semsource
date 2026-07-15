## Why

The fusion HTTP gateway currently collapses malformed input, graph dependency failures, request
timeouts, cancellations, and unexpected server faults into the same plain-text `400 bad request`.
The opt-in workbench needs stable, honest failure semantics before it can decide whether to correct a
request, retry, degrade a view, or report an operator fault.

## What Changes

- **BREAKING (error responses only):** replace the former plain-text/generic-400 failure behavior
  with versioned JSON bodies and differentiated 4xx/5xx statuses. Successful fusion responses remain
  wire-compatible.
- Return a versioned, machine-readable JSON error envelope from the fusion HTTP routes.
- Distinguish malformed requests, oversized bodies, unavailable/transient graph failures,
  server-side deadlines, upstream contract/fatal failures, and unexpected local failures with
  stable codes and appropriate HTTP statuses.
- Preserve the successful `fusion.Response` body and status unchanged.
- Apply the same behavior to every `/code-context/*` and `/doc-context/*` verb.
- Add contract and integration tests that exercise the classifications without matching error text.

## Non-goals

- Changing the SemStreams fusion request, success response, lens, graph-query, or NATS RPC contracts.
- Defining the proposed fusion v2 graph/facts facet or blocking fusion-backed workbench views on it.
- Changing the MCP tool response contract or the direct `code.v1.*` / `docs.v1.*` NATS surfaces.
- Exposing internal dependency messages, stack traces, entity data, or infrastructure details to
  browser clients.
- Standardizing every SemSource HTTP error response in this change.
- Promising a synthetic response after the caller has already canceled or disconnected.

## Consumers

- The optional SemSource workbench uses codes and retryability to separate request correction,
  temporary degradation, timeout UX, and server faults.
- SemTeams, SemSpec, SemDragon, SemOps, and other HTTP consumers may adopt the versioned error
  contract while their successful fusion integration remains unchanged.
- Existing MCP and NATS consumers are unaffected by this HTTP-only change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `fusion-gateway`: add a stable HTTP error envelope and honest HTTP status mapping while preserving
  successful fusion responses.

## Impact

- `processor/code-context` gains typed HTTP error classification and JSON response writing shared by
  both lens instances.
- Unit and integration coverage expands across all error classes and both route prefixes.
- Browser clients may stop parsing the current plain-text `bad request` response and branch on the
  documented code and retryability fields.
- Existing HTTP clients that assert the former 400/plain-text failure behavior require a release-note
  migration to status-plus-code handling.
- No SemStreams dependency change or upstream implementation is required; this change translates
  already-classified dependency and context failures at SemSource's HTTP boundary.
