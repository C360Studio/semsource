# Design: No Silent Entity Loss

## Context

Audit 2026-07-19 reproduced end-to-end: AST-produced IDs with `+ $ [ ] ( ) @` or leading `_`
segments pass SemSource's publish gate (`internal/entitypub/entity_state.go` checks only NATS-KV
charset + 6 parts) and are Termed by semstreams graph-ingest (`pkg/types.ValidateEntityID`:
first byte alphanumeric, remaining `[a-zA-Z0-9_-]`). The publisher additionally drops on buffer
overflow (DropOldest) with a `dropped` counter that is never incremented, and ast-source counts
entities at parse time, not delivery.

## Goals / Non-Goals

- Goals: zero silent loss between parse and persistence; producer-visible attribution for every
  rejected/dropped entity; deterministic sanitized IDs; status that reflects delivery truth.
- Non-goals: retraction/staleness (separate change), ranking, substrate changes, changing the
  6-part scheme itself.

## Decisions

- **D1 — one sanitizer, in `entityid`**: extend `entityid` with `SanitizeSegment(string) string`
  (same allowlist/collapse/trim/hash-fallback semantics as `SanitizeInstance`, shared internals).
  `source/ast` `BuildScopedInstanceID` and `SanitizePathSegment` route every path- and
  symbol-derived fragment through it. Alternative considered: sanitize at the publish boundary —
  rejected: edges built from the same raw fragments would no longer byte-match node IDs; identity
  must be sanitized at construction so nodes and edge endpoints agree.
- **D2 — validate with the substrate's own validator**: `internal/entitypub` calls semstreams
  `pkg/types.ValidateEntityID` (already a dependency) instead of duplicating a regex. A pinning
  test asserts our sanitizer's output always passes it (property-style over nasty inputs).
- **D3 — publisher backpressure over silent drop**: on buffer full, `Send` blocks with a bounded
  timeout (ingest loops are our own and tolerate it); on timeout it drops LOUDLY: increments the
  (now real) `dropped` counter, logs WARN with entity ID + source, and the counter is surfaced in
  source status. DropOldest is removed. Alternative (unbounded block) rejected: a wedged NATS
  would freeze watch loops invisibly — bounded + loud is honest.
- **D4 — count on confirmation**: `entitiesIndexed`-style counters split into `published` and
  `confirmed` (delivery ack from the publisher path); status `entity_count` reports confirmed;
  parse failures and rejections increment `error_count` with a per-source `last_error`.
- **D5 — no ID migration needed**: every ID this change alters was previously rejected downstream
  (never landed), so no consumer-visible ID changes; a reindex simply fills the holes.

## Risks / Trade-offs

- [Sanitizer collisions] `+page` vs `-page`-like inputs could collapse → mitigated by
  SanitizeInstance's hash-suffix fallback semantics; property test asserts distinct raw inputs
  with confusable sanitizations stay distinct.
- [Backpressure slows seed] bounded timeout keeps worst-case bounded; seed is already async.
- [Counter semantics change] `entity_count` may dip vs. old inflated numbers — release-noted;
  honest-readiness change (P0-2) owns the distinct-count semantics.

## Migration Plan

Ship; reindex (periodic or restart) fills previously-dropped entities. Rollback = revert; no data
shape changes.

## Open Questions

- none blocking; timeout value (D3) tuned in implementation with a race-tested default.
