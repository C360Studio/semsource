# Design: Versioned-Source Current Marker & Historical Demotion

## Context

Item #2 relates versions with `code.lineage.supersedes` (newer→older) and its inverse
`code.lineage.superseded_by` (older→newer), emitted by the `supersession` pass onto the older entity of
each adjacent corresponding pair. So **an entity carries `superseded_by` iff a newer corresponding
version of that exact symbol exists** — i.e. it is *not* the current version of itself. That edge is
already in the graph; this change only teaches the ranker to read it.

semstreams beta.132 (gh#441) made `vocabulary.WithWeight` signed: negative = demote, and the fusion
ranker "folds an entity's strongest boost and strongest demotion together, so a negative weight is a
bounded reordering, never an exclusion."

## Goals / Non-Goals

**Goals:** current version of a symbol ranks above its historical versions in everyday retrieval;
historical versions stay fully retained and queryable; smallest possible surface (tier-0, no model, no
new producer).

**Non-Goals:** an explicit `code.artifact.current` presence marker / queryable current-filter /
tracking-vs-pinned gating (deferred); demoting whole historical *source versions* including
removed-in-newer symbols; deletion; retention-depth budgeting.

## Decisions

### D1. Demote by weighting `code.lineage.superseded_by` negative; "current" is emergent

Register `code.lineage.superseded_by` with a negative `WithWeight`. No new predicate, marker, pass, or
producer: the version that carries no `superseded_by` edge is by definition the newest of itself, so
demoting the superseded ones leaves the current one on top. This reuses the exact signal item #2
already emits and keeps the change to a single vocabulary edit.

*Alternative considered:* a dedicated `code.artifact.historical` marker stamped by the supersession
pass. Rejected for v1 — it duplicates information `superseded_by` already carries and adds a producer
write for no ranking gain.

### D2. Weight magnitude: demote comparable to the task-#38 boost, assert on ordering not value

Ranking folds strongest-boost + strongest-demotion. A current entity and its historical sibling carry
the *same* boosted predicates (e.g. both exported → +2.0), so the only rank difference between them is
the historical one's demotion — meaning **any** negative weight orders current above historical. The
magnitude only sets how far historical sinks relative to *unrelated* entities. Choose **-2.0**, mirroring
task #38's +2.0 exported boost, so a historical-but-exported symbol lands roughly back at neutral rather
than far below undocumented noise. The value is a tuning knob; the spec and tests assert the **ordering
invariant** (current ranks above its historical sibling), never a numeric score.

### D3. Retention-safety is intrinsic

A signed weight is a bounded reorder, never an exclusion (semstreams gh#441). Historical entities and
all their triples remain present and returnable by `graph.query.*`; they simply rank lower. This change
publishes nothing and retracts nothing — it is purely a registration.

## Risks / Trade-offs

- **Removed-in-newer symbols aren't demoted** — a symbol only in an older version has no
  `superseded_by`, so it ranks as current. Accepted (Non-goal): it is the latest of itself; whole-version
  historical demotion needs the explicit current marker, deferred.
- **Weight interplay with other salient predicates** — a heavily-boosted historical entity (capability
  +3.0) still outranks a bare current entity. Accepted: demotion is relative reordering, not a gate;
  tuning is a follow-up if consumers report noise.
- **No queryable "current"** — consumers wanting a hard current-only view can't filter yet. Accepted
  (Non-goal); the explicit marker follow-up covers it.

## Migration Plan

Purely additive: one `WithWeight(-2.0)` on an already-registered predicate. No IDs, triples, or stored
data change. Rollback = drop the weight (predicate returns to neutral). Effective on the next ranker
read; no reindex. Requires beta.132 (already on `main`).

## Open Questions

- **OQ1 — exact demote magnitude.** -2.0 is the proposed default (mirrors #38). Confirm against a live
  A/B once wired (does historical sink enough without burying legitimately-boosted historical hits?).
  The ordering invariant holds regardless, so this is tuning, not correctness.
