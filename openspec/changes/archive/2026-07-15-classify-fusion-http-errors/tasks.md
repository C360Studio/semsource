## 1. Lock the public contract with failing tests

- [x] 1.1 Add table-driven `TestFusionHTTPErrorContract_LocalRequestFailures` coverage for 405 plus
      `Allow: POST`, 413 oversized bodies, 400 malformed JSON, 400 blank query, and 503 not-started,
      decoding and asserting required envelope fields, content type, class, and retryability before
      implementation without depending on JSON field order.
- [x] 1.2 Add `TestFusionHTTPErrorContract_DependencyFailures` with injected downstream failures for
      classified invalid/contract drift (502), `natsclient.IsNoResponders`,
      `natsclient.ErrNotConnected`, `natsclient.ErrCircuitOpen`, classified transient (503), NATS and
      context deadline (504), explicit fatal (502), and unclassified dependency-origin (502); assert
      response bodies are sanitized and do not contain injected internal text.
- [x] 1.3 Add `TestFusionHTTPErrorContract_LocalFailures` proving an unclassified local assembly or
      encoding failure returns sanitized, non-retryable 500 rather than dependency 502 or transient
      503, without error-string matching.
- [x] 1.4 Add `TestFusionHTTPErrorContract_SuccessHonestyStates` proving not-ready and ready-with-miss
      remain unchanged HTTP 200 `fusion.Response` payloads.
- [x] 1.5 Add `TestFusionHTTPErrorContract_AllRegisteredRoutes` applying the contract table to every
      registered verb for both code and docs component instances so aliases cannot drift.

## 2. Implement the bounded HTTP adapter

- [x] 2.1 Add a private version-1 error envelope, stable code/class constants, and one JSON writer in
      `processor/code-context`; prove it with the focused writer assertions from task 1.1.
- [x] 2.2 Separate `*http.MaxBytesError`, JSON decoding, and local request validation; reject a blank or
      whitespace-only query before graph I/O while continuing to tolerate unknown JSON fields.
- [x] 2.3 Introduce a private typed `request|dependency|local` stage or equivalent separated control
      flow and implement precedence without error-text matching: canceled request stops; context/NATS
      timeout to 504; no-responders/not-connected/open-circuit/classified transient to 503; upstream
      invalid/decode to 502; fatal or unclassified dependency to 502; unclassified local to 500. Do
      not use `errs.Classify` as the unknown default or map generic `errs.IsInvalid` to caller 400.
- [x] 2.4 Wire the shared writer and classifier through method, lifecycle, request, and serve failures
      for both lens instances without wrapping successful `fusion.Response` bodies.

## 3. Integration, review, and documentation

- [x] 3.1 Add `TestFusionHTTPErrorContract_PublicHandlers` through each component's
      `RegisterHTTPHandlers` shared-mux boundary (the interface used by ServiceManager) for at least
      one code and one docs route, proving status, JSON envelope, sanitization, and unchanged success
      behavior at the public boundary.
- [x] 3.2 Document the version-1 fusion HTTP error envelope, status/code table, retryability semantics,
      and unchanged 200 not-ready/miss behavior in `README.md`; update
      `docs/testing/readme-surface-coverage.md` to name the public handler contract tests and add the
      breaking error-path migration note.
- [x] 3.3 Obtain go-reviewer sign-off on context handling, classification ordering, information
      disclosure, response-write behavior after cancellation, and parity across both lens instances.

## 4. Gates

- [x] 4.1 Run focused `go test ./processor/code-context`, then `go test ./...`, `go vet ./...`, and
      `task lint`; all pass with no revive warnings.
- [x] 4.2 Run `go test -race -tags=integration ./processor/code-context`; verify no arbitrary sleeps
      were introduced and the named public-boundary contract test passes for both lens instances.
- [x] 4.3 Run `openspec validate classify-fusion-http-errors --strict` and complete an adversarial
      verification pass before archive; only then may a workbench client depend on error contract 1.
