# Proposal: Staleness and Retraction Markers

**Priority: P2** — design-first; completes ADR-0008's deferred deletion exception

## Why

The audit proved live that the retention-first graph currently has **no way to distinguish a
phantom from a fact**:

1. A deleted source file's entities remain served as current — full body,
   `provenance: "deterministic"`, at a LATER index revision, indistinguishable from live code
   (probe: entity still authoritative 80s+ after file deletion; `OpDelete` events are discarded,
   `processor/ast-source/component.go:386`). The readiness note promises "a miss once ready is a
   genuine absence" — the mirror statement ("a hit is a live fact") is unprovable today.
2. Doc identity makes this worse: the instance segment is the first 6 hex chars of the CONTENT
   hash (`handler/doc/entities.go:50`), so every edit mints a new entity and orphans the old one —
   stale doc versions accumulate and get served; the 24-bit prefix also collides at a few thousand
   docs.
3. `remove_source` leaves all ingested entities permanently query-visible with no provenance
   marker (`processor/source-manifest/ingest.go:307`) — deregistered data is indistinguishable and
   unpurgeable.
4. "watch disabled" / "one-shot snapshot" wording is contradicted by the always-on 60s periodic
   reindex — consumers pinning ephemeral worktrees (ADR-0007 sidecar) need a real snapshot
   semantics decision.

ADR-0008 chose RETAIN & RELATE deliberately (deletion by policy is graph-unsafe); it also named a
deferred deletion/staleness exception. This change designs and lands that exception.

## What Changes

(Design-first; the design doc decides mechanisms before tasks are finalized.)

- A staleness/absence marker: entities whose source artifact no longer exists (file deleted or
  renamed, source removed, doc superseded by edit) carry a governed marker predicate with signed
  negative salience (demoted, retained, filterable) — never silent equality with live facts.
- Doc edits stop orphaning: doc identity moves to a path-derived instance with the content hash as
  a predicate (edits become supersession, consistent with versioned-source-supersession), or an
  equivalent mechanism decided in design; the collision-prone 6-hex instance is retired.
- `remove_source` marks the removed source's entities with removal provenance (completing
  source-removal-integrity's deferral).
- Snapshot semantics: `watch:false` + periodic reindex behavior is specified honestly (either
  periodic updates are part of the contract and documented, or snapshot mode really freezes).
- Rename/delete handling emits the marker (not hard deletion); hard deletion remains the rare
  graph-aware exception gated on the upstream cascade-delete ask (asks #15/#433).

## Capabilities

### Modified Capabilities
- `versioned-source-supersession`: "Retention-safe supersession" extends to edit-as-supersession
  for docs; "Historical version demotion" extends to staleness demotion.
- `typed-source-change-events`: delete/rename watch events SHALL publish typed state changes (the
  marker), not be discarded.

### New Capabilities
- `entity-staleness`: the graph distinguishes live facts from retained-but-stale facts on every
  query surface, via governed markers and salience — retention-first, deletion-free.

## Impact

- `handler/doc/entities.go` (identity — **BREAKING for doc entity IDs**, migration = reindex),
  `processor/*-source/` watch paths, `source/vocabulary` (marker predicate), supersession pass,
  docs (snapshot semantics), ADR-0008 update (the exception's mechanics reference this spec).
- Consumers: agents can finally trust hits; UI can badge stale entities.
- Boundary check: marker + salience are product-side on existing substrate; cascade-delete stays
  an upstream ask (docs/upstream/semstreams-asks.md #15) — NOT implemented here.
