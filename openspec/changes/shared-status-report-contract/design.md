# Design — shared status-report contract

See `proposal.md` for motivation; deltas modify `ingestion-readiness` and
`onboarding-quickstart`.

## Context

- Producers: all eight `processor/*-source` components publish anonymous
  inline structs to `semsource.internal.status` from `publishStatusReport`.
  All eight already embed `internal/entitypub.Publisher`, which exposes
  `InBackpressure()` and `Lost()`.
- Consumer: `processor/source-manifest` decodes into its private
  `SourceStatusReport` (`status.go`), aggregates, and serves `SourceStatus`
  entries inside the registered `StatusPayload` (HTTP + MCP).
- Neither `SourceStatusReport` nor `SubmoduleStatus` is referenced outside
  `processor/source-manifest` today; git-source keeps a mirrored private
  `submoduleState` ("reports are duck-typed").
- semsource is one process: producer and consumer are always the same build,
  so wire-version skew between them cannot exist.

## Goals / Non-Goals

**Goals:** one definition of the report shape; drift structurally
impossible; `backpressure` + `boundaries_skipped` on every status surface;
no envelope breaks.

**Non-Goals:** new instrumentation (doc-source seed counters etc.); payload
registry registration for the internal subject; metrics changes.

## Decisions

### D1. `internal/sourcestatus` owns the report shape

New package `internal/sourcestatus` with `Report` and `SubmoduleStatus`.
Import direction: `processor/*` and `processor/source-manifest` →
`internal/sourcestatus` → `internal/seedsup` (for `*seedsup.Error`). No
cycles: `sourcestatus` imports nothing from `processor/`.
`sourcemanifest.SourceStatusReport` and `sourcemanifest.SubmoduleStatus`
become type aliases of the shared types so in-package call sites and tests
keep reading naturally; git-source's `submoduleState` is deleted outright.

*Why not register it in the payload registry?* The registry governs
polymorphic published contracts; this is monomorphic in-process chatter on a
point-to-point subject. The failure #188 exposed is re-declaration, and the
single shared type plus strict decode eliminates that. Registration remains
open to a later uniformity pass; nothing here precludes it.

### D2. Field census: the union, populated where true

`Report` carries: `instance_name`, `source_type`, `phase`, `entity_count`,
`publish_total`, `files_parsed`, `bodies_offloaded`, `boundaries_skipped`,
`error_count`, `type_counts`, `backpressure`, `submodules`, `last_error`,
`timestamp` — every field any producer sends today, all optional fields
`omitempty`. Producers populate what they measure: all eight wire
`Backpressure: c.publisher.InBackpressure()` (the publisher is already
there); `files_parsed`/`bodies_offloaded`/`boundaries_skipped` stay
ast-source-only until other sources grow the counters (out of scope).

### D3. Strict decode: reject loudly, don't fall back

The aggregator decodes with `DisallowUnknownFields`. On failure it logs at
error level (a code bug — see Context: skew is impossible in-process, so an
unknown field always means something bypassed the shared type) and drops the
report. No lenient fallback: a fallback would re-create exactly the silent
partial acceptance this change exists to kill, and the dropped source's
absence from status makes the bug undeniable in any test that looks.

### D4. Surface passthrough is additive

`SourceStatus` (inside the registered `StatusPayload`) gains `backpressure`
and `boundaries_skipped` with `omitempty` — the external envelope is
extended, never reshaped, so HTTP/MCP consumers see new optional fields
only. The health envelope derives from phases and is untouched.

### D5. The quickstart row lands with the signal

`docs/QUICKSTART.md` troubleshooting gains one row keyed on
`"backpressure": true` (meaning: publisher retrying against a
refusing/saturated transport — slow vs stalled; action: check NATS health/
capacity; the flag clearing on its own is recovery). Prose-only: no marked
blocks change, so the doc-driven e2e step tables are untouched (the tracks
still run in CI because the doc file changed — that's the drift gate doing
its job, not a step change).

## Risks / Trade-offs

- [Strict decode drops a buggy report entirely] → intended (D3); the bug is
  a same-binary code defect, caught by the round-trip/passthrough tests in
  this change before it could ship.
- [Alias types linger in sourcemanifest] → cosmetic; call sites keep
  compiling and the single definition still lives in one file. A later
  mechanical rename can drop the aliases.
- [Producers forget to wire a new field] → the field is simply absent
  (`omitempty`), same as today for sources without a counter — visible in
  the per-source status, not silent loss of a populated value.

## Open Questions

_None._
