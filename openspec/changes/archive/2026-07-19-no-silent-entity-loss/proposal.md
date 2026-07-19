# Proposal: No Silent Entity Loss

**Priority: P0** (audit 2026-07-19, findings: critical ×2, high ×1, medium ×3)

## Why

The 2026-07-19 release-readiness audit empirically proved a silent-data-loss class between
SemSource's publishers and the governed graph: entities are published, rejected downstream (or
dropped in-process), and never land — while every producer-side surface reports healthy. An MCP
consumer gets a silently incomplete graph, which is the exact "hoping, not knowing" failure the
product exists to eliminate. This gates any public claim against competitors.

Concretely (all confirmed, most reproduced end-to-end):

1. **AST instance segments violate the graph-ingest segment alphabet** — `BuildScopedInstanceID`
   appends symbol names raw and `SanitizePathSegment` only maps `/` and `.`
   (`source/ast/entities.go:141-171`). SvelteKit `+page.svelte`, `[slug]`/`(group)`/`@modal`
   directories, `$`-identifiers, and `_`-prefixed paths all pass SemSource's publish gate and are
   rejected by semstreams `ValidateEntityID` — including this repo's own `ui/` workbench.
2. **The publish-boundary validator is weaker than graph-ingest's contract**
   (`internal/entitypub/entity_state.go:46`): no per-segment alphabet check, so drops happen where
   the producer cannot see them (graph-ingest Terms the message; only a substrate WARN log remains).
3. **The buffered publisher silently discards under backpressure** (`internal/entitypub/publisher.go:115`):
   DropOldest on overflow, files are marked ingested before delivery, and the surfaced `dropped`
   counter can never increment.
4. **ast-source counts entities as indexed before/regardless of delivery** and omits parse failures
   from `error_count` (`processor/ast-source/component.go:560,712`) — status shows `error_count: 0`
   while files were skipped and entities dropped.

## What Changes

- All AST-emitted ID segments (symbol names AND path-derived segments) are sanitized through the
  canonical `entityid` sanitizer so every produced ID satisfies the graph-ingest per-segment
  alphabet `^[a-zA-Z0-9][a-zA-Z0-9_-]*$` while remaining deterministic and collision-resistant
  (hash-fallback semantics, matching `entityid.SanitizeInstance`).
- The publish-boundary validator (`internal/entitypub`) enforces the same per-segment contract as
  semstreams graph-ingest — an entity that would be rejected downstream is rejected at the
  producer with an actionable error attributed to the source.
- The entity publisher stops silently discarding: overflow either applies backpressure or drops
  loudly — a real, incrementing drop counter surfaced in source status and logs.
- Source status honesty: `error_count` includes parse failures and publish rejections/drops;
  entities are counted as ingested only when delivery is confirmed (or counted separately as
  "published" vs "confirmed").
- **BREAKING (IDs)**: entities whose instance segments previously contained `+ $ [ ] ( ) @` or a
  leading `_` get new (sanitized) IDs. Today those entities never landed in the graph at all, so no
  consumer can hold references to the old IDs; the break is theoretical, not observable.

## Capabilities

### New Capabilities
- `entity-identity-safety`: every produced 6-part entity ID is deterministic AND valid per the
  graph-ingest segment contract, for all languages and all path shapes; sanitization is centralized
  in `entityid`.
- `entity-publish-integrity`: the publish boundary enforces downstream contract parity, never drops
  silently, and producer status reflects delivery truth (confirmed persists, visible failures).

### Modified Capabilities
<!-- none — existing specs (code-call-graph, code-reference-resolution) govern edges, not identity
     construction or publish delivery; no existing requirement changes. -->

## Impact

- `source/ast/entities.go` (BuildScopedInstanceID, SanitizePathSegment), `entityid/` (shared
  segment sanitizer), `internal/entitypub/` (validator + publisher), `processor/ast-source/`
  (counters, error_count), `processor/*-source/` (same counter contract).
- Consumers: all sem* MCP/GraphQL consumers get complete graphs for Svelte/TS repos; semspec canary
  should be re-run after this lands.
- Edges referencing sanitized IDs (Contains/Calls) must be constructed through the same sanitizer
  so edge endpoints match node IDs byte-for-byte.
- Boundary check: the segment contract itself is semstreams-owned (substrate); this change only
  makes SemSource honor it at the producer. No semstreams changes required.
