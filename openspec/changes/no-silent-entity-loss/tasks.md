# Tasks: No Silent Entity Loss

## 1. Canonical segment sanitizer

- [ ] 1.1 Add `entityid.SanitizeSegment` sharing `SanitizeInstance` internals (allowlist, dash
      collapse, trim, hash fallback); unit tests incl. `+page`, `[slug]`, `(group)`, `@modal`,
      `clicks$`, `_examples`, unicode, empty, overlong
- [ ] 1.2 Property test: for a corpus of nasty inputs, `SanitizeSegment` output always passes
      semstreams `pkg/types.ValidateEntityID` as a segment, and distinct inputs never collide

## 2. AST identity integration

- [ ] 2.1 Route `source/ast` `SanitizePathSegment` and `BuildScopedInstanceID` (symbol names,
      receiver/scope markers, path fragments) through `entityid.SanitizeSegment`
- [ ] 2.2 Ensure all edge-endpoint construction (contains/calls/references/embeds…) uses the same
      sanitized fragments; test: edge targets byte-match node IDs for sanitized symbols
- [ ] 2.3 Fixture tests: SvelteKit tree (`+page.svelte`, `+layout.svelte`, `[slug]/+page.ts`),
      TS `$`-identifiers, `_`-prefixed dirs — every produced entity passes `ValidateEntityID`
- [ ] 2.4 Determinism test: repeated indexing of the fixtures yields identical IDs

## 3. Publish-gate parity

- [ ] 3.1 `internal/entitypub` ValidatePayload calls semstreams `ValidateEntityID`; rejection
      error names source instance + entity ID + offending segment
- [ ] 3.2 Rejections propagate to per-source `error_count`/`last_error`; unit tests

## 4. Publisher honesty

- [ ] 4.1 Replace DropOldest with bounded-blocking Send + loud drop after timeout; remove the
      dead-counter path so `dropped` actually increments
- [ ] 4.2 Surface drop counter in per-source status payload; WARN log per dropped entity
- [ ] 4.3 Race-tested unit tests: transient overflow delivers all; sustained overflow counts
      exactly; no goroutine leak (`-race`)

## 5. Delivery-truth counters

- [ ] 5.1 Split published vs confirmed counting in ast-source (and align the other source
      components' counter semantics); status reports confirmed
- [ ] 5.2 ast-source reports parse failures in `error_count` (audit: silently omitted); test

## 6. End-to-end proof

- [ ] 6.1 Integration test: ingest a fixture repo containing the SvelteKit/TS/underscore shapes →
      count of walker-eligible symbols equals count of entities landed in the graph (zero loss),
      and status error/drop counts are zero
- [ ] 6.2 Re-run against this repo with `ui/` included; verify workbench route components are
      queryable via code_context

## 7. Finalize

- [ ] 7.1 Release note: counter semantics + IDs now landing that previously never landed;
      `openspec validate no-silent-entity-loss`; gates green (revive pinned v1.15.0, gofmt, vet,
      `go test -race`)
